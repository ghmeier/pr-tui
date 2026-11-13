package plugin

import (
	"slices"
	"time"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// boardState is a registered board and everything the host keeps about it while it is shown.
type boardState struct {
	spec    Board
	rows    []Row
	visible []Row
	// data holds, for each column, the values its fetches delivered, by row key.
	data       []map[string]any
	facets     []facetState
	menuCursor int
	loadedAt   time.Time
	cursor     int
	offset     int
	notice     string
}

func (board *boardState) selected() (Row, bool) {
	if board.cursor < 0 || board.cursor >= len(board.visible) {
		return nil, false
	}
	return board.visible[board.cursor], true
}

// applyFilters recomputes the visible rows and keeps the cursor on the same row.
func (board *boardState) applyFilters() {
	selected, hadSelection := board.selected()
	board.visible = nil
	for _, row := range board.rows {
		if board.matches(row) {
			board.visible = append(board.visible, row)
		}
	}
	if !hadSelection {
		return
	}
	for index, row := range board.visible {
		if row.Key() == selected.Key() {
			board.cursor = index
			return
		}
	}
}

func (board *boardState) matches(row Row) bool {
	for _, state := range board.facets {
		if !state.matches(row) {
			return false
		}
	}
	return true
}

func (board *boardState) selection() (Selection, bool) {
	row, hasSelection := board.selected()
	if !hasSelection {
		return Selection{}, false
	}
	return Selection{Row: row, Data: func(header string) any {
		index := slices.IndexFunc(board.spec.Columns, func(column Column) bool { return column.Header == header })
		if index < 0 {
			return nil
		}
		return board.data[index][row.Key()]
	}}, true
}

func (board *boardState) cells() [][][]ui.Segment {
	cells := make([][][]ui.Segment, len(board.visible))
	for rowIndex, row := range board.visible {
		cells[rowIndex] = make([][]ui.Segment, len(board.spec.Columns))
		for columnIndex, column := range board.spec.Columns {
			cells[rowIndex][columnIndex] = column.Cell(row, board.data[columnIndex][row.Key()])
		}
	}
	return cells
}

func (board *boardState) table(width, selected, offset, rowCount int) string {
	headers := make([]string, len(board.spec.Columns))
	flexible := -1
	for index, column := range board.spec.Columns {
		headers[index] = column.Header
		if column.Flexible {
			flexible = index
		}
	}
	return renderTable(headers, flexible, board.cells(), width, selected, offset, rowCount)
}
