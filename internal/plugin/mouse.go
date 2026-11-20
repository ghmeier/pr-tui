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
	rowKey string
	column int
	at     time.Time
}

// handleMouse applies a mouse event to the board. While a filter menu, question, or choice is
// open, the mouse does nothing.
func (m *Model) handleMouse(event tea.MouseMsg) tea.Cmd {
	if m.mode != boardMode {
		return nil
	}
	board := m.current()
	switch {
	case event.Button == tea.MouseButtonWheelUp && event.Action == tea.MouseActionPress:
		board.cursor--
	case event.Button == tea.MouseButtonWheelDown && event.Action == tea.MouseActionPress:
		board.cursor++
	case event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionPress:
		return m.handleClick(event.X, event.Y)
	}
	return nil
}

// handleClick selects the row that was clicked, and copies the cell's text when the click is the
// second on that cell within the double click interval.
func (m *Model) handleClick(x, y int) tea.Cmd {
	board := m.current()
	rowOnScreen := y - m.firstRowLine()
	if rowOnScreen < 0 || rowOnScreen >= m.tableRows() || x < 0 || x >= m.width {
		return nil
	}
	rowIndex := board.offset + rowOnScreen
	if rowIndex >= len(board.visible) {
		return nil
	}
	cells := board.cells()
	column, found := columnAt(columnWidths(board.spec.Columns, cells, m.width), x)
	if !found {
		return nil
	}
	board.cursor = rowIndex

	row := board.visible[rowIndex]
	click := cellClick{rowKey: row.Key(), column: column, at: now()}
	previous := m.lastClick
	m.lastClick = click
	isDoubleClick := previous.rowKey == click.rowKey && previous.column == click.column &&
		click.at.Sub(previous.at) <= doubleClickInterval
	if !isDoubleClick {
		return nil
	}
	m.lastClick = cellClick{}
	text := ui.PlainText(cells[rowIndex][column])
	if text == "" {
		return nil
	}
	boardIndex := m.active
	return m.startCommand(boardIndex, func() error { return writeClipboard(text) },
		commandFinishedMessage{successNotice: "Copied " + text})
}
