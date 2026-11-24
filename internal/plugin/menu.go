package plugin

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// maxMenuOptions is how many options a menu shows at once; the others scroll into view.
const maxMenuOptions = 10

// menuOpen says whether the pending choice is shown as a menu, which takes every key.
func (m *Model) menuOpen() bool {
	return m.mode == choiceMode && m.choice.Menu
}

// matching are the options that contain the typed text, ignoring case, in their own order.
func (choice *Choice) matching(typed string) []string {
	if typed == "" {
		return choice.Options
	}
	typed = strings.ToLower(typed)
	var matches []string
	for _, option := range choice.Options {
		if strings.Contains(strings.ToLower(option), typed) {
			matches = append(matches, option)
		}
	}
	return matches
}

// closeChoice ends the pending choice and forgets what was typed into it.
func (m *Model) closeChoice() {
	m.mode, m.choice, m.choiceTyped, m.choiceCursor = boardMode, nil, "", 0
}

// handleMenuKey gives a key to the open menu. Printable characters are typed, so none of them
// can double as a command.
func (m *Model) handleMenuKey(key string) tea.Cmd {
	matches := m.choice.matching(m.choiceTyped)
	switch key {
	case "esc":
		m.closeChoice()
	case "up", "ctrl+p":
		m.choiceCursor = max(m.choiceCursor-1, 0)
	case "down", "ctrl+n":
		m.choiceCursor = max(min(m.choiceCursor+1, len(matches)-1), 0)
	case "backspace":
		if m.choiceTyped != "" {
			_, size := utf8.DecodeLastRuneInString(m.choiceTyped)
			m.choiceTyped = m.choiceTyped[:len(m.choiceTyped)-size]
		}
	case "enter":
		if m.choiceCursor >= len(matches) {
			return nil
		}
		choice, index, picked := m.choice, m.pendingBoard, matches[m.choiceCursor]
		m.closeChoice()
		return m.applyEffect(index, choice.Pick(picked))

	default:
		if character, _ := utf8.DecodeRuneInString(key); utf8.RuneCountInString(key) == 1 && unicode.IsPrint(character) {
			m.choiceTyped += key
		}
	}
	if m.choice != nil {
		m.choiceCursor = max(min(m.choiceCursor, len(m.choice.matching(m.choiceTyped))-1), 0)
	}
	return nil
}

// menuView draws the open menu: the question with the typed text, the options around the cursor,
// and a count when some options are not shown.
func (m *Model) menuView() string {
	matches := m.choice.matching(m.choiceTyped)
	cursor := max(min(m.choiceCursor, len(matches)-1), 0)
	first := max(min(cursor-maxMenuOptions/2, len(matches)-maxMenuOptions), 0)
	shown := matches[first:min(first+maxMenuOptions, len(matches))]

	lines := []string{ui.KeyStyle.Render(m.choice.Question) + " " + m.choiceTyped + ui.DimStyle.Render("▏")}
	for index, option := range shown {
		marker := " "
		if first+index == cursor {
			marker = ui.CursorMarker
		}
		if option == m.choice.Current {
			option = ui.BoldStyle.Render(option)
		}
		lines = append(lines, marker+" "+option)
	}
	if len(matches) == 0 {
		lines = append(lines, "  "+ui.DimStyle.Render("No matches"))
	}
	if len(shown) != len(m.choice.Options) {
		lines = append(lines, "  "+ui.DimStyle.Render(fmt.Sprintf("%d of %d", len(shown), len(m.choice.Options))))
	}
	lines = append(lines, "", "  "+ui.HelpLine("↑↓", "move", "enter", "pick", "esc", "cancel"))
	return ui.MenuStyle.Render(strings.Join(lines, "\n"))
}
