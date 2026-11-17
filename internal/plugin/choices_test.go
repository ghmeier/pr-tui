package plugin

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

var sizes = []string{"small", "medium", "large"}

// choosingSize is an effect that asks for a size, and picks the effect the function returns.
func choosingSize(current string, pick func(option string) Effect) Effect {
	return Effect{Choice: &Choice{Question: "Size?", Options: sizes, Current: current, Pick: pick}}
}

// commandsByOption makes each option's effect run a command of its own, counted separately.
func commandsByOption() (map[string]*counter, func(option string) Effect) {
	counters := map[string]*counter{}
	for _, option := range sizes {
		counters[option] = &counter{}
	}
	return counters, func(option string) Effect { return Effect{Command: counters[option].command} }
}

func TestChoiceAppliesTheEffectPickReturnsForTheNumberedOption(t *testing.T) {
	counters, pick := commandsByOption()
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return choosingSize("", pick) }))

	plugintest.Press(t, model, "x", "2")

	if counters[sizes[1]].runs != 1 || counters[sizes[0]].runs != 0 || counters[sizes[2]].runs != 0 {
		t.Fatalf("runs = %d, %d, %d", counters[sizes[0]].runs, counters[sizes[1]].runs, counters[sizes[2]].runs)
	}
	if model.mode != boardMode {
		t.Fatalf("the choice should be over")
	}
}

func TestChoiceEscapeAppliesNothing(t *testing.T) {
	counters, pick := commandsByOption()
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return choosingSize("", pick) }))

	plugintest.Press(t, model, "x", "esc", "1")

	for option, count := range counters {
		if count.runs != 0 {
			t.Fatalf("%s ran %d times", option, count.runs)
		}
	}
	if model.mode != boardMode || strings.Contains(statusLine(model), "Size?") {
		t.Fatalf("status = %q", statusLine(model))
	}
}

func TestChoiceIgnoresANumberBeyondTheOptionsAndOtherKeys(t *testing.T) {
	counters, pick := commandsByOption()
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return choosingSize("", pick) }))

	plugintest.Press(t, model, "x", "4", "0", "y", "j", "f", "x")

	for option, count := range counters {
		if count.runs != 0 {
			t.Fatalf("%s ran %d times", option, count.runs)
		}
	}
	if model.mode != choiceMode || !strings.Contains(statusLine(model), "Size?") {
		t.Fatalf("the choice should still be pending, status = %q", statusLine(model))
	}
}

func TestChoiceShowsNoMoreThanNineOptions(t *testing.T) {
	var picked []string
	options := strings.Split("a b c d e f g h i j", " ")
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect {
		return Effect{Choice: &Choice{Question: "Letter?", Options: options, Pick: func(option string) Effect {
			picked = append(picked, option)
			return Effect{}
		}}}
	}))

	plugintest.Press(t, model, "x")
	shown := statusLine(model)
	plugintest.Press(t, model, "9")

	if strings.Contains(shown, "10") || strings.Contains(shown, options[9]) || len(picked) != 1 || picked[0] != options[8] {
		t.Fatalf("status = %q, picked %v", shown, picked)
	}
}

func TestChoicePickedEffectThatConfirmsAsksItsQuestion(t *testing.T) {
	count := &counter{}
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect {
		return choosingSize("", func(string) Effect { return Effect{Confirm: "Really?", Command: count.command} })
	}))

	plugintest.Press(t, model, "x", "1")
	asked, ranBeforeAnswer := statusLine(model), count.runs
	plugintest.Press(t, model, "y")

	if !strings.Contains(asked, "Really?") || ranBeforeAnswer != 0 || count.runs != 1 {
		t.Fatalf("asked %q, ran %d times before answering and %d after", asked, ranBeforeAnswer, count.runs)
	}
}

func TestChoiceStatusLineListsNumberedOptionsAndHowToCancel(t *testing.T) {
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return choosingSize("", nil) }))

	plugintest.Press(t, model, "x")

	expected := "Size?  1 small  2 medium  3 large  esc cancel"
	if status := statusLine(model); status != expected {
		t.Fatalf("status = %q, expected %q", status, expected)
	}
	if help := helpLine(model); help != "" {
		t.Fatalf("help line = %q", help)
	}
}

func TestChoiceStatusLineMarksOnlyTheCurrentOption(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return choosingSize(sizes[1], nil) }))

	plugintest.Press(t, model, "x")
	status := model.View()

	if !strings.Contains(status, ui.BoldStyle.Render(sizes[1])) {
		t.Fatalf("the current option is not bold in %q", status)
	}
	for _, other := range []string{sizes[0], sizes[2]} {
		if strings.Contains(status, ui.BoldStyle.Render(other)) {
			t.Fatalf("%s should not be bold in %q", other, status)
		}
	}
}

func TestChoiceFromRunActionShowsBeneathTheScreenAndKeysDoNotReachTheScreen(t *testing.T) {
	counters, pick := commandsByOption()
	model, screen := modelWithScreenRunningAction(t, newRowSource(groceries()...), func(fakeRow) Effect {
		return choosingSize("", pick)
	})

	plugintest.Press(t, model, "g")
	asked := lines(model)
	plugintest.Press(t, model, "q", "j", "x", "3")

	if asked[0] != "screen "+screen.title || !strings.Contains(asked[len(asked)-1], "Size?") {
		t.Fatalf("view while choosing = %q", asked)
	}
	if len(screen.receivedKeys()) != 0 || plugintest.Quits(model, "q") {
		t.Fatalf("the screen received %v", screen.receivedKeys())
	}
	if counters[sizes[2]].runs != 1 || len(model.screens) != 1 || model.mode != boardMode {
		t.Fatalf("runs = %d, screens = %d", counters[sizes[2]].runs, len(model.screens))
	}
}

func TestChoiceFromRunActionCanBeCancelledAndLeavesTheScreenOpen(t *testing.T) {
	counters, pick := commandsByOption()
	model, screen := modelWithScreenRunningAction(t, newRowSource(groceries()...), func(fakeRow) Effect {
		return choosingSize("", pick)
	})

	plugintest.Press(t, model, "g", "esc")

	if counters[sizes[0]].runs != 0 || len(model.screens) != 1 || screenText(model) != "screen "+screen.title {
		t.Fatalf("view = %q", screenText(model))
	}
}
