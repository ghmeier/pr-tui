package plugin

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

func boardWithFacets(rows []Row, facets ...Facet) Board {
	board := groceryBoard(newRowSource(rows...))
	board.Facets = facets
	return board
}

func TestMultipleFacetStartsWithEveryOptionSelected(t *testing.T) {
	rows := groceries()

	model := modelForBoard(t, boardWithFacets(rows, groupFacet()))

	if !reflect.DeepEqual(visibleNames(model), namesOf(rows...)) {
		t.Fatalf("visible = %v", visibleNames(model))
	}
}

func TestSingleFacetStartsWithItsFirstOptionSelected(t *testing.T) {
	rows := groceries()
	facet := sizeFacet("s")

	model := modelForBoard(t, boardWithFacets(rows, facet))

	for _, row := range model.current().visible {
		if sizeOf(row.(fakeRow)) != facet.Options[0].Value {
			t.Fatalf("%v is shown but is not %s", row, facet.Options[0].Value)
		}
	}
}

func TestFacetStartsWithItsInitialSelectionWhenOneIsGiven(t *testing.T) {
	rows := groceries()
	facet := groupFacet()
	facet.Selected = []string{"vegetable"}

	model := modelForBoard(t, boardWithFacets(rows, facet))

	if !reflect.DeepEqual(visibleNames(model), namesOf(rows[1])) {
		t.Fatalf("visible = %v", visibleNames(model))
	}
}

func TestSingleFacetStartsWithItsInitialSelectionWhenOneIsGiven(t *testing.T) {
	rows := groceries()
	facet := sizeFacet("s")
	facet.Selected = []string{"large"}

	model := modelForBoard(t, boardWithFacets(rows, facet))

	for _, row := range model.current().visible {
		if sizeOf(row.(fakeRow)) != "large" {
			t.Fatalf("%v is shown but is not large", row)
		}
	}
	if len(model.current().visible) == 0 {
		t.Fatalf("the fixture should have a large row")
	}
}

func TestSummaryLineShowsEveryOptionOfAMultipleFacetAndOnlyTheChosenOptionOfASingleFacet(t *testing.T) {
	model := modelForBoard(t, boardWithFacets(groceries(), groupFacet(), sizeFacet("s")))

	plugintest.Press(t, model, "f", "2")

	expected := "Groups: ● fruit  ○ vegetable    Sizes: small"
	if summary := lines(model)[0]; summary != expected {
		t.Fatalf("summary = %q, expected %q", summary, expected)
	}
}

func TestFilterKeyOpensTheMenuAndEscapeEnterAndFCloseIt(t *testing.T) {
	for _, closing := range []string{"esc", "enter", "f"} {
		model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))

		plugintest.Press(t, model, "f")
		opened := strings.Contains(screenText(model), "Filters")
		plugintest.Press(t, model, closing)

		if !opened || strings.Contains(screenText(model), "Filters") {
			t.Fatalf("menu opened = %v, still shown after %s = %v", opened, closing, strings.Contains(screenText(model), "Filters"))
		}
	}
}

func TestNumberKeysDoNothingWhileTheMenuIsClosed(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, groceryBoard(newRowSource(rows...)))

	plugintest.Press(t, model, "1", "2")

	if !reflect.DeepEqual(visibleNames(model), namesOf(rows...)) {
		t.Fatalf("visible = %v", visibleNames(model))
	}
}

func TestMenuGroupsTheOptionsOfEachFacetUnderItsTitle(t *testing.T) {
	model := modelForBoard(t, boardWithFacets(groceries(), groupFacet(), sizeFacet("s")))

	plugintest.Press(t, model, "f")

	// The summary line above the menu mentions the same words, so the search starts below it.
	menu := lines(model)[2:]
	positions := []int{}
	for _, word := range []string{"Groups", "fruit", "vegetable", "Sizes", "small", "large"} {
		positions = append(positions, slices.IndexFunc(menu, func(line string) bool { return strings.Contains(line, word) }))
	}
	if !slices.IsSorted(positions) || slices.Contains(positions, -1) {
		t.Fatalf("positions = %v in:\n%s", positions, strings.Join(menu, "\n"))
	}
}

func TestMenuMarksOnlyTheRowUnderTheCursor(t *testing.T) {
	facet := groupFacet()
	model := modelForBoard(t, boardWithFacets(groceries(), facet))

	plugintest.Press(t, model, "f", "down")

	for index, option := range facet.Options {
		var row string
		for _, line := range lines(model) {
			if strings.Contains(line, option.Label) && strings.Contains(line, "●") && strings.Contains(line, "  ") {
				row = line
			}
		}
		if marked := strings.Contains(row, "▌"); marked != (index == 1) {
			t.Fatalf("row %q marked = %v, cursor is on option 1", row, marked)
		}
	}
}

func TestMenuCursorMovesWithEitherPairOfKeysAndStopsAtTheEnds(t *testing.T) {
	model := modelForBoard(t, boardWithFacets(groceries(), groupFacet(), sizeFacet("s")))
	last := len(groupFacet().Options) + len(sizeFacet("s").Options) - 1

	plugintest.Press(t, model, "f", "up", "k")
	atTop := model.current().menuCursor
	plugintest.Press(t, model, "down", "j")
	afterTwo := model.current().menuCursor
	plugintest.Press(t, model, slices.Repeat([]string{"down"}, 10)...)
	atBottom := model.current().menuCursor

	if atTop != 0 || afterTwo != 2 || atBottom != last {
		t.Fatalf("cursor = %d at the top, %d after two moves, %d at the bottom, expected 0, 2 and %d", atTop, afterTwo, atBottom, last)
	}
}

func TestSpaceTogglesTheOptionUnderTheCursorOfAMultipleFacet(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, boardWithFacets(rows, groupFacet()))

	plugintest.Press(t, model, "f", "down", " ")
	hidden := visibleNames(model)
	plugintest.Press(t, model, " ")

	if !reflect.DeepEqual(hidden, namesOf(rows[0], rows[2])) || !reflect.DeepEqual(visibleNames(model), namesOf(rows...)) {
		t.Fatalf("visible = %v after toggling off, %v after toggling back", hidden, visibleNames(model))
	}
}

func TestSpaceSelectsTheOptionUnderTheCursorOfASingleFacetAndDeselectsTheOthers(t *testing.T) {
	rows := groceries()
	facet := sizeFacet("s")
	model := modelForBoard(t, boardWithFacets(rows, facet))

	plugintest.Press(t, model, "f", "down", " ")
	afterSelectingLarge := slices.Clone(visibleNames(model))
	plugintest.Press(t, model, " ")

	for _, row := range model.current().visible {
		if sizeOf(row.(fakeRow)) != facet.Options[1].Value {
			t.Fatalf("%v is shown but is not %s", row, facet.Options[1].Value)
		}
	}
	if !reflect.DeepEqual(afterSelectingLarge, visibleNames(model)) || len(afterSelectingLarge) == 0 {
		t.Fatalf("selecting the same option again changed %v to %v", afterSelectingLarge, visibleNames(model))
	}
}

func TestNumberKeyTogglesTheOptionWithThatNumberWithoutMovingTheCursor(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, boardWithFacets(rows, groupFacet()))

	plugintest.Press(t, model, "f", "2")

	if !reflect.DeepEqual(visibleNames(model), namesOf(rows[0], rows[2])) || model.current().menuCursor != 0 {
		t.Fatalf("visible = %v, menu cursor = %d", visibleNames(model), model.current().menuCursor)
	}
}

func TestNumberKeysCountOptionsAcrossFacets(t *testing.T) {
	facet := sizeFacet("s")
	model := modelForBoard(t, boardWithFacets(groceries(), groupFacet(), facet))
	third := len(groupFacet().Options) + 1

	plugintest.Press(t, model, "f", fmt.Sprint(third))

	for _, row := range model.current().visible {
		if sizeOf(row.(fakeRow)) != facet.Options[0].Value {
			t.Fatalf("%v is shown after selecting %s", row, facet.Options[0].Value)
		}
	}
}

// manyOptionFacet has one option for each row of manyRows, so it has more options than the
// nine number keys reach.
func manyOptionFacet(count int) Facet {
	facet := Facet{Title: "Names", Value: func(row Row) string { return row.(fakeRow).name }}
	for _, row := range manyRows(count) {
		name := row.(fakeRow).name
		facet.Options = append(facet.Options, FacetOption{Value: name, Label: name})
	}
	return facet
}

func TestOnlyTheFirstNineOptionsAreNumbered(t *testing.T) {
	facet := manyOptionFacet(10)
	model := modelForBoard(t, boardWithFacets(manyRows(10), facet))

	plugintest.Press(t, model, "f")

	menu := screenText(model)
	for index, option := range facet.Options {
		numbered := strings.Contains(menu, fmt.Sprintf("%d  ● %s", index+1, option.Label))
		if numbered != (index < 9) {
			t.Fatalf("option %d numbered = %v:\n%s", index, numbered, menu)
		}
	}
}

func TestNumberKeyBeyondTheOptionsDoesNothing(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, boardWithFacets(rows, groupFacet()))

	plugintest.Press(t, model, "f", "9", "0")

	if !reflect.DeepEqual(visibleNames(model), namesOf(rows...)) {
		t.Fatalf("visible = %v", visibleNames(model))
	}
}

func TestSpaceTogglesAnOptionBeyondTheNumberKeys(t *testing.T) {
	facet := manyOptionFacet(11)
	rows := manyRows(11)
	model := modelForBoard(t, boardWithFacets(rows, facet))

	plugintest.Press(t, model, append([]string{"f"}, append(slices.Repeat([]string{"down"}, 10), " ")...)...)

	if slices.Contains(visibleNames(model), facet.Options[10].Value) || len(visibleNames(model)) != len(rows)-1 {
		t.Fatalf("visible = %v", visibleNames(model))
	}
}

func TestCycleKeySelectsTheNextOptionOfASingleFacetAndWrapsAround(t *testing.T) {
	facet := sizeFacet("s")
	model := modelForBoard(t, boardWithFacets(groceries(), facet))
	shownSize := func() string { return sizeOf(model.current().visible[0].(fakeRow)) }

	plugintest.Press(t, model, "f", "s")
	afterOne := shownSize()
	plugintest.Press(t, model, "s")
	afterTwo := shownSize()

	if afterOne != facet.Options[1].Value || afterTwo != facet.Options[0].Value {
		t.Fatalf("shown sizes %q then %q", afterOne, afterTwo)
	}
}

func TestCycleKeyWorksOnlyWhileTheMenuIsOpen(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, boardWithFacets(rows, sizeFacet("s")))
	before := visibleNames(model)

	plugintest.Press(t, model, "s")

	if !reflect.DeepEqual(visibleNames(model), before) {
		t.Fatalf("visible = %v, expected %v", visibleNames(model), before)
	}
}

func TestCycleKeyDoesNotChangeAMultipleFacet(t *testing.T) {
	facet := groupFacet()
	facet.CycleKey = "g"
	rows := groceries()
	model := modelForBoard(t, boardWithFacets(rows, facet))

	plugintest.Press(t, model, "f", "g")

	if !reflect.DeepEqual(visibleNames(model), namesOf(rows...)) {
		t.Fatalf("visible = %v", visibleNames(model))
	}
}

func TestBoardActionsAreIgnoredWhileTheMenuIsOpenAndRunAgainOnceItIsClosed(t *testing.T) {
	board := groceryBoard(newRowSource(groceries()...))
	runs := 0
	board.Actions = []Action{{Key: "x", Run: func(Selection) Effect {
		runs++
		return Effect{}
	}}}
	model := modelForBoard(t, board)

	plugintest.Press(t, model, "f", "x")
	whileOpen := runs
	plugintest.Press(t, model, "esc", "x")

	if whileOpen != 0 || runs != 1 {
		t.Fatalf("the action ran %d times while the menu was open and %d times after closing it", whileOpen, runs)
	}
}

func TestQuitKeyStillQuitsWhileTheMenuIsOpen(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))
	plugintest.Press(t, model, "f")

	if !plugintest.Quits(model, "q") {
		t.Fatalf("q should quit with the menu open")
	}
}

func TestMenuHelpOffersClosingAndQuitting(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))

	plugintest.Press(t, model, "f")

	help := ansi.Strip(model.help())
	if !strings.Contains(help, "esc close filters") || !strings.Contains(help, "q quit") {
		t.Fatalf("help = %q", help)
	}
}

func TestMenuShrinksTheTableByItsOwnHeightSoTheViewStillFits(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(manyRows(40)...)))
	plugintest.Resize(t, model, 80, 30)
	closed := len(lines(model))

	plugintest.Press(t, model, "f")

	if open := len(lines(model)); open != closed {
		t.Fatalf("the view has %d lines with the menu open and %d with it closed", open, closed)
	}
}
