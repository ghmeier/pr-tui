package plugin

import (
	"time"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// boardState is a registered board and everything the host keeps about it while it is shown.
type boardState struct {
	spec Board
	// index is the board's position among the host's boards, which messages from background
	// work carry back to find it.
	index int
	// tab is the position of the board's tab among the host's tabs.
	tab     int
	rows    []Row
	visible []Row
	// data holds, for each column, the values its fetches delivered, by row key.
	data []map[string]any
	// dataGenerations holds the load generation that each value in data came from, so that a
	// value from an older load never replaces one from a newer load.
	dataGenerations []map[string]int
	// loadGeneration counts the loads started. rowsGeneration is the generation of the load
	// whose rows are shown.
	loadGeneration int
	rowsGeneration int
	facets         []facetState
	menuCursor     int
	loadedAt       time.Time
	cursor         int
	offset         int
	notice         string
	// failure is the text of the last notice that reported a failure. A success does not clear
	// a notice that still says so, because the failure has not been read yet.
	failure string
	// running counts the commands of effects that have started and not yet finished.
	running int
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

// selection copies the selected row's loaded values, because an action's command reads them
// on a background goroutine while column results keep arriving on the update loop.
func (board *boardState) selection() (Selection, bool) {
	row, hasSelection := board.selected()
	if !hasSelection {
		return Selection{}, false
	}
	return board.selectionOf(row), true
}

// selectionOf is the selection for any row of the board, with its loaded values copied.
func (board *boardState) selectionOf(row Row) Selection {
	loaded := map[string]any{}
	for index, column := range board.spec.Columns {
		if value, found := board.data[index][row.Key()]; found {
			loaded[column.Header] = value
		}
	}
	return Selection{
		Row:        row,
		Data:       func(header string) any { return loaded[header] },
		boardIndex: board.index,
	}
}

// reportFailure shows a failure notice that later successes leave in place until a load or a
// new effect replaces it.
func (board *boardState) reportFailure(notice string) {
	board.notice = notice
	board.failure = notice
}

// reportFinished replaces the notice once no command is running, unless it still shows a
// failure that has not been read.
func (board *boardState) reportFinished(notice string) {
	if board.running == 0 && (board.failure == "" || board.notice != board.failure) {
		board.notice = notice
	}
}

// rowsChanged rebuilds what is derived from the rows: the facet options and the visible rows.
func (board *boardState) rowsChanged() {
	for facet := range board.facets {
		board.facets[facet].refreshOptionsFromRows(board.rows)
	}
	board.menuCursor = max(0, min(board.menuCursor, optionCount(board.facets)-1))
	board.applyFilters()
}

// forgetData drops the column values held for a row.
func (board *boardState) forgetData(key string) {
	for column, values := range board.data {
		delete(values, key)
		delete(board.dataGenerations[column], key)
	}
}

// rowWithKey finds a loaded row, whether or not the filters show it.
func (board *boardState) rowWithKey(key string) (Row, bool) {
	for _, row := range board.rows {
		if row.Key() == key {
			return row, true
		}
	}
	return nil, false
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
	return renderTable(board.spec.Columns, board.cells(), width, selected, offset, rowCount)
}
