package plugin

import (
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// twoBoards is a groceries board and a second board of its own rows, one plugin each.
func twoBoards(first, second *rowSource) []Plugin {
	other := groceryBoard(second)
	other.Name, other.Title, other.RowNoun = "pantry", "Pantry", "pantry items"
	return []Plugin{boardPlugin("shop", groceryBoard(first)), boardPlugin("larder", other)}
}

func TestSingleBoardHasNoTabBar(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))

	if first := lines(model)[0]; strings.Contains(first, "Groceries") || !strings.Contains(first, "Groups") {
		t.Fatalf("first line = %q, expected the filter summary", first)
	}
}

func TestTabBarIsTheFirstLineAndListsEveryBoardWhenThereAreTwo(t *testing.T) {
	model := modelFor(t, twoBoards(newRowSource(groceries()...), newRowSource(groceries()...))...)

	first := lines(model)[0]

	for _, title := range []string{"Groceries", "Pantry"} {
		if !strings.Contains(first, title) {
			t.Fatalf("first line = %q, expected the tab %q", first, title)
		}
	}
}

func TestTabKeySwitchesToTheNextBoardAndWrapsAround(t *testing.T) {
	model := modelFor(t, twoBoards(newRowSource(groceries()...), newRowSource(groceries()...))...)

	plugintest.Press(t, model, "tab")
	onSecond := statusLine(model)
	plugintest.Press(t, model, "tab")
	backOnFirst := statusLine(model)

	if !strings.Contains(onSecond, "pantry items") || !strings.Contains(backOnFirst, "groceries") {
		t.Fatalf("status lines = %q then %q", onSecond, backOnFirst)
	}
}

func TestShiftTabSwitchesToThePreviousBoardAndWrapsAround(t *testing.T) {
	model := modelFor(t, twoBoards(newRowSource(groceries()...), newRowSource(groceries()...))...)

	plugintest.Press(t, model, "shift+tab")

	if status := statusLine(model); !strings.Contains(status, "pantry items") {
		t.Fatalf("status line = %q", status)
	}
}

func TestTabKeyDoesNothingWithOneBoard(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))
	before := screenText(model)

	plugintest.Press(t, model, "tab", "shift+tab")

	if screenText(model) != before {
		t.Fatalf("the view changed:\n%s", screenText(model))
	}
}

func TestEachBoardKeepsItsOwnCursorAndFilters(t *testing.T) {
	model := modelFor(t, twoBoards(newRowSource(groceries()...), newRowSource(groceries()...))...)
	plugintest.Press(t, model, "down", "tab")
	onSecondAtStart := selectedName(t, model)
	plugintest.Press(t, model, "f", "2", "esc", "tab")

	if selectedName(t, model) != "broccoli" || len(visibleNames(model)) != len(groceries()) {
		t.Fatalf("first board: selected %q, shows %v", selectedName(t, model), visibleNames(model))
	}
	plugintest.Press(t, model, "tab")
	if onSecondAtStart != "apple" || len(visibleNames(model)) != 2 {
		t.Fatalf("second board: started on %q, shows %v", onSecondAtStart, visibleNames(model))
	}
}

func TestRefreshKeyReloadsOnlyTheBoardInView(t *testing.T) {
	first, second := newRowSource(groceries()...), newRowSource(groceries()...)
	model := modelFor(t, twoBoards(first, second)...)
	firstBefore, secondBefore := first.loadCount(), second.loadCount()

	plugintest.Press(t, model, "tab", "r")

	if first.loadCount() != firstBefore || second.loadCount() != secondBefore+1 {
		t.Fatalf("the first board loaded %d more times, the second %d", first.loadCount()-firstBefore, second.loadCount()-secondBefore)
	}
}

func TestActionsOfAnotherBoardAreNotAvailable(t *testing.T) {
	ran := false
	first := groceryBoard(newRowSource(groceries()...))
	first.Actions = []Action{{Key: "x", Run: func(Selection) Effect {
		ran = true
		return Effect{}
	}}}
	second := groceryBoard(newRowSource(groceries()...))
	second.Name, second.Title = "pantry", "Pantry"
	model := modelFor(t, boardPlugin("shop", first), boardPlugin("larder", second))

	plugintest.Press(t, model, "tab", "x")

	if ran {
		t.Fatalf("an action of the first board ran from the second")
	}
}

func TestTableHasOneLessRowWhenTheTabBarTakesALine(t *testing.T) {
	single := modelForBoard(t, groceryBoard(newRowSource(manyRows(40)...)))
	tabbed := modelFor(t, twoBoards(newRowSource(manyRows(40)...), newRowSource(manyRows(40)...))...)

	plugintest.Resize(t, single, 80, 20)
	plugintest.Resize(t, tabbed, 80, 20)

	if len(lines(single)) != 20 || len(lines(tabbed)) != 20 {
		t.Fatalf("the views have %d and %d lines, expected 20", len(lines(single)), len(lines(tabbed)))
	}
}
