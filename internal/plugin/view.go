package plugin

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// tableRows is how many rows of its table the focused board shows.
func (m *Model) tableRows() int {
	return m.paneOf(m.active).tableRows()
}

func (m *Model) statusLine(board *boardState) string {
	switch {
	case m.mode == choiceMode:
		return m.choice.line()
	case m.mode == confirmMode:
		return ui.KeyStyle.Render(m.question) + "  " + ui.HelpLine("y", "yes", "n", "no")
	case board.notice != "" && board.running > 1:
		return fmt.Sprintf("%s (%d running)", board.notice, board.running)
	case board.notice != "":
		return board.notice
	case board.loadedAt.IsZero():
		return ui.DimStyle.Render("Loading…")
	}
	return ui.DimStyle.Render(fmt.Sprintf(
		"%d of %d %s · updated %s · refreshes every %s",
		len(board.visible), len(board.rows), board.spec.RowNoun, board.loadedAt.Format("15:04"), intervalText(m.delayOf(board)),
	))
}

// intervalText says a refresh interval in whole minutes where it can, as in "3 minutes". A
// single minute or second has no number, so the status line reads "every second".
func intervalText(interval time.Duration) string {
	switch {
	case interval == time.Minute:
		return "minute"
	case interval == time.Second:
		return "second"
	case interval%time.Minute == 0:
		return fmt.Sprintf("%d minutes", interval/time.Minute)
	}
	return fmt.Sprintf("%d seconds", int(interval.Round(time.Second)/time.Second))
}

func (m *Model) help() string {
	switch m.mode {
	case filterMode:
		return ui.HelpLine("esc", "close filters", "q", "quit")
	case confirmMode, choiceMode:
		return ""
	}
	var keysAndLabels []string
	row, _ := m.current().selected()
	for _, action := range m.current().spec.Actions {
		if label, _ := action.LabelOf(row); label != "" {
			keysAndLabels = append(keysAndLabels, action.Key, label)
		}
	}
	if len(m.current().facets) > 0 {
		keysAndLabels = append(keysAndLabels, "f", "filters")
	}
	if len(m.tabs[m.currentTab()].boards) > 1 {
		keysAndLabels = append(keysAndLabels, "←/→", "section")
	}
	return ui.HelpLine(append(keysAndLabels, "r", "refresh", "q", "quit")...)
}

func (m *Model) tabBar() string {
	tabs := make([]string, len(m.tabs))
	for index, shown := range m.tabs {
		if index == m.currentTab() {
			tabs[index] = ui.SelectedStyle.Render(" " + shown.title + " ")
		} else {
			tabs[index] = ui.DimStyle.Render(" " + shown.title + " ")
		}
	}
	return strings.Join(tabs, " ") + "  " + ui.HelpLine("tab", "next")
}

func (m *Model) View() string {
	if len(m.screens) > 0 {
		return m.viewTopScreen()
	}
	lines := m.aboveLines(m.currentTab())
	lines = append(lines, m.renderPanes(m.panesOf(m.currentTab()))...)
	lines = append(lines,
		ansi.Truncate(m.statusLine(m.current()), m.width, "…"),
		ansi.Truncate(m.help(), m.width, "…"),
	)
	return strings.Join(lines, "\n")
}

// viewTopScreen draws the top screen. While a question or choice is pending or its board has a
// notice, the screen gives up its last line to the board's status line, because the board is
// hidden.
func (m *Model) viewTopScreen() string {
	top := len(m.screens) - 1
	board := m.boards[m.screenBoards[top]]
	if !m.asking() && board.notice == "" {
		return m.screens[top].View(m.width, m.height)
	}
	return m.screens[top].View(m.width, m.height-1) + "\n" + ansi.Truncate(m.statusLine(board), m.width, "…")
}

// maxShownOptions is how many options a choice can number with one key.
const maxShownOptions = 9

// shownOptions are the options that have a number key.
func (choice *Choice) shownOptions() []string {
	return choice.Options[:min(len(choice.Options), maxShownOptions)]
}

// line is the status line of a pending choice: the question, the numbered options with the
// current one in bold, and how to cancel.
func (choice *Choice) line() string {
	options := choice.shownOptions()
	labels := make([]string, len(options))
	for index, option := range options {
		label := option
		if option == choice.Current {
			label = ui.BoldStyle.Render(option)
		}
		labels[index] = ui.KeyStyle.Render(strconv.Itoa(index+1)) + " " + label
	}
	return ui.KeyStyle.Render(choice.Question) + "  " + strings.Join(labels, "  ") + "  " + ui.HelpLine("esc", "cancel")
}
