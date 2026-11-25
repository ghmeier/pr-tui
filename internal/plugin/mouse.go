package plugin

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// doubleClickInterval is the longest time between two clicks on a cell that still counts as a
// double click.
const doubleClickInterval = 500 * time.Millisecond

// now is the clock that recognises double clicks. Tests replace it.
var now = time.Now

// cellClick remembers a left click on a cell. A row is identified by its key, because the
// rows may be reordered or reloaded between the two clicks of a double click.
type cellClick struct {
	board  int
	rowKey string
	column int
	at     time.Time
}

// handleMouse applies a mouse event to the pane it happened in. While a filter menu, question,
// or choice is open, the mouse does nothing.
func (m *Model) handleMouse(event tea.MouseMsg) tea.Cmd {
	if m.mode != boardMode {
		return nil
	}
	switch {
	case event.Button == tea.MouseButtonWheelUp && event.Action == tea.MouseActionPress:
		m.scrollPaneAt(event.X, event.Y, -1)
	case event.Button == tea.MouseButtonWheelDown && event.Action == tea.MouseActionPress:
		m.scrollPaneAt(event.X, event.Y, 1)
	case event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionPress:
		return m.handleClick(event.X, event.Y)
	}
	return nil
}

// scrollPaneAt moves the cursor of the board under the pointer, without changing the focus. A
// tab with one board scrolls it wherever the pointer is.
func (m *Model) scrollPaneAt(x, y, step int) {
	panes := m.panesOf(m.currentTab())
	target, found := m.paneAt(x, y)
	if !found && len(panes) == 1 {
		target, found = panes[0], true
	}
	if found {
		m.boards[target.board].cursor += step
	}
}

// handleClick focuses the pane that was clicked and selects the row that was clicked, and
// copies the cell's text when the click is the second on that cell within the double click
// interval.
func (m *Model) handleClick(x, y int) tea.Cmd {
	clicked, found := m.paneAt(x, y)
	if !found {
		return nil
	}
	m.focus(clicked.board)
	board := m.boards[clicked.board]
	rowOnScreen := y - clicked.headerLine() - 1
	if rowOnScreen < 0 || rowOnScreen >= clicked.tableRows() {
		return nil
	}
	rowIndex := board.offset + rowOnScreen
	if rowIndex >= len(board.visible) {
		return nil
	}
	cells := board.cells()
	column, found := columnAt(columnWidths(board.drawnColumns(), cells, clicked.width), x-clicked.x)
	if !found {
		return nil
	}
	board.cursor = rowIndex

	row := board.visible[rowIndex]
	click := cellClick{board: clicked.board, rowKey: row.Key(), column: column, at: now()}
	previous := m.lastClick
	m.lastClick = click
	isDoubleClick := previous.board == click.board && previous.rowKey == click.rowKey && previous.column == click.column &&
		click.at.Sub(previous.at) <= doubleClickInterval
	if !isDoubleClick {
		return nil
	}
	m.lastClick = cellClick{}
	text := ui.PlainText(cells[rowIndex][column])
	if text == "" {
		return nil
	}
	return m.copyToClipboard(clicked.board, text, copiedNotice(text))
}
