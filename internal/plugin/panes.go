package plugin

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/ui"
)

const (
	// minimumPaneWidth is the narrowest a pane is made before panes wrap onto another row.
	minimumPaneWidth = 60
	paneSeparator    = "│"
	// linesBelowPanes are the status line and the help line.
	linesBelowPanes = 2
)

// pane is the part of the screen where one board of the shown tab is drawn. The view and the
// mouse both take their positions from it.
type pane struct {
	board         int
	x, y          int
	width, height int
	// titled panes draw the board's title and facet summary above its table. The only pane of a
	// tab has neither, because the lines above the panes show the summary.
	titled bool
}

// headerLine is the line of the screen where the table's header is drawn.
func (shown pane) headerLine() int {
	if shown.titled {
		return shown.y + 2
	}
	return shown.y
}

// tableRows is how many rows of the table fit under its header.
func (shown pane) tableRows() int {
	return max(shown.y+shown.height-shown.headerLine()-1, 1)
}

func (shown pane) contains(x, y int) bool {
	return x >= shown.x && x < shown.x+shown.width && y >= shown.y && y < shown.y+shown.height
}

// aboveLines are the lines drawn over the panes of a tab: the tab bar when there is more than
// one tab, the summary of the filters when the tab has a single board, and the filter menu while
// it is open for the shown tab.
func (m *Model) aboveLines(tabIndex int) []string {
	var sections []string
	if len(m.tabs) > 1 {
		sections = append(sections, ansi.Truncate(m.tabBar(), m.width, "…"))
	}
	boards := m.tabs[tabIndex].boards
	if len(boards) == 1 {
		sections = append(sections, facetSummary(m.boards[boards[0]].facets))
	}
	if m.mode == filterMode && tabIndex == m.currentTab() {
		sections = append(sections, filterMenu(m.current().facets, m.current().menuCursor))
	}
	var lines []string
	for _, section := range sections {
		lines = append(lines, strings.Split(section, "\n")...)
	}
	return lines
}

// panesOf lays the boards of a tab out in a grid of rows and columns that fills the space
// between the lines above and the lines below. Boards run along each row in registration order.
// A row that is not full gives its width to the panes it has.
func (m *Model) panesOf(tabIndex int) []pane {
	boards := m.tabs[tabIndex].boards
	top := len(m.aboveLines(tabIndex))
	available := max(m.height-top-linesBelowPanes, 1)
	columns := min(len(boards), max(1, m.width/minimumPaneWidth))
	gridRows := (len(boards) + columns - 1) / columns

	var panes []pane
	y := top
	for gridRow := range gridRows {
		first := gridRow * columns
		inRow := min(columns, len(boards)-first)
		height := available / gridRows
		if gridRow == gridRows-1 {
			height = available - height*(gridRows-1)
		}
		usable := m.width - (inRow - 1)
		width := usable / inRow
		x := 0
		for position := range inRow {
			paneWidth := width
			if position == inRow-1 {
				paneWidth = usable - width*(inRow-1)
			}
			panes = append(panes, pane{board: boards[first+position], x: x, y: y, width: paneWidth, height: height, titled: len(boards) > 1})
			x += paneWidth + len(paneSeparator)
		}
		y += height
	}
	return panes
}

// paneOf is where a board is drawn when its tab is shown.
func (m *Model) paneOf(boardIndex int) pane {
	for _, candidate := range m.panesOf(m.boards[boardIndex].tab) {
		if candidate.board == boardIndex {
			return candidate
		}
	}
	return pane{}
}

// paneAt finds the pane of the shown tab that covers a position of the screen.
func (m *Model) paneAt(x, y int) (pane, bool) {
	for _, candidate := range m.panesOf(m.currentTab()) {
		if candidate.contains(x, y) {
			return candidate, true
		}
	}
	return pane{}, false
}

// renderPane draws a pane as exactly its height in lines. Only the focused pane shows its cursor
// row, so that the one that keys apply to is clear.
func (m *Model) renderPane(shown pane) []string {
	board := m.boards[shown.board]
	var lines []string
	if shown.titled {
		title := ansi.Truncate(board.spec.Title, shown.width, "…")
		if shown.board == m.active {
			lines = append(lines, ui.SelectedStyle.Render(title))
		} else {
			lines = append(lines, ui.DimStyle.Bold(true).Render(title))
		}
		lines = append(lines, ansi.Truncate(facetSummary(board.facets), shown.width, "…"))
	}
	selected := -1
	if shown.board == m.active {
		selected = board.cursor
	}
	lines = append(lines, strings.Split(board.table(shown.width, selected, board.offset, shown.tableRows()), "\n")...)
	for len(lines) < shown.height {
		lines = append(lines, "")
	}
	return lines
}

// renderPanes draws the panes row by row. Panes that share a row are padded to their width and
// separated by a vertical bar.
func (m *Model) renderPanes(panes []pane) []string {
	var lines []string
	for start := 0; start < len(panes); {
		end := start + 1
		for end < len(panes) && panes[end].y == panes[start].y {
			end++
		}
		row := panes[start:end]
		if len(row) == 1 {
			lines = append(lines, m.renderPane(row[0])...)
		} else {
			columns := make([][]string, len(row))
			for index, shown := range row {
				columns[index] = m.renderPane(shown)
			}
			for line := range row[0].height {
				cells := make([]string, len(row))
				for index, shown := range row {
					cells[index] = padToWidth(columns[index][line], shown.width)
				}
				lines = append(lines, strings.Join(cells, paneSeparator))
			}
		}
		start = end
	}
	return lines
}

func padToWidth(line string, width int) string {
	return line + strings.Repeat(" ", max(width-lipgloss.Width(line), 0))
}
