package plugin

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

func TestBoardListsTheRowsOfTheLoadInOrderUnderTheColumnHeaders(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, groceryBoard(newRowSource(rows...)))

	view := tableLines(model)

	headerLine := view[0]
	if !strings.Contains(headerLine, "Name") || !strings.Contains(headerLine, "Group") {
		t.Fatalf("header line = %q", headerLine)
	}
	for index, row := range rows {
		if !strings.Contains(view[1+index], row.(fakeRow).name) {
			t.Fatalf("line %d = %q, expected the row %q", 1+index, view[1+index], row.(fakeRow).name)
		}
	}
}

func TestBoardShowsTheLoadFailureAndNoRowsWhenTheFirstLoadFails(t *testing.T) {
	source := newRowSource(groceries()...)
	source.fail(errorForTests)

	model := modelForBoard(t, groceryBoard(source))

	if !strings.Contains(screenText(model), "Refresh failed: "+errorForTests.Error()) || len(visibleNames(model)) != 0 {
		t.Fatalf("view:\n%s", screenText(model))
	}
}

func TestBoardKeepsItsRowsWhenALaterRefreshFails(t *testing.T) {
	source := newRowSource(groceries()...)
	model := modelForBoard(t, groceryBoard(source))
	source.fail(errorForTests)

	plugintest.Press(t, model, "r")

	if len(visibleNames(model)) != len(groceries()) || !strings.Contains(screenText(model), "Refresh failed: ") {
		t.Fatalf("view:\n%s", screenText(model))
	}
}

func TestBoardShowsOnlyTheFirstLineOfAFailure(t *testing.T) {
	source := newRowSource(groceries()...)
	source.fail(fmt.Errorf("first line\nsecond line"))

	model := modelForBoard(t, groceryBoard(source))

	if view := screenText(model); !strings.Contains(view, "first line") || strings.Contains(view, "second line") {
		t.Fatalf("view:\n%s", view)
	}
}

func TestTableTruncatesTheFlexibleColumnToTheTerminalWidth(t *testing.T) {
	long := fruit("long-key", strings.Repeat("long name ", 30))
	model := modelForBoard(t, groceryBoard(newRowSource(long)))

	plugintest.Resize(t, model, 60, 20)

	for _, line := range lines(model) {
		if width := len([]rune(line)); width > 60 {
			t.Fatalf("line is %d columns wide: %q", width, line)
		}
	}
	if view := screenText(model); !strings.Contains(view, "…") || !strings.Contains(view, long.group) {
		t.Fatalf("the long name should be cut with an ellipsis and the next column kept:\n%s", view)
	}
}

func TestTableKeepsTheFlexibleColumnAtItsMinimumWidthInANarrowTerminal(t *testing.T) {
	long := fruit("long-key", strings.Repeat("x", 100))
	model := modelForBoard(t, groceryBoard(newRowSource(long)))

	plugintest.Resize(t, model, 30, 20)

	row := tableLines(model)[1]
	if count := strings.Count(row, "x"); count < minimumFlexibleWidth-1 {
		t.Fatalf("row %q keeps only %d letters of the flexible column", row, count)
	}
}

func TestTableCutsARowAtTheTerminalWidthWhenNoColumnIsFlexible(t *testing.T) {
	board := groceryBoard(newRowSource(fruit("long-key", strings.Repeat("x", 100))))
	board.Columns[0].Flexible = false
	model := modelForBoard(t, board)

	plugintest.Resize(t, model, 40, 20)

	for _, line := range lines(model) {
		if width := len([]rune(line)); width > 40 {
			t.Fatalf("line is %d columns wide: %q", width, line)
		}
	}
	if strings.Contains(tableLines(model)[1], "fruit") {
		t.Fatalf("the unshrunk first column should push the others off the screen: %q", tableLines(model)[1])
	}
}

func TestCursorMovesWithEitherPairOfKeysAndStopsAtTheFirstAndLastRow(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, groceryBoard(newRowSource(rows...)))

	plugintest.Press(t, model, "up", "k")
	atTop := selectedName(t, model)
	plugintest.Press(t, model, "down", "j")
	inMiddleOfDescent := selectedName(t, model)
	plugintest.Press(t, model, "j", "down")
	atBottom := selectedName(t, model)

	first, last := namesOf(rows[0])[0], namesOf(rows[len(rows)-1])[0]
	if atTop != first || inMiddleOfDescent != last || atBottom != last {
		t.Fatalf("selected %q at the top, %q after moving down twice, %q at the bottom", atTop, inMiddleOfDescent, atBottom)
	}
}

func TestCursorJumpsWithHomeAndEnd(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, groceryBoard(newRowSource(rows...)))

	plugintest.Press(t, model, "end")
	atEnd := selectedName(t, model)
	plugintest.Press(t, model, "home")
	atHome := selectedName(t, model)

	if atEnd != namesOf(rows[len(rows)-1])[0] || atHome != namesOf(rows[0])[0] {
		t.Fatalf("selected %q after end and %q after home", atEnd, atHome)
	}
}

// manyRows are enough rows to need scrolling in a short terminal.
func manyRows(count int) []Row {
	var rows []Row
	for index := range count {
		rows = append(rows, fruit(fmt.Sprintf("key-%02d", index), fmt.Sprintf("fruit%02d", index)))
	}
	return rows
}

func TestPagingMovesTheCursorByTheRowsThatFitAndScrollsTheTable(t *testing.T) {
	rows := manyRows(40)
	model := modelForBoard(t, groceryBoard(newRowSource(rows...)))
	plugintest.Resize(t, model, 80, 14)
	pageSize := model.tableRows()

	plugintest.Press(t, model, "pgdown")

	expected := rows[pageSize].(fakeRow).name
	if selectedName(t, model) != expected || !strings.Contains(screenText(model), expected) {
		t.Fatalf("selected %q, expected %q on screen:\n%s", selectedName(t, model), expected, screenText(model))
	}
	if strings.Contains(screenText(model), rows[0].(fakeRow).name) {
		t.Fatalf("the first row should have scrolled out of view:\n%s", screenText(model))
	}

	plugintest.Press(t, model, "pgup")

	if selectedName(t, model) != rows[0].(fakeRow).name {
		t.Fatalf("selected %q after paging back", selectedName(t, model))
	}
}

func TestTableHasAsManyLinesAsTheTerminalHasHeight(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(manyRows(40)...)))

	plugintest.Resize(t, model, 80, 14)

	if count := len(lines(model)); count != 14 {
		t.Fatalf("the view has %d lines, expected 14", count)
	}
}

func TestCursorStaysOnTheSameRowWhenARefreshReordersTheRows(t *testing.T) {
	apple, broccoli, cherry := fruit("apple-key", "apple"), vegetable("broccoli-key", "broccoli"), fruit("cherry-key", "cherry")
	source := newRowSource(apple, broccoli, cherry)
	model := modelForBoard(t, groceryBoard(source))
	plugintest.Press(t, model, "down")
	source.set(cherry, apple, broccoli)

	plugintest.Press(t, model, "r")

	if selectedName(t, model) != broccoli.name {
		t.Fatalf("selected %q, expected %q", selectedName(t, model), broccoli.name)
	}
}

func TestCursorKeepsItsPositionWhenARefreshRemovesTheSelectedRow(t *testing.T) {
	rows := groceries()
	source := newRowSource(rows...)
	model := modelForBoard(t, groceryBoard(source))
	plugintest.Press(t, model, "down")
	source.set(rows[0], rows[2])

	plugintest.Press(t, model, "r")

	if selectedName(t, model) != rows[2].(fakeRow).name {
		t.Fatalf("selected %q", selectedName(t, model))
	}
}

func TestCursorStaysOnTheSameRowWhenAFilterRemovesRowsAboveIt(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, groceryBoard(newRowSource(rows...)))
	plugintest.Press(t, model, "end")

	plugintest.Press(t, model, "f", "2")

	if selectedName(t, model) != rows[2].(fakeRow).name {
		t.Fatalf("selected %q, expected %q", selectedName(t, model), rows[2].(fakeRow).name)
	}
}

func TestStatusLineCountsVisibleRowsAgainstAllRows(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, groceryBoard(newRowSource(rows...)))

	plugintest.Press(t, model, "f", "2")

	expected := fmt.Sprintf("%d of %d groceries", len(visibleNames(model)), len(rows))
	if status := lines(model)[len(lines(model))-2]; !strings.Contains(status, expected) {
		t.Fatalf("status line = %q, expected it to contain %q", status, expected)
	}
}

func TestStatusLineShowsWhenTheBoardLoadedAndHowOftenItRefreshes(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))

	status := lines(model)[len(lines(model))-2]

	loaded := model.current().loadedAt.Format("15:04")
	expected := "updated " + loaded + " · refreshes every " + intervalText(model.refreshInterval)
	if !strings.Contains(status, expected) {
		t.Fatalf("status line = %q, expected it to contain %q", status, expected)
	}
}

func TestStatusLineSaysLoadingUntilTheFirstLoadFinishes(t *testing.T) {
	host, err := buildHost(t, `plugins = ["fake"]`, boardPlugin("fake", groceryBoard(newRowSource(groceries()...))))
	if err != nil {
		t.Fatal(err)
	}

	model := host.Model()

	if status := lines(model)[len(lines(model))-2]; !strings.Contains(status, "Loading…") {
		t.Fatalf("status line = %q", status)
	}
}

func TestIntervalTextUsesWholeMinutesWhenPossibleAndSecondsOtherwise(t *testing.T) {
	cases := map[time.Duration]string{
		time.Second:      "second",
		time.Minute:      "minute",
		7 * time.Minute:  "7 minutes",
		90 * time.Second: "90 seconds",
	}

	for interval, expected := range cases {
		if text := intervalText(interval); text != expected {
			t.Errorf("intervalText(%v) = %q, expected %q", interval, text, expected)
		}
	}
}

func TestRefreshKeyReloadsTheBoardAndClearsItsNotice(t *testing.T) {
	source := newRowSource(groceries()...)
	model := modelForBoard(t, groceryBoard(source))
	before := source.loadCount()

	_, command := model.Update(plugintest.KeyMessage("r"))

	if status := lines(model)[len(lines(model))-2]; !strings.Contains(status, "Refreshing…") {
		t.Fatalf("status line = %q while refreshing", status)
	}
	plugintest.Settle(t, model, command)
	if source.loadCount() != before+1 || strings.Contains(screenText(model), "Refreshing…") {
		t.Fatalf("loaded %d times after %d, view:\n%s", source.loadCount(), before, screenText(model))
	}
}

func TestRefreshTickReloadsEveryBoardAndStartsTheNextTimer(t *testing.T) {
	first, second := newRowSource(groceries()...), newRowSource(groceries()...)
	otherBoard := groceryBoard(second)
	otherBoard.Name, otherBoard.Title = "other", "Other"
	model := modelFor(t, boardPlugin("one", groceryBoard(first)), boardPlugin("two", otherBoard))
	var timers []time.Duration
	model.startTimer = func(interval time.Duration) tea.Cmd {
		timers = append(timers, interval)
		return nil
	}
	before := first.loadCount()

	plugintest.Send(t, model, refreshTickMessage{})

	if first.loadCount() != before+1 || second.loadCount() != before+1 {
		t.Fatalf("loads = %d and %d after %d", first.loadCount(), second.loadCount(), before)
	}
	if !reflect.DeepEqual(timers, []time.Duration{model.refreshInterval}) {
		t.Fatalf("timers started = %v", timers)
	}
}

func TestQuitKeyAndCtrlCQuit(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))

	if !plugintest.Quits(model, "q") || !plugintest.Quits(model, "ctrl+c") {
		t.Fatalf("q and ctrl+c should quit")
	}
	if plugintest.Quits(model, "j") {
		t.Fatalf("j should not quit")
	}
}

func TestHelpListsActionsInRegistrationOrderThenFiltersRefreshAndQuit(t *testing.T) {
	board := groceryBoard(newRowSource(groceries()...))
	board.Actions = []Action{{Key: "x", Label: "first", Run: doNothing}, {Key: "y", Label: "second", Run: doNothing}}
	late := fakePlugin{name: "late", setup: func(host *Host) error {
		host.AddAction("groceries", Action{Key: "z", Label: "third", Run: doNothing})
		return nil
	}}
	model := modelFor(t, boardPlugin("owner", board), late)

	help := lines(model)[len(lines(model))-1]

	expected := "x first · y second · z third · f filters · r refresh · q quit"
	if help != expected {
		t.Fatalf("help = %q, expected %q", help, expected)
	}
}

func TestHelpHidesAnActionWithoutALabelButStillRunsIt(t *testing.T) {
	board := groceryBoard(newRowSource(groceries()...))
	ran := false
	board.Actions = []Action{{Key: "x", Run: func(Selection) Effect {
		ran = true
		return Effect{}
	}}}
	model := modelForBoard(t, board)

	plugintest.Press(t, model, "x")

	if help := lines(model)[len(lines(model))-1]; strings.Contains(help, "x ") || !ran {
		t.Fatalf("help = %q, ran = %v", help, ran)
	}
}

func TestHelpOffersNoFiltersForABoardWithoutFacets(t *testing.T) {
	board := groceryBoard(newRowSource(groceries()...))
	board.Facets = nil
	model := modelForBoard(t, board)

	help := lines(model)[len(lines(model))-1]

	if strings.Contains(help, "filters") || !reflect.DeepEqual(strings.Fields(help), strings.Fields("r refresh · q quit")) {
		t.Fatalf("help = %q", help)
	}
}
