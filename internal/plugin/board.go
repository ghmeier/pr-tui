package plugin

import (
	"slices"
	"time"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// board is a registered board and everything the host keeps about it while it is shown.
type board struct {
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

func (b *board) selected() (Row, bool) {
	if b.cursor < 0 || b.cursor >= len(b.visible) {
		return nil, false
	}
	return b.visible[b.cursor], true
}

// applyFilters recomputes the visible rows and keeps the cursor on the same row.
func (b *board) applyFilters() {
	selected, hadSelection := b.selected()
	b.visible = nil
	for _, row := range b.rows {
		if b.matches(row) {
			b.visible = append(b.visible, row)
		}
	}
	if !hadSelection {
		return
	}
	for index, row := range b.visible {
		if row.Key() == selected.Key() {
			b.cursor = index
			return
		}
	}
}

func (b *board) matches(row Row) bool {
	for _, state := range b.facets {
		if !state.matches(row) {
			return false
		}
	}
	return true
}

func (b *board) selection() (Selection, bool) {
	row, hasSelection := b.selected()
	if !hasSelection {
		return Selection{}, false
	}
	return Selection{Row: row, Data: func(header string) any {
		index := slices.IndexFunc(b.spec.Columns, func(column Column) bool { return column.Header == header })
		if index < 0 {
			return nil
		}
		return b.data[index][row.Key()]
	}}, true
}

func (b *board) cells() [][][]ui.Segment {
	cells := make([][][]ui.Segment, len(b.visible))
	for rowIndex, row := range b.visible {
		cells[rowIndex] = make([][]ui.Segment, len(b.spec.Columns))
		for columnIndex, column := range b.spec.Columns {
			cells[rowIndex][columnIndex] = column.Cell(row, b.data[columnIndex][row.Key()])
		}
	}
	return cells
}

func (b *board) table(width, selected, offset, rowCount int) string {
	headers := make([]string, len(b.spec.Columns))
	flexible := -1
	for index, column := range b.spec.Columns {
		headers[index] = column.Header
		if column.Flexible {
			flexible = index
		}
	}
	return renderTable(headers, flexible, b.cells(), width, selected, offset, rowCount)
}
