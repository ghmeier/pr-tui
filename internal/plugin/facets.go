package plugin

import (
	"slices"
	"strconv"
	"strings"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// numberKeyCount is how many options get a number key, as the keys are 1 to 9.
const numberKeyCount = 9

// facetState is a facet and which of its options are selected.
type facetState struct {
	facet    Facet
	selected map[string]bool
}

func newFacetState(facet Facet) facetState {
	selected := map[string]bool{}
	switch {
	case len(facet.Selected) > 0:
		for _, value := range facet.Selected {
			selected[value] = true
		}
	case facet.Single:
		if len(facet.Options) > 0 {
			selected[facet.Options[0].Value] = true
		}
	default:
		for _, option := range facet.Options {
			selected[option.Value] = true
		}
	}
	return facetState{facet: facet, selected: selected}
}

func (state facetState) matches(row Row) bool {
	return state.selected[state.facet.Value(row)]
}

// choose selects the option; for a single facet it replaces the selection, and for the others
// it toggles it.
func (state facetState) choose(value string) {
	if state.facet.Single {
		clear(state.selected)
		state.selected[value] = true
		return
	}
	state.selected[value] = !state.selected[value]
}

// cycle moves a single facet's selection to the next option, wrapping at the end.
func (state facetState) cycle() {
	current := slices.IndexFunc(state.facet.Options, func(option FacetOption) bool { return state.selected[option.Value] })
	next := (current + 1) % len(state.facet.Options)
	state.choose(state.facet.Options[next].Value)
}

func (state facetState) mark(option FacetOption) string {
	if state.selected[option.Value] {
		return "● " + option.Label
	}
	return ui.DimStyle.Render("○ " + option.Label)
}

// summary is the facet as one part of the line above the table: every option of a multiple
// facet, and only the chosen one of a single facet.
func (state facetState) summary() string {
	if state.facet.Single {
		var chosen []string
		for _, option := range state.facet.Options {
			if state.selected[option.Value] {
				chosen = append(chosen, option.Label)
			}
		}
		return state.facet.Title + ": " + ui.BoldStyle.Render(strings.Join(chosen, ", "))
	}
	parts := make([]string, len(state.facet.Options))
	for index, option := range state.facet.Options {
		parts[index] = state.mark(option)
	}
	return state.facet.Title + ": " + strings.Join(parts, "  ")
}

func facetSummary(facets []facetState) string {
	parts := make([]string, len(facets))
	for index, state := range facets {
		parts[index] = state.summary()
	}
	return strings.Join(parts, "    ")
}

// optionCount is the number of options over every facet, which is the length of the filter
// menu's cursor list.
func optionCount(facets []facetState) int {
	count := 0
	for _, state := range facets {
		count += len(state.facet.Options)
	}
	return count
}

// optionAt finds the facet and option at a position of the filter menu's cursor list.
func optionAt(facets []facetState, position int) (facetState, FacetOption, bool) {
	for _, state := range facets {
		if position < len(state.facet.Options) {
			return state, state.facet.Options[position], true
		}
		position -= len(state.facet.Options)
	}
	return facetState{}, FacetOption{}, false
}

func filterMenu(facets []facetState, cursor int) string {
	lines := []string{ui.BoldStyle.Render("Filters")}
	position := 0
	for _, state := range facets {
		title := ui.BoldStyle.Render(state.facet.Title)
		if state.facet.CycleKey != "" {
			title += "  " + ui.KeyStyle.Render(state.facet.CycleKey) + " " + ui.DimStyle.Render("next")
		}
		lines = append(lines, "", title)
		for _, option := range state.facet.Options {
			marker, key := " ", " "
			if position == cursor {
				marker = ui.CursorMarker
			}
			if position < numberKeyCount {
				key = ui.KeyStyle.Render(strconv.Itoa(position + 1))
			}
			lines = append(lines, marker+" "+key+"  "+state.mark(option))
			position++
		}
	}
	lines = append(lines, "", "  "+ui.HelpLine("↑↓", "move", "space", "toggle", "esc", "close"))
	return ui.MenuStyle.Render(strings.Join(lines, "\n"))
}

// handleFilterKey changes the selection for a key of the filter menu and reports whether the
// key was one of them. The menu cursor moves in place.
func handleFilterKey(facets []facetState, cursor *int, key string) bool {
	switch key {
	case "up", "k":
		*cursor = max(*cursor-1, 0)
		return false
	case "down", "j":
		*cursor = max(min(*cursor+1, optionCount(facets)-1), 0)
		return false
	case " ":
		if state, option, found := optionAt(facets, *cursor); found {
			state.choose(option.Value)
		}
		return true
	}
	if number, err := strconv.Atoi(key); err == nil && number >= 1 && number <= min(numberKeyCount, optionCount(facets)) {
		state, option, _ := optionAt(facets, number-1)
		state.choose(option.Value)
		return true
	}
	changed := false
	for _, state := range facets {
		if state.facet.Single && state.facet.CycleKey == key && len(state.facet.Options) > 0 {
			state.cycle()
			changed = true
		}
	}
	return changed
}
