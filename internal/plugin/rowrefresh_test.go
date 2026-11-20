package plugin

import (
	"slices"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// reloader is a board's Reload that answers from a fixed set of rows and records what it was
// asked to reload. A row it has no entry for is reported as gone.
type reloader struct {
	mutex   sync.Mutex
	current map[string]Row
	asked   []string
}

func (reload *reloader) reload(row Row) (Row, error) {
	reload.mutex.Lock()
	defer reload.mutex.Unlock()
	reload.asked = append(reload.asked, row.Key())
	return reload.current[row.Key()], nil
}

func (reload *reloader) askedKeys() []string {
	reload.mutex.Lock()
	defer reload.mutex.Unlock()
	return slices.Clone(reload.asked)
}

// boardRefreshingRow is a grocery board whose action on "x" runs the command and reloads the
// selected row.
func boardRefreshingRow(source *rowSource, reload *reloader, command func() error) Board {
	board := groceryBoard(source)
	if reload != nil {
		board.Reload = reload.reload
	}
	board.Actions = []Action{{Key: "x", Run: func(selection Selection) Effect {
		return Effect{Notice: "Working on it…", Command: command, RefreshRow: selection.Row}
	}}}
	return board
}

func succeeds() error { return nil }

func TestRefreshRowReplacesOnlyThatRowWithoutLoadingTheBoard(t *testing.T) {
	rows := groceries()
	source := newRowSource(rows...)
	renamed := fruit(rows[0].Key(), "green apple")
	reload := &reloader{current: map[string]Row{renamed.key: renamed}}
	model := modelForBoard(t, boardRefreshingRow(source, reload, succeeds))
	loadsBefore := source.loadCount()

	plugintest.Press(t, model, "x")

	if expected := namesOf(renamed, rows[1], rows[2]); !slices.Equal(visibleNames(model), expected) {
		t.Fatalf("visible = %v, expected %v", visibleNames(model), expected)
	}
	if source.loadCount() != loadsBefore {
		t.Fatalf("the board loaded %d times", source.loadCount()-loadsBefore)
	}
	if asked := reload.askedKeys(); !slices.Equal(asked, []string{renamed.key}) {
		t.Fatalf("reloaded %v", asked)
	}
}

func TestRefreshRowKeepsTheCursorOnTheReloadedRow(t *testing.T) {
	source := newRowSource(groceries()...)
	renamed := vegetable("broccoli-key", "purple broccoli")
	reload := &reloader{current: map[string]Row{renamed.key: renamed}}
	model := modelForBoard(t, boardRefreshingRow(source, reload, succeeds))

	plugintest.Press(t, model, "down", "x")

	if name := selectedName(t, model); name != renamed.name {
		t.Fatalf("selected = %q, expected %q", name, renamed.name)
	}
}

func TestRefreshRowRemovesTheRowWhenReloadReturnsNil(t *testing.T) {
	rows := groceries()
	source := newRowSource(rows...)
	model := modelForBoard(t, boardRefreshingRow(source, &reloader{}, succeeds))

	plugintest.Press(t, model, "down", "x")

	if expected := namesOf(rows[0], rows[2]); !slices.Equal(visibleNames(model), expected) {
		t.Fatalf("visible = %v, expected %v", visibleNames(model), expected)
	}
	if name := selectedName(t, model); name != rows[2].(fakeRow).name {
		t.Fatalf("selected = %q, expected the row that moved up into the removed row's place", name)
	}
}

func TestRefreshRowKeepsTheRowWhenReloadReturnsIt(t *testing.T) {
	rows := groceries()
	source := newRowSource(rows...)
	reload := &reloader{current: map[string]Row{rows[1].Key(): rows[1]}}
	model := modelForBoard(t, boardRefreshingRow(source, reload, succeeds))

	plugintest.Press(t, model, "down", "x")

	if expected := namesOf(rows...); !slices.Equal(visibleNames(model), expected) {
		t.Fatalf("visible = %v, expected %v", visibleNames(model), expected)
	}
}

func TestRefreshRowLoadsTheWholeBoardWhenTheBoardCannotReloadARow(t *testing.T) {
	source := newRowSource(groceries()...)
	model := modelForBoard(t, boardRefreshingRow(source, nil, succeeds))
	loadsBefore := source.loadCount()

	plugintest.Press(t, model, "x")

	if source.loadCount() != loadsBefore+1 {
		t.Fatalf("loaded %d times after %d", source.loadCount(), loadsBefore)
	}
}

func TestRefreshRowDoesNotReloadWhenTheCommandFails(t *testing.T) {
	source := newRowSource(groceries()...)
	reload := &reloader{current: map[string]Row{}}
	model := modelForBoard(t, boardRefreshingRow(source, reload, func() error { return errorForTests }))

	plugintest.Press(t, model, "x")

	if asked := reload.askedKeys(); len(asked) != 0 {
		t.Fatalf("reloaded %v after a failed command", asked)
	}
	if status := statusLine(model); !strings.Contains(status, "Failed: "+errorForTests.Error()) {
		t.Fatalf("status = %q", status)
	}
}

func TestRowReloadedBeforeTheBoardLoadedAgainIsDropped(t *testing.T) {
	source := newRowSource(groceries()...)
	model := modelForBoard(t, boardRefreshingRow(source, &reloader{}, succeeds))
	board := model.current()
	staleGeneration := board.rowsGeneration - 1

	plugintest.Send(t, model, rowReloadedMessage{board: 0, generation: staleGeneration, key: "apple-key"})

	if expected := namesOf(groceries()...); !slices.Equal(visibleNames(model), expected) {
		t.Fatalf("visible = %v, expected %v", visibleNames(model), expected)
	}
}

func TestRowReloadedAfterTheLatestLoadIsApplied(t *testing.T) {
	source := newRowSource(groceries()...)
	model := modelForBoard(t, boardRefreshingRow(source, &reloader{}, succeeds))
	board := model.current()

	plugintest.Send(t, model, rowReloadedMessage{board: 0, generation: board.rowsGeneration, key: "apple-key"})

	if expected := namesOf(groceries()[1:]...); !slices.Equal(visibleNames(model), expected) {
		t.Fatalf("visible = %v, expected %v", visibleNames(model), expected)
	}
}

func TestRefreshRowLoadsColumnDataForTheReloadedRowOnly(t *testing.T) {
	var mutex sync.Mutex
	var rowsGiven [][]string
	board := boardRefreshingRow(newRowSource(groceries()...), &reloader{current: map[string]Row{
		"apple-key": fruit("apple-key", "green apple"),
	}}, succeeds)
	board.Columns = append(board.Columns, stockColumn(func(rows []Row) []Fetch {
		mutex.Lock()
		defer mutex.Unlock()
		var keys []string
		for _, row := range rows {
			keys = append(keys, row.Key())
		}
		rowsGiven = append(rowsGiven, keys)
		return []Fetch{func() (map[string]any, error) {
			values := map[string]any{}
			for _, key := range keys {
				values[key] = "in stock"
			}
			return values, nil
		}}
	}))
	model := modelForBoard(t, board)
	mutex.Lock()
	rowsGiven = nil
	mutex.Unlock()

	plugintest.Press(t, model, "x")

	mutex.Lock()
	defer mutex.Unlock()
	if len(rowsGiven) != 1 || !slices.Equal(rowsGiven[0], []string{"apple-key"}) {
		t.Fatalf("column load was given %v", rowsGiven)
	}
}

func TestRefreshRowDoesNotLoadColumnDataForARemovedRow(t *testing.T) {
	loads := 0
	board := boardRefreshingRow(newRowSource(groceries()...), &reloader{}, succeeds)
	board.Columns = append(board.Columns, stockColumn(func([]Row) []Fetch {
		loads++
		return nil
	}))
	model := modelForBoard(t, board)
	loadsAfterStart := loads

	plugintest.Press(t, model, "x")

	if loads != loadsAfterStart {
		t.Fatalf("column load ran %d more times", loads-loadsAfterStart)
	}
}

func TestRefreshRowRemovesTheColumnDataOfARemovedRow(t *testing.T) {
	board := boardRefreshingRow(newRowSource(groceries()...), &reloader{}, succeeds)
	board.Columns = append(board.Columns, stockColumn(func(rows []Row) []Fetch {
		return []Fetch{func() (map[string]any, error) {
			values := map[string]any{}
			for _, row := range rows {
				values[row.Key()] = "in stock"
			}
			return values, nil
		}}
	}))
	model := modelForBoard(t, board)

	plugintest.Press(t, model, "x")

	if _, found := model.current().data[2]["apple-key"]; found {
		t.Fatalf("column data of the removed row is still held")
	}
	if _, found := model.current().data[2]["broccoli-key"]; !found {
		t.Fatalf("column data of a row that stayed is gone")
	}
}

// startTwoCommands starts the action twice without letting either finish, and returns the
// commands that would finish them.
func startTwoCommands(model *Model) []tea.Cmd {
	_, first := model.Update(plugintest.KeyMessage("x"))
	_, second := model.Update(plugintest.KeyMessage("x"))
	return []tea.Cmd{first, second}
}

func TestStatusLineSaysHowManyCommandsAreRunningWhenThereAreSeveral(t *testing.T) {
	model := modelForBoard(t, boardRefreshingRow(newRowSource(groceries()...), &reloader{}, succeeds))

	commands := startTwoCommands(model)
	whileRunning := statusLine(model)
	for _, command := range commands {
		plugintest.Settle(t, model, command)
	}

	if !strings.Contains(whileRunning, "Working on it… (2 running)") {
		t.Fatalf("status = %q while two commands run", whileRunning)
	}
	if strings.Contains(statusLine(model), "running") {
		t.Fatalf("status = %q after both finished", statusLine(model))
	}
}

func TestStatusLineSaysNothingAboutRunningCommandsWhenThereIsOne(t *testing.T) {
	model := modelForBoard(t, boardRefreshingRow(newRowSource(groceries()...), &reloader{}, succeeds))

	_, command := model.Update(plugintest.KeyMessage("x"))
	whileRunning := statusLine(model)
	plugintest.Settle(t, model, command)

	if !strings.Contains(whileRunning, "Working on it…") || strings.Contains(whileRunning, "running") {
		t.Fatalf("status = %q while one command runs", whileRunning)
	}
}

func TestSuccessLeavesTheNoticeAloneWhileAnotherCommandIsRunning(t *testing.T) {
	release := make(chan struct{})
	finished := make(chan struct{})
	board := boardRefreshingRow(newRowSource(groceries()...), nil, succeeds)
	board.Actions = []Action{
		{Key: "x", Run: func(Selection) Effect {
			return Effect{Notice: "Slow work…", Command: func() error { <-release; return nil }}
		}},
		{Key: "y", Run: func(Selection) Effect {
			return Effect{Notice: "Quick work…", Command: func() error { close(finished); return nil }}
		}},
	}
	model := modelForBoard(t, board)

	_, slow := model.Update(plugintest.KeyMessage("x"))
	_, quick := model.Update(plugintest.KeyMessage("y"))
	slowDone := make(chan tea.Msg)
	go func() { slowDone <- slow() }()
	plugintest.Settle(t, model, quick)
	<-finished
	whileSlowRuns := statusLine(model)
	close(release)
	model.Update(<-slowDone)

	if !strings.Contains(whileSlowRuns, "Quick work…") {
		t.Fatalf("status = %q while the slow command still runs", whileSlowRuns)
	}
	if strings.Contains(statusLine(model), "work…") {
		t.Fatalf("status = %q after every command finished", statusLine(model))
	}
}

func TestSuccessDoesNotClearAFailureThatNobodyHasReadYet(t *testing.T) {
	board := boardRefreshingRow(newRowSource(groceries()...), nil, succeeds)
	board.Actions = []Action{
		{Key: "x", Run: func(Selection) Effect { return Effect{Command: func() error { return errorForTests }} }},
		{Key: "y", Run: func(Selection) Effect { return Effect{Command: succeeds} }},
	}
	model := modelForBoard(t, board)

	plugintest.Press(t, model, "x", "y")

	if status := statusLine(model); !strings.Contains(status, "Failed: "+errorForTests.Error()) {
		t.Fatalf("status = %q", status)
	}
}
