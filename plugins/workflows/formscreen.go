package workflows

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

const (
	branchLabel = "Branch"
	// formHeaderLines and formFooterLines are the lines around the fields: the title and a
	// blank line above, and the help line below.
	formHeaderLines = 2
	formFooterLines = 1
	textInputWidth  = 60
)

// formField is one value the form asks for.
type formField struct {
	// name is the input's name, and empty for the branch.
	name        string
	label       string
	description string
	kind        InputKind
	required    bool
	options     []string
	selected    int
	checked     bool
	input       textinput.Model
	// problem is why the field blocks submitting, and empty when it does not.
	problem string
}

func (field *formField) editsText() bool {
	return field.kind != InputKindChoice && field.kind != InputKindBoolean
}

func (field *formField) value() string {
	switch field.kind {
	case InputKindChoice:
		if len(field.options) == 0 {
			return ""
		}
		return field.options[field.selected]
	case InputKindBoolean:
		return strconv.FormatBool(field.checked)
	}
	return strings.TrimSpace(field.input.Value())
}

// validate sets the problem of a field that cannot be submitted as it is.
func (field *formField) validate() {
	field.problem = ""
	value := field.value()
	switch {
	case value == "" && field.required:
		field.problem = "required"
	case value != "" && field.kind == InputKindNumber:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			field.problem = "must be a number"
		}
	}
}

func newTextField(label string, value string) formField {
	input := textinput.New()
	input.Prompt = ""
	input.Width = textInputWidth
	input.SetValue(value)
	// A static cursor needs no timer, so the screen runs no background commands.
	input.Cursor.SetMode(cursor.CursorStatic)
	return formField{label: label, kind: InputKindText, input: input}
}

func newInputField(input WorkflowInput) formField {
	field := newTextField(input.Name, input.Default)
	field.name, field.description, field.kind, field.required = input.Name, input.Description, input.Kind, input.Required
	field.options = input.Options
	field.selected = max(slices.Index(input.Options, input.Default), 0)
	field.checked = input.Default == "true"
	return field
}

// formScreen asks for the branch and the inputs of a manual run, and starts it once confirmed.
type formScreen struct {
	provider Provider
	workflow Workflow
	// selection is what the screen was opened from, and how it hands the run to the board.
	selection plugin.Selection
	fields    []formField
	focused   int
}

func newFormScreen(provider Provider, workflow Workflow, selection plugin.Selection) *formScreen {
	branch := newTextField(branchLabel, workflow.DefaultBranch)
	branch.required = true
	fields := []formField{branch}
	for _, input := range workflow.Inputs {
		fields = append(fields, newInputField(input))
	}
	screen := &formScreen{provider: provider, workflow: workflow, selection: selection, fields: fields}
	screen.fields[0].input.Focus()
	return screen
}

func (screen *formScreen) Init() tea.Cmd {
	return nil
}

// Update ignores everything but keys, because screens are given every message that the host
// does not handle.
func (screen *formScreen) Update(message tea.Msg) (plugin.Screen, tea.Cmd) {
	if key, isKey := message.(tea.KeyMsg); isKey {
		return screen.handleKey(key)
	}
	return screen, nil
}

func (screen *formScreen) handleKey(message tea.KeyMsg) (plugin.Screen, tea.Cmd) {
	switch message.String() {
	case "esc":
		return nil, nil
	case "ctrl+s":
		return screen.submit()
	case "up", "shift+tab":
		screen.moveFocus(-1)
		return screen, nil
	case "down", "tab":
		screen.moveFocus(1)
		return screen, nil
	}
	field := &screen.fields[screen.focused]
	switch {
	case field.editsText():
		field.input, _ = field.input.Update(message)
	case field.kind == InputKindChoice && len(field.options) > 0:
		switch message.String() {
		case "left", "h":
			field.selected = (field.selected + len(field.options) - 1) % len(field.options)
		case "right", "l":
			field.selected = (field.selected + 1) % len(field.options)
		}
	case field.kind == InputKindBoolean:
		if message.String() == " " {
			field.checked = !field.checked
		}
	}
	field.problem = ""
	return screen, nil
}

func (screen *formScreen) moveFocus(step int) {
	next := max(0, min(screen.focused+step, len(screen.fields)-1))
	screen.fields[screen.focused].input.Blur()
	screen.focused = next
	screen.fields[next].input.Focus()
}

// submit hands the run to the board as an effect, which asks before anything starts. A field
// that cannot be submitted is marked, and the form stays open.
func (screen *formScreen) submit() (plugin.Screen, tea.Cmd) {
	firstProblem := -1
	for index := range screen.fields {
		screen.fields[index].validate()
		if screen.fields[index].problem != "" && firstProblem < 0 {
			firstProblem = index
		}
	}
	if firstProblem >= 0 {
		screen.moveFocus(firstProblem - screen.focused)
		return screen, nil
	}

	branch := screen.fields[0].value()
	values := map[string]string{}
	var given []string
	for _, field := range screen.fields[1:] {
		value := field.value()
		values[field.name] = value
		if value != "" {
			given = append(given, field.name+"="+value)
		}
	}
	question := fmt.Sprintf("Run %s in %s on %s", screen.workflow.Name, screen.workflow.Repository.Name, branch)
	if len(given) > 0 {
		question += " with " + strings.Join(given, ", ")
	}
	provider, workflow := screen.provider, screen.workflow
	effect := plugin.Effect{
		Confirm: question + "?",
		Notice:  "Starting " + workflow.Name + "…",
		Command: func() error { return provider.StartWorkflow(workflow, branch, values) },
		Refresh: true,
	}
	return nil, screen.selection.Run(effect)
}

// fieldLines draws every field, with the focused field's description beneath it, and says which
// line the focused field is on.
func (screen *formScreen) fieldLines(width int) (lines []string, focusedLine int) {
	labelWidth := 0
	for _, field := range screen.fields {
		labelWidth = max(labelWidth, ansi.StringWidth(field.label))
	}
	for index := range screen.fields {
		field := &screen.fields[index]
		focused := index == screen.focused
		marker, label := "  ", ui.Fit(field.label, labelWidth)
		if focused {
			focusedLine = len(lines)
			marker, label = ui.CursorMarker+" ", ui.BoldStyle.Render(label)
		}
		line := marker + label + "  " + field.valueText(focused)
		if field.problem != "" {
			line += "  " + ui.RedStyle.Render(field.problem)
		}
		lines = append(lines, ansi.Truncate(line, width, "…"))
		if focused && field.description != "" {
			indent := strings.Repeat(" ", 4+labelWidth)
			lines = append(lines, ansi.Truncate(indent+ui.DimStyle.Render(field.description), width, "…"))
		}
	}
	return lines, focusedLine
}

func (field *formField) valueText(focused bool) string {
	switch field.kind {
	case InputKindChoice:
		value := field.value()
		if focused {
			return "◂ " + value + " ▸"
		}
		return value
	case InputKindBoolean:
		if field.checked {
			return "[x]"
		}
		return "[ ]"
	}
	return field.input.View()
}

func (screen *formScreen) help() string {
	keysAndLabels := []string{"↑/↓", "field"}
	switch screen.fields[screen.focused].kind {
	case InputKindChoice:
		keysAndLabels = append(keysAndLabels, "←/→", "option")
	case InputKindBoolean:
		keysAndLabels = append(keysAndLabels, "space", "toggle")
	}
	return ui.HelpLine(append(keysAndLabels, "ctrl+s", "run", "esc", "cancel")...)
}

// View scrolls so that the focused field stays on screen.
func (screen *formScreen) View(width, height int) string {
	title := ui.BoldStyle.Render("Run " + screen.workflow.Name + " in " + screen.workflow.Repository.FullName())
	lines, focusedLine := screen.fieldLines(width)
	rows := max(height-formHeaderLines-formFooterLines, 1)
	first := max(0, min(focusedLine-rows/2, len(lines)-rows))
	body := lines[first:min(len(lines), first+rows)]
	for len(body) < rows {
		body = append(body, "")
	}
	view := append([]string{ansi.Truncate(title, width, "…"), ""}, body...)
	return strings.Join(append(view, ansi.Truncate(screen.help(), width, "…")), "\n")
}
