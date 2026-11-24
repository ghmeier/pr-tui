package plugin

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

var fruitNames = []string{"apple", "banana", "cherry", "date", "elderberry", "fig", "grape"}

// choosingFruit asks for a fruit in a menu and records what is picked.
func choosingFruit(options []string, picked *[]string) Effect {
	return Effect{Choice: &Choice{Question: "Fruit?", Menu: true, Options: options, Pick: func(option string) Effect {
		*picked = append(*picked, option)
		return Effect{}
	}}}
}

func menuModel(t *testing.T, options []string, picked *[]string) *Model {
	t.Helper()
	return modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return choosingFruit(options, picked) }))
}

func TestMenuShowsTheQuestionTheOptionsAndHowToUseThemAboveTheBoard(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x")
	view := screenText(model)

	for _, expected := range append([]string{"Fruit?", "enter pick", "esc cancel"}, fruitNames...) {
		if !strings.Contains(view, expected) {
			t.Fatalf("%q is not in %q", expected, view)
		}
	}
	if strings.Index(view, "Fruit?") > strings.Index(view, "apple-key") && strings.Contains(view, "apple-key") {
		t.Fatalf("the menu should be above the table in %q", view)
	}
	if strings.Contains(statusLine(model), "Fruit?") {
		t.Fatalf("the question should not also be in the status line")
	}
}

func TestMenuTypingNarrowsTheOptionsIgnoringCase(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x", "E", "R")
	view := screenText(model)

	if !strings.Contains(view, "cherry") || !strings.Contains(view, "elderberry") || strings.Contains(view, "banana") {
		t.Fatalf("view = %q", view)
	}
	if !strings.Contains(view, "2 of 7") {
		t.Fatalf("the count is missing in %q", view)
	}
}

func TestMenuBackspaceWidensTheOptionsAgain(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x", "z")
	narrowed := screenText(model)
	plugintest.Press(t, model, "backspace")

	if !strings.Contains(narrowed, "No matches") || !strings.Contains(screenText(model), "banana") {
		t.Fatalf("narrowed = %q, widened = %q", narrowed, screenText(model))
	}
}

func TestMenuTypesQInsteadOfQuitting(t *testing.T) {
	var picked []string
	model := menuModel(t, []string{"quince", "pear"}, &picked)

	plugintest.Press(t, model, "x")
	quits := plugintest.Quits(model, "q")
	view := screenText(model)

	if quits || !strings.Contains(view, "quince") || strings.Contains(view, "pear") {
		t.Fatalf("quits = %v, view = %q", quits, view)
	}
}

func TestBoardStillQuitsWithQWhileAStatusLineChoiceIsOpen(t *testing.T) {
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return choosingSize("", nil) }))

	plugintest.Press(t, model, "x")

	if !plugintest.Quits(model, "q") {
		t.Fatalf("q should quit")
	}
}

func TestMenuCtrlCQuits(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x")

	if !plugintest.Quits(model, "ctrl+c") {
		t.Fatalf("ctrl+c should quit")
	}
}

func TestMenuTypesSpaceDigitsAndMovementLetters(t *testing.T) {
	var picked []string
	model := menuModel(t, []string{"j 1 k", "other"}, &picked)

	plugintest.Press(t, model, "x", "j", " ", "1", " ", "k", "enter")

	if len(picked) != 1 || picked[0] != "j 1 k" {
		t.Fatalf("picked %v", picked)
	}
}

func TestMenuEnterPicksTheOptionUnderTheCursor(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x", "down", "down", "enter")

	if len(picked) != 1 || picked[0] != fruitNames[2] || model.mode != boardMode {
		t.Fatalf("picked %v, mode %v", picked, model.mode)
	}
}

func TestMenuControlKeysMoveTheCursorLikeTheArrows(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x", "ctrl+n", "ctrl+n", "ctrl+p", "enter")

	if len(picked) != 1 || picked[0] != fruitNames[1] {
		t.Fatalf("picked %v", picked)
	}
}

func TestMenuEnterPicksFromTheNarrowedOptions(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x", "r", "r", "down", "enter")

	if len(picked) != 1 || picked[0] != "elderberry" {
		t.Fatalf("picked %v", picked)
	}
}

func TestMenuCursorStopsAtTheEndsAndFollowsANarrowedList(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x", "up", "down", "down", "down", "down", "down", "down", "down", "down", "enter")
	plugintest.Press(t, model, "x", "down", "down", "down", "g", "enter")

	if len(picked) != 2 || picked[0] != fruitNames[len(fruitNames)-1] || picked[1] != "grape" {
		t.Fatalf("picked %v", picked)
	}
}

func TestMenuEnterWithNoMatchDoesNothing(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x", "z", "enter")

	if len(picked) != 0 || model.mode != choiceMode {
		t.Fatalf("picked %v, mode %v", picked, model.mode)
	}
}

func TestMenuEscapeCancelsAndForgetsWhatWasTyped(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x", "a", "esc")
	closed := model.mode
	plugintest.Press(t, model, "x")

	if len(picked) != 0 || closed != boardMode || !strings.Contains(screenText(model), "banana") {
		t.Fatalf("picked %v, closed mode %v, view %q", picked, closed, screenText(model))
	}
}

func manyOptions(count int) []string {
	options := make([]string, count)
	for index := range options {
		options[index] = fmt.Sprintf("option-%02d", index)
	}
	return options
}

func TestMenuShowsNoMoreThanTenOptionsAndScrollsToKeepTheCursorVisible(t *testing.T) {
	var picked []string
	options := manyOptions(25)
	model := menuModel(t, options, &picked)

	plugintest.Press(t, model, "x")
	first := screenText(model)
	for range 20 {
		plugintest.Press(t, model, "down")
	}
	scrolled := screenText(model)
	plugintest.Press(t, model, "enter")

	if strings.Count(first, "option-") != maxMenuOptions || !strings.Contains(first, options[0]) || strings.Contains(first, options[maxMenuOptions]) {
		t.Fatalf("first view = %q", first)
	}
	if strings.Count(scrolled, "option-") != maxMenuOptions || !strings.Contains(scrolled, options[20]) || strings.Contains(scrolled, options[0]) {
		t.Fatalf("scrolled view = %q", scrolled)
	}
	if !strings.Contains(scrolled, "10 of 25") || picked[0] != options[20] {
		t.Fatalf("picked %v, view = %q", picked, scrolled)
	}
}

func TestMenuShowsNoCountWhenEveryOptionIsShown(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x")

	if strings.Contains(screenText(model), " of ") {
		t.Fatalf("view = %q", screenText(model))
	}
}

func TestMenuMarksOnlyTheCursorOption(t *testing.T) {
	var picked []string
	model := menuModel(t, fruitNames, &picked)

	plugintest.Press(t, model, "x", "down")

	marked := 0
	for _, line := range strings.Split(model.View(), "\n") {
		if strings.Contains(line, ui.CursorMarker) {
			marked++
			if !strings.Contains(line, fruitNames[1]) {
				t.Fatalf("the cursor is on %q", line)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("%d lines have the cursor", marked)
	}
}

func TestMenuOverAScreenIsDrawnBelowItAndTakesTheKeys(t *testing.T) {
	var picked []string
	model, screen := modelWithScreenRunningAction(t, newRowSource(groceries()...), func(fakeRow) Effect {
		return choosingFruit(fruitNames, &picked)
	})

	plugintest.Press(t, model, "g")
	asked := lines(model)
	menuHeight := len(asked) - 1
	heightWhileChoosing := screen.height
	plugintest.Press(t, model, "q", "backspace", "down", "enter")

	if asked[0] != "screen "+screen.title || !strings.Contains(screenText(model), "screen "+screen.title) {
		t.Fatalf("view while choosing = %q", asked)
	}
	if !strings.Contains(strings.Join(asked, "\n"), "Fruit?") || heightWhileChoosing != model.height-menuHeight {
		t.Fatalf("the menu is missing or the screen height is %d, view = %q", heightWhileChoosing, asked)
	}
	if len(screen.receivedKeys()) != 0 || len(picked) != 1 || picked[0] != fruitNames[1] {
		t.Fatalf("the screen received %v, picked %v", screen.receivedKeys(), picked)
	}
}
