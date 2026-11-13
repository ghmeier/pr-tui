package plugin

import (
	"fmt"
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

func (m *Model) statusLine() string {
	b := m.current()
	switch {
	case m.mode == confirmMode:
		return ui.KeyStyle.Render(m.question) + "  " + ui.HelpLine("y", "yes", "n", "no")
	case b.notice != "":
		return b.notice
	case b.loadedAt.IsZero():
		return ui.DimStyle.Render("Loading…")
	}
	return ui.DimStyle.Render(fmt.Sprintf(
		"%d of %d %s · updated %s · refreshes every %s",
		len(b.visible), len(b.rows), b.spec.RowNoun, b.loadedAt.Format("15:04"), intervalText(m.refreshInterval),
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
	case confirmMode:
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
	for index, b := range m.boards {
		if index == m.active {
			tabs[index] = ui.SelectedStyle.Render(" " + b.spec.Title + " ")
		} else {
			tabs[index] = ui.DimStyle.Render(" " + b.spec.Title + " ")
		}
	}
	return strings.Join(tabs, " ") + "  " + ui.HelpLine("tab", "next")
}

func (m *Model) View() string {
	if len(m.screens) > 0 {
		return m.screens[len(m.screens)-1].View(m.width, m.height)
	}
	b := m.current()
	var sections []string
	if len(m.boards) > 1 {
		sections = append(sections, ansi.Truncate(m.tabBar(), m.width, "…"))
	}
	sections = append(sections, facetSummary(b.facets))
	if m.mode == filterMode {
		sections = append(sections, filterMenu(b.facets, b.menuCursor))
	}
	rows := m.tableRows()
	table := b.table(m.width, b.cursor, b.offset, rows)
	blankLines := rows + 1 - lipgloss.Height(table)
	sections = append(sections, table+strings.Repeat("\n", max(blankLines, 0)))
	sections = append(sections, ansi.Truncate(m.statusLine(), m.width, "…"), ansi.Truncate(m.help(), m.width, "…"))
	return strings.Join(sections, "\n")
}
