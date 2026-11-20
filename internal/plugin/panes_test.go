package plugin

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

const sectionsTab = "Sections"

// sectionBoard is a groceries board that shares the sections tab with the other section boards.
func sectionBoard(name, title string, source *rowSource) Board {
	board := groceryBoard(source)
	board.Name, board.Title, board.RowNoun, board.Tab = name, title, name+" items", sectionsTab
	return board
}

// boardsPlugin registers every given board from one plugin.
func boardsPlugin(boards ...Board) fakePlugin {
	return fakePlugin{name: "sections", setup: func(host *Host) error {
		for _, board := range boards {
			host.AddBoard(board)
		}
		return nil
	}}
}

// threeSections is a model with the boards fresh, frozen, and canned in one tab.
func threeSections(t *testing.T, width, height int) *Model {
	t.Helper()
	model := modelFor(t, boardsPlugin(
		sectionBoard("fresh", "Fresh", newRowSource(groceries()...)),
		sectionBoard("frozen", "Frozen", newRowSource(groceries()...)),
		sectionBoard("canned", "Canned", newRowSource(groceries()...)),
	))
	plugintest.Resize(t, model, width, height)
	return model
}

// soloBoard is a groceries board that is a tab of its own.
func soloBoard() Board {
	board := groceryBoard(newRowSource(groceries()...))
	board.Name, board.Title = "solo", "Solo"
	return board
}

func focusedTitle(model *Model) string { return model.current().spec.Title }

func TestBoardsWithTheSameTabShareOneTabAtTheFirstOfTheirPositions(t *testing.T) {
	solo := soloBoard()
	model := modelFor(t, boardsPlugin(
		sectionBoard("fresh", "Fresh", newRowSource(groceries()...)),
		solo,
		sectionBoard("frozen", "Frozen", newRowSource(groceries()...)),
	))

	if len(model.tabs) != 2 || model.tabs[0].title != sectionsTab || model.tabs[1].title != "Solo" {
		t.Fatalf("tabs = %+v", model.tabs)
	}
	if len(model.tabs[0].boards) != 2 || model.tabs[0].boards[0] != 0 || model.tabs[0].boards[1] != 2 {
		t.Fatalf("boards of the shared tab = %v", model.tabs[0].boards)
	}
}

func TestTabBarListsTabsAndNotTheBoardsInsideThem(t *testing.T) {
	solo := soloBoard()
	model := modelFor(t, boardsPlugin(
		sectionBoard("fresh", "Fresh", newRowSource(groceries()...)),
		sectionBoard("frozen", "Frozen", newRowSource(groceries()...)),
		solo,
	))

	first := lines(model)[0]

	if !strings.Contains(first, sectionsTab) || !strings.Contains(first, "Solo") {
		t.Fatalf("first line = %q, expected both tabs", first)
	}
	if strings.Contains(first, "Fresh") || strings.Contains(first, "Frozen") {
		t.Fatalf("first line = %q, the boards of a tab are not tabs", first)
	}
}

func TestTabKeyReturnsToTheBoardThatHadTheFocusInEachTab(t *testing.T) {
	solo := soloBoard()
	model := modelFor(t, boardsPlugin(
		sectionBoard("fresh", "Fresh", newRowSource(groceries()...)),
		sectionBoard("frozen", "Frozen", newRowSource(groceries()...)),
		solo,
	))

	plugintest.Press(t, model, "right", "tab")
	onSolo := focusedTitle(model)
	plugintest.Press(t, model, "tab")

	if onSolo != "Solo" || focusedTitle(model) != "Frozen" {
		t.Fatalf("focus after tab = %q, after tab again = %q", onSolo, focusedTitle(model))
	}
}

func TestRightAndLMoveTheFocusToTheNextBoardOfTheTab(t *testing.T) {
	for _, key := range []string{"right", "l"} {
		model := threeSections(t, 200, 40)

		plugintest.Press(t, model, key)

		if focusedTitle(model) != "Frozen" {
			t.Errorf("after %q the focus is on %q", key, focusedTitle(model))
		}
	}
}

func TestLeftAndHMoveTheFocusToThePreviousBoardOfTheTab(t *testing.T) {
	for _, key := range []string{"left", "h"} {
		model := threeSections(t, 200, 40)
		plugintest.Press(t, model, "right", "right")

		plugintest.Press(t, model, key)

		if focusedTitle(model) != "Frozen" {
			t.Errorf("after %q the focus is on %q", key, focusedTitle(model))
		}
	}
}

func TestFocusStopsAtTheFirstAndLastBoardOfTheTab(t *testing.T) {
	model := threeSections(t, 200, 40)

	plugintest.Press(t, model, "left", "h")
	atStart := focusedTitle(model)
	plugintest.Press(t, model, "right", "right", "right", "l")

	if atStart != "Fresh" || focusedTitle(model) != "Canned" {
		t.Fatalf("focus at the start = %q, at the end = %q", atStart, focusedTitle(model))
	}
}

func TestFocusKeysDoNothingInATabWithOneBoard(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))
	before := screenText(model)

	plugintest.Press(t, model, "left", "right", "h", "l")

	if screenText(model) != before {
		t.Fatalf("the view changed:\n%s", screenText(model))
	}
}

func TestActionRunsOnTheSelectedRowOfTheFocusedBoard(t *testing.T) {
	var ranOn []string
	record := func(title string) []Action {
		return []Action{{Key: "x", Run: func(selection Selection) Effect {
			ranOn = append(ranOn, title+":"+selection.Row.Key())
			return Effect{}
		}}}
	}
	first := sectionBoard("fresh", "Fresh", newRowSource(groceries()...))
	first.Actions = record("Fresh")
	second := sectionBoard("frozen", "Frozen", newRowSource(vegetable("peas-key", "peas"), vegetable("corn-key", "corn")))
	second.Actions = record("Frozen")
	model := modelFor(t, boardsPlugin(first, second))

	plugintest.Press(t, model, "right", "down", "x")

	if len(ranOn) != 1 || ranOn[0] != "Frozen:corn-key" {
		t.Fatalf("the action ran on %v", ranOn)
	}
}

func TestRefreshKeyReloadsOnlyTheFocusedBoardOfATab(t *testing.T) {
	sources := []*rowSource{newRowSource(groceries()...), newRowSource(groceries()...), newRowSource(groceries()...)}
	model := modelFor(t, boardsPlugin(
		sectionBoard("fresh", "Fresh", sources[0]),
		sectionBoard("frozen", "Frozen", sources[1]),
		sectionBoard("canned", "Canned", sources[2]),
	))
	before := []int{sources[0].loadCount(), sources[1].loadCount(), sources[2].loadCount()}

	plugintest.Press(t, model, "right", "r")

	for index, source := range sources {
		expected := before[index]
		if index == 1 {
			expected++
		}
		if source.loadCount() != expected {
			t.Errorf("board %d loaded %d times, expected %d", index, source.loadCount(), expected)
		}
	}
}

func TestEachBoardOfATabKeepsItsOwnCursor(t *testing.T) {
	model := threeSections(t, 200, 40)

	plugintest.Press(t, model, "down", "right", "down", "down", "left")

	if selectedName(t, model) != groceries()[1].(fakeRow).name {
		t.Fatalf("the first board's cursor is on %q", selectedName(t, model))
	}
	plugintest.Press(t, model, "right")
	if selectedName(t, model) != groceries()[2].(fakeRow).name {
		t.Fatalf("the second board's cursor is on %q", selectedName(t, model))
	}
}

func rowsOfPanes(panes []pane) int {
	rows := map[int]bool{}
	for _, shown := range panes {
		rows[shown.y] = true
	}
	return len(rows)
}

func TestThreeBoardsSitInOneRowWhenThreePanesFit(t *testing.T) {
	model := threeSections(t, 3*minimumPaneWidth+2, 40)

	panes := model.panesOf(0)

	if len(panes) != 3 || rowsOfPanes(panes) != 1 {
		t.Fatalf("panes = %+v", panes)
	}
}

func TestThreeBoardsWrapToTwoRowsWhenOnlyTwoPanesFit(t *testing.T) {
	model := threeSections(t, 2*minimumPaneWidth, 40)

	panes := model.panesOf(0)

	if rowsOfPanes(panes) != 2 || panes[0].y != panes[1].y || panes[2].y == panes[0].y {
		t.Fatalf("panes = %+v", panes)
	}
	if panes[2].width != model.width {
		t.Fatalf("the lone pane of the last row is %d wide, expected %d", panes[2].width, model.width)
	}
}

func TestThreeBoardsStackInOneColumnWhenOnlyOnePaneFits(t *testing.T) {
	model := threeSections(t, minimumPaneWidth+30, 40)

	if rowsOfPanes(model.panesOf(0)) != 3 {
		t.Fatalf("panes = %+v", model.panesOf(0))
	}
}

func TestPanesOfARowFillTheWidthWithOneColumnForEachSeparator(t *testing.T) {
	model := threeSections(t, 200, 40)

	total := len(model.panesOf(0)) - 1
	for _, shown := range model.panesOf(0) {
		total += shown.width
	}

	if total != model.width {
		t.Fatalf("panes and separators take %d columns of %d", total, model.width)
	}
}

func TestPanesOfRowsFillTheHeightBetweenTheLinesAboveAndBelow(t *testing.T) {
	model := threeSections(t, 2*minimumPaneWidth, 41)

	panes := model.panesOf(0)

	if panes[0].y+panes[0].height != panes[2].y || panes[2].y+panes[2].height != model.height-linesBelowPanes {
		t.Fatalf("panes = %+v for a height of %d", panes, model.height)
	}
}

func TestEveryBoardTitleIsOnTheFirstPaneLine(t *testing.T) {
	model := threeSections(t, 200, 50)

	first := lines(model)[0]

	for _, title := range []string{"Fresh", "Frozen", "Canned"} {
		if !strings.Contains(first, title) {
			t.Fatalf("first line = %q, expected the title %q", first, title)
		}
	}
}

func TestViewWithSidePanesFillsTheHeightOfTheTerminal(t *testing.T) {
	model := threeSections(t, 200, 50)

	if len(lines(model)) != model.height {
		t.Fatalf("the view has %d lines, expected %d", len(lines(model)), model.height)
	}
}

func TestEveryLineOfAViewWithSidePanesIsAsWideAsTheTerminal(t *testing.T) {
	model := threeSections(t, 200, 40)

	for index, line := range lines(model)[:len(lines(model))-2] {
		if width := len([]rune(line)); width != model.width {
			t.Fatalf("line %d is %d columns wide: %q", index, width, line)
		}
	}
}

func TestHelpLineOffersSectionKeysOnlyInATabWithSeveralBoards(t *testing.T) {
	several := threeSections(t, 200, 40)
	single := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))

	if !strings.Contains(helpLine(several), "←/→ section") || strings.Contains(helpLine(single), "section") {
		t.Fatalf("help lines = %q and %q", helpLine(several), helpLine(single))
	}
}

func TestStatusLineIsOfTheFocusedBoard(t *testing.T) {
	model := threeSections(t, 200, 40)

	plugintest.Press(t, model, "right")

	if !strings.Contains(statusLine(model), "frozen items") {
		t.Fatalf("status line = %q", statusLine(model))
	}
}

func TestFilterMenuOpensForTheFocusedBoardAndShrinksThePanes(t *testing.T) {
	model := threeSections(t, 200, 40)
	heightBefore := model.paneOf(1).height

	plugintest.Press(t, model, "right", "f", "2")

	if len(model.boards[1].visible) >= len(model.boards[0].visible) {
		t.Fatalf("the menu changed the wrong board's filter")
	}
	if model.paneOf(1).height >= heightBefore || !strings.Contains(screenText(model), "Filters") {
		t.Fatalf("pane height %d, was %d:\n%s", model.paneOf(1).height, heightBefore, screenText(model))
	}
}

// paneCellPosition is where a cell of a board's pane is drawn.
func paneCellPosition(model *Model, boardIndex, rowIndex, column int) (x, y int) {
	shown, board := model.paneOf(boardIndex), model.boards[boardIndex]
	x = shown.x
	for _, width := range columnWidths(board.spec.Columns, board.cells(), shown.width)[:column] {
		x += width + len(columnGap)
	}
	return x, shown.headerLine() + 1 + rowIndex - board.offset
}

func TestClickInAnotherPaneFocusesItsBoardAndSelectsTheClickedRow(t *testing.T) {
	replaceClock(t)
	model := threeSections(t, 200, 40)
	x, y := paneCellPosition(model, 1, 2, 0)

	click(t, model, x, y)

	if focusedTitle(model) != "Frozen" || selectedName(t, model) != groceries()[2].(fakeRow).name {
		t.Fatalf("focus on %q, selected %q", focusedTitle(model), selectedName(t, model))
	}
	if model.boards[0].cursor != 0 {
		t.Fatalf("the first board's cursor moved to %d", model.boards[0].cursor)
	}
}

func TestClickOnTheTitleOfAnotherPaneFocusesItWithoutMovingItsCursor(t *testing.T) {
	model := threeSections(t, 200, 40)
	shown := model.paneOf(2)

	click(t, model, shown.x, shown.y)

	if focusedTitle(model) != "Canned" || model.boards[2].cursor != 0 {
		t.Fatalf("focus on %q, cursor %d", focusedTitle(model), model.boards[2].cursor)
	}
}

func TestDoubleClickInAnotherPaneCopiesThatPanesCell(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	model := threeSections(t, 200, 40)
	expected := model.boards[1].visible[1].(fakeRow).group
	x, y := paneCellPosition(model, 1, 1, 1)

	click(t, model, x, y)
	click(t, model, x, y)

	if len(*copied) != 1 || (*copied)[0] != expected {
		t.Fatalf("copied = %v, expected %q", *copied, expected)
	}
}

func TestTwoQuickClicksOnTheSameCellOfDifferentPanesCopyNothing(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	model := threeSections(t, 200, 40)
	firstX, firstY := paneCellPosition(model, 0, 1, 0)
	secondX, secondY := paneCellPosition(model, 1, 1, 0)

	click(t, model, firstX, firstY)
	click(t, model, secondX, secondY)

	if len(*copied) != 0 {
		t.Fatalf("copied = %v", *copied)
	}
}

func TestWheelScrollsThePaneUnderThePointerAndKeepsTheFocus(t *testing.T) {
	model := threeSections(t, 200, 40)
	x, y := paneCellPosition(model, 2, 0, 0)

	plugintest.Send(t, model, tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})

	if model.boards[2].cursor != 1 || model.boards[0].cursor != 0 || focusedTitle(model) != "Fresh" {
		t.Fatalf("cursors = %d, %d, %d; focus on %q", model.boards[0].cursor, model.boards[1].cursor, model.boards[2].cursor, focusedTitle(model))
	}
}

func TestEachPaneKeepsItsOwnCursorInViewWithItsOwnRowCount(t *testing.T) {
	model := modelFor(t, boardsPlugin(
		sectionBoard("fresh", "Fresh", newRowSource(manyRows(40)...)),
		sectionBoard("frozen", "Frozen", newRowSource(manyRows(40)...)),
	))
	plugintest.Resize(t, model, 130, 20)

	plugintest.Press(t, model, "right", "end")

	last := model.boards[1]
	if last.offset+model.paneOf(1).tableRows() != len(last.visible) || model.boards[0].offset != 0 {
		t.Fatalf("offsets = %d and %d", model.boards[0].offset, last.offset)
	}
}

func boardsWithOwnInterval(t *testing.T, interval time.Duration) (*Model, *rowSource, *rowSource) {
	t.Helper()
	quick, ordinary := newRowSource(groceries()...), newRowSource(groceries()...)
	quickBoard := sectionBoard("quick", "Quick", quick)
	quickBoard.RefreshInterval = interval
	return modelFor(t, boardsPlugin(quickBoard, sectionBoard("ordinary", "Ordinary", ordinary))), quick, ordinary
}

func TestStatusLineShowsTheIntervalOfTheFocusedBoard(t *testing.T) {
	model, _, _ := boardsWithOwnInterval(t, 30*time.Second)

	onQuick := statusLine(model)
	plugintest.Press(t, model, "right")

	if !strings.Contains(onQuick, "refreshes every 30 seconds") || !strings.Contains(statusLine(model), "refreshes every "+intervalText(model.refreshInterval)) {
		t.Fatalf("status lines = %q and %q", onQuick, statusLine(model))
	}
}

func TestPrintOnceTitlesBoardsOfATabWithTheTabAndTheirOwnTitle(t *testing.T) {
	output, err := printOnce(t, 100, boardsPlugin(
		sectionBoard("fresh", "Fresh", newRowSource(groceries()...)),
		sectionBoard("frozen", "Frozen", newRowSource(groceries()...)),
	))

	if err != nil || !strings.Contains(output, sectionsTab+" · Fresh") || !strings.Contains(output, sectionsTab+" · Frozen") {
		t.Fatalf("err = %v, output:\n%s", err, output)
	}
}

func TestBoardWithANegativeRefreshIntervalIsRejected(t *testing.T) {
	board := groceryBoard(newRowSource(groceries()...))
	board.RefreshInterval = -time.Second

	_, err := buildHost(t, `plugins = ["fake"]`, boardPlugin("fake", board))

	if err == nil || !strings.Contains(err.Error(), "RefreshInterval") {
		t.Fatalf("err = %v", err)
	}
}

func TestActionsOnTheFocusKeysAreRejected(t *testing.T) {
	for _, key := range []string{"left", "right", "h", "l"} {
		board := groceryBoard(newRowSource(groceries()...))
		board.Actions = []Action{{Key: key, Run: doNothing}}

		_, err := buildHost(t, `plugins = ["fake"]`, boardPlugin("fake", board))

		if err == nil || !strings.Contains(err.Error(), "key the board uses itself") {
			t.Errorf("action on %q: err = %v", key, err)
		}
	}
}

func TestEachPaneStartsOneColumnAfterTheSeparatorDrawnBeforeIt(t *testing.T) {
	model := threeSections(t, 200, 40)

	screen := lines(model)

	for boardIndex := 1; boardIndex < 3; boardIndex++ {
		shown := model.paneOf(boardIndex)
		drawn := []rune(screen[shown.headerLine()])
		if string(drawn[shown.x-1]) != paneSeparator {
			t.Fatalf("pane %d starts at column %d, but column %d is %q:\n%s", boardIndex, shown.x, shown.x-1, string(drawn[shown.x-1]), screenText(model))
		}
	}
}
