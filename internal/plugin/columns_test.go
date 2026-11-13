package plugin

import (
	"sync"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// stockFor delivers the given stock for rows by key.
func stockFor(values map[string]any) Fetch {
	return func() (map[string]any, error) { return values, nil }
}

// boardWithStock is a board whose Stock column fetches what the function returns.
func boardWithStock(rows []Row, load func(rows []Row) []Fetch) Board {
	board := groceryBoard(newRowSource(rows...))
	board.Columns = append(board.Columns, stockColumn(load))
	return board
}

// stockCell is the text of the last column of the visible row at the index.
func stockCell(model *Model, index int) string {
	board := model.current()
	return ui.PlainText(board.cells()[index][len(board.spec.Columns)-1])
}

func TestCellGetsNoDataUntilAFetchHasDeliveredAValueForTheRow(t *testing.T) {
	rows := groceries()
	release := make(chan struct{})
	board := boardWithStock(rows, func([]Row) []Fetch {
		return []Fetch{func() (map[string]any, error) {
			<-release
			return map[string]any{rows[0].Key(): "12 left"}, nil
		}}
	})
	host, err := buildHost(t, `plugins = ["fake"]`, boardPlugin("fake", board))
	if err != nil {
		t.Fatal(err)
	}
	model := host.Model()

	_, fetching := model.Update(loadedMessage{board: 0, rows: rows})
	waiting := stockCell(model, 0)
	close(release)
	plugintest.Settle(t, model, fetching)

	if waiting != "…" || stockCell(model, 0) != "12 left" {
		t.Fatalf("cell before the fetch = %q, after = %q", waiting, stockCell(model, 0))
	}
}

func TestOnlyTheRowsAFetchDeliveredHaveData(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, boardWithStock(rows, func([]Row) []Fetch {
		return []Fetch{stockFor(map[string]any{rows[1].Key(): "none"})}
	}))

	if stockCell(model, 1) != "none" || stockCell(model, 0) != "…" || stockCell(model, 2) != "…" {
		t.Fatalf("cells = %q, %q, %q", stockCell(model, 0), stockCell(model, 1), stockCell(model, 2))
	}
}

func TestFetchesOfAColumnRunConcurrently(t *testing.T) {
	rows := groceries()
	var gate sync.WaitGroup
	gate.Add(2)
	meet := func(key, value string) Fetch {
		return func() (map[string]any, error) {
			gate.Done()
			gate.Wait()
			return map[string]any{key: value}, nil
		}
	}
	finished := make(chan *Model)

	go func() {
		finished <- modelForBoard(t, boardWithStock(rows, func([]Row) []Fetch {
			return []Fetch{meet(rows[0].Key(), "first"), meet(rows[1].Key(), "second")}
		}))
	}()

	select {
	case model := <-finished:
		if stockCell(model, 0) != "first" || stockCell(model, 1) != "second" {
			t.Fatalf("cells = %q and %q", stockCell(model, 0), stockCell(model, 1))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the fetches did not run at the same time, so neither could finish")
	}
}

func TestFetchErrorLeavesThoseCellsUnloadedAndKeepsTheOthers(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, boardWithStock(rows, func([]Row) []Fetch {
		return []Fetch{
			func() (map[string]any, error) { return map[string]any{rows[0].Key(): "ignored"}, errorForTests },
			stockFor(map[string]any{rows[1].Key(): "none"}),
		}
	}))

	if stockCell(model, 0) != "…" || stockCell(model, 1) != "none" {
		t.Fatalf("cells = %q and %q", stockCell(model, 0), stockCell(model, 1))
	}
}

// switchableStock delivers a value for the first row only while it is switched on.
type switchableStock struct {
	mutex sync.Mutex
	on    bool
	key   string
}

func (stock *switchableStock) switchTo(on bool) {
	stock.mutex.Lock()
	defer stock.mutex.Unlock()
	stock.on = on
}

func (stock *switchableStock) load([]Row) []Fetch {
	return []Fetch{func() (map[string]any, error) {
		stock.mutex.Lock()
		defer stock.mutex.Unlock()
		if !stock.on {
			return nil, errorForTests
		}
		return map[string]any{stock.key: "12 left"}, nil
	}}
}

func TestLoadedValuesSurviveARefreshWhoseFetchFails(t *testing.T) {
	rows := groceries()
	stock := &switchableStock{on: true, key: rows[0].Key()}
	model := modelForBoard(t, boardWithStock(rows, stock.load))
	stock.switchTo(false)

	plugintest.Press(t, model, "r")

	if stockCell(model, 0) != "12 left" {
		t.Fatalf("cell after a refresh whose fetch failed = %q", stockCell(model, 0))
	}
}

func TestValuesOfRowsThatLeftTheBoardAreDroppedAtTheRefreshThatDropsThem(t *testing.T) {
	rows := groceries()
	source := newRowSource(rows...)
	stock := &switchableStock{on: true, key: rows[0].Key()}
	board := groceryBoard(source)
	board.Columns = append(board.Columns, stockColumn(stock.load))
	model := modelForBoard(t, board)
	stock.switchTo(false)
	source.set(rows[1], rows[2])
	plugintest.Press(t, model, "r")
	source.set(rows...)

	plugintest.Press(t, model, "r")

	if stockCell(model, 0) != "…" {
		t.Fatalf("cell of a row that was away for a refresh = %q", stockCell(model, 0))
	}
}

func TestValueArrivingAfterTheRefreshThatStartedItStillShows(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, boardWithStock(rows, func([]Row) []Fetch { return nil }))
	column := len(model.current().spec.Columns) - 1

	plugintest.Send(t, model, columnLoadedMessage{board: 0, column: column, values: map[string]any{rows[0].Key(): "late"}})

	if stockCell(model, 0) != "late" {
		t.Fatalf("cell = %q", stockCell(model, 0))
	}
}

func TestLoadIsGivenTheRowsOfTheRefresh(t *testing.T) {
	rows := groceries()
	var given []Row
	var mutex sync.Mutex

	modelForBoard(t, boardWithStock(rows, func(loaded []Row) []Fetch {
		mutex.Lock()
		defer mutex.Unlock()
		given = loaded
		return nil
	}))

	if len(given) != len(rows) || given[0].Key() != rows[0].Key() {
		t.Fatalf("Load was given %v", given)
	}
}

// actionReadingStock is an action that records the Stock data of its selection.
func actionReadingStock(seen *any) Action {
	return Action{Key: "x", Run: func(selection Selection) Effect {
		*seen = selection.Data("Stock")
		return Effect{}
	}}
}

func TestSelectionDataIsTheLoadedValueOfAnotherColumnForTheSelectedRow(t *testing.T) {
	rows := groceries()
	board := boardWithStock(rows, func([]Row) []Fetch {
		return []Fetch{stockFor(map[string]any{rows[0].Key(): "12 left"})}
	})
	var seen any
	board.Actions = []Action{actionReadingStock(&seen)}
	model := modelForBoard(t, board)

	plugintest.Press(t, model, "x")

	if seen != "12 left" {
		t.Fatalf("data of the Stock column = %v", seen)
	}
}

func TestSelectionDataIsNilForAColumnNotLoadedForTheSelectedRowOrNotOnTheBoard(t *testing.T) {
	rows := groceries()
	board := boardWithStock(rows, func([]Row) []Fetch {
		return []Fetch{stockFor(map[string]any{rows[1].Key(): "none"})}
	})
	var unloaded any = "unset"
	var unknown any = "unset"
	board.Actions = []Action{
		actionReadingStock(&unloaded),
		{Key: "y", Run: func(selection Selection) Effect {
			unknown = selection.Data("No such column")
			return Effect{}
		}},
	}
	model := modelForBoard(t, board)

	plugintest.Press(t, model, "x", "y")

	if unloaded != nil || unknown != nil {
		t.Fatalf("unloaded = %v, unknown = %v", unloaded, unknown)
	}
}
