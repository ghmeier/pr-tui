package plugin

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/ui"
)

func (m *Model) tableRows() int {
	rows := m.height - 4
	if len(m.boards) > 1 {
		rows--
	}
	if m.mode == filterMode {
		rows -= lipgloss.Height(filterMenu(m.current().facets, m.current().menuCursor))
	}
	return max(rows, 1)
}

func (m *Model) statusLine(board *boardState) string {
	switch {
	case m.mode == choiceMode:
		return m.choice.line()
	case m.mode == confirmMode:
		return ui.KeyStyle.Render(m.question) + "  " + ui.HelpLine("y", "yes", "n", "no")
	case board.notice != "":
		return board.notice
	case board.loadedAt.IsZero():
		return ui.DimStyle.Render("Loading…")
	}
	return ui.DimStyle.Render(fmt.Sprintf(
		"%d of %d %s · updated %s · refreshes every %s",
		len(board.visible), len(board.rows), board.spec.RowNoun, board.loadedAt.Format("15:04"), intervalText(m.refreshInterval),
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
	for _, action := range m.current().spec.Actions {
		if action.Label != "" {
			keysAndLabels = append(keysAndLabels, action.Key, action.Label)
		}
	}
	if len(m.current().facets) > 0 {
		keysAndLabels = append(keysAndLabels, "f", "filters")
	}
	return ui.HelpLine(append(keysAndLabels, "r", "refresh", "q", "quit")...)
}

func (m *Model) tabBar() string {
	tabs := make([]string, len(m.boards))
	for index, board := range m.boards {
		if index == m.active {
			tabs[index] = ui.SelectedStyle.Render(" " + board.spec.Title + " ")
		} else {
			tabs[index] = ui.DimStyle.Render(" " + board.spec.Title + " ")
		}
	}
	return strings.Join(tabs, " ") + "  " + ui.HelpLine("tab", "next")
}

// aboveTable is everything drawn over the table: the tab bar when there is more than one board,
// the summary of the filters, and the filter menu while it is open.
func (m *Model) aboveTable() string {
	board := m.current()
	var sections []string
	if len(m.boards) > 1 {
		sections = append(sections, ansi.Truncate(m.tabBar(), m.width, "…"))
	}
	sections = append(sections, facetSummary(board.facets))
	if m.mode == filterMode {
		sections = append(sections, filterMenu(board.facets, board.menuCursor))
	}
	return strings.Join(sections, "\n")
}

// firstRowLine is the line of the screen where the table's first row is drawn, below the lines
// above the table and the header.
func (m *Model) firstRowLine() int {
	return strings.Count(m.aboveTable(), "\n") + 2
}

func (m *Model) View() string {
	if len(m.screens) > 0 {
		return m.viewTopScreen()
	}
	board := m.current()
	rows := m.tableRows()
	table := board.table(m.width, board.cursor, board.offset, rows)
	blankLines := rows + 1 - lipgloss.Height(table)
	return strings.Join([]string{
		m.aboveTable(),
		table + strings.Repeat("\n", max(blankLines, 0)),
		ansi.Truncate(m.statusLine(board), m.width, "…"),
		ansi.Truncate(m.help(), m.width, "…"),
	}, "\n")
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
