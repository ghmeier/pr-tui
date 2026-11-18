package plugin

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// boardWithEffect is a board whose action on "x" returns the effect the function builds for
// the selected row.
func boardWithEffect(effect func(row fakeRow) Effect) Board {
	board := groceryBoard(newRowSource(groceries()...))
	board.Actions = []Action{{Key: "x", Label: "act", Run: func(selection Selection) Effect {
		return effect(selection.Row.(fakeRow))
	}}}
	return board
}

func statusLine(model *Model) string {
	return lines(model)[len(lines(model))-2]
}

func helpLine(model *Model) string {
	return lines(model)[len(lines(model))-1]
}

var errorWithTwoLines = errors.New("first line\nsecond line")

// counter counts the runs of a command that succeeds, or fails with the given error.
type counter struct {
	runs int
	err  error
}

func (count *counter) command() error {
	count.runs++
	return count.err
}

func TestActionRunsOnTheSelectedRow(t *testing.T) {
	rows := groceries()
	var selected []string
	model := modelForBoard(t, boardWithEffect(func(row fakeRow) Effect {
		selected = append(selected, row.name)
		return Effect{}
	}))

	plugintest.Press(t, model, "x", "down", "x")

	if expected := namesOf(rows[0], rows[1]); strings.Join(selected, ",") != strings.Join(expected, ",") {
		t.Fatalf("selected = %v, expected %v", selected, expected)
	}
}

func TestActionDoesNotRunWhenNoRowIsSelected(t *testing.T) {
	ran := false
	board := boardWithEffect(func(fakeRow) Effect {
		ran = true
		return Effect{}
	})
	board.Load = func() ([]Row, error) { return nil, nil }
	model := modelForBoard(t, board)

	plugintest.Press(t, model, "x")

	if ran {
		t.Fatalf("the action ran with nothing selected")
	}
}

func TestEffectNoticeShowsWhileTheCommandRunsAndIsClearedWhenItSucceeds(t *testing.T) {
	count := &counter{}
	notice := "Doing the thing…"
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Notice: notice, Command: count.command} }))

	_, command := model.Update(plugintest.KeyMessage("x"))
	whileRunning := statusLine(model)
	plugintest.Settle(t, model, command)

	if !strings.Contains(whileRunning, notice) || count.runs != 1 {
		t.Fatalf("status = %q while running, ran %d times", whileRunning, count.runs)
	}
	if strings.Contains(statusLine(model), notice) {
		t.Fatalf("status = %q after the command finished", statusLine(model))
	}
}

func TestEffectCommandFailureShowsFailedAndSkipsTheRefresh(t *testing.T) {
	source := newRowSource(groceries()...)
	board := groceryBoard(source)
	count := &counter{err: errorForTests}
	board.Actions = []Action{{Key: "x", Run: func(Selection) Effect {
		return Effect{Command: count.command, Refresh: true}
	}}}
	model := modelForBoard(t, board)
	before := source.loadCount()

	plugintest.Press(t, model, "x")

	if status := statusLine(model); !strings.Contains(status, "Failed: "+errorForTests.Error()) {
		t.Fatalf("status = %q", status)
	}
	if source.loadCount() != before {
		t.Fatalf("the board reloaded %d times after a failed command", source.loadCount()-before)
	}
}

func TestEffectCommandFailureKeepsOnlyTheFirstLine(t *testing.T) {
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect {
		return Effect{Command: func() error { return errorWithTwoLines }}
	}))

	plugintest.Press(t, model, "x")

	if status := statusLine(model); !strings.Contains(status, "Failed: first line") || strings.Contains(status, "second line") {
		t.Fatalf("status = %q", status)
	}
}

func TestEffectRefreshReloadsTheBoardAfterTheCommandSucceeds(t *testing.T) {
	source := newRowSource(groceries()...)
	board := groceryBoard(source)
	board.Actions = []Action{{Key: "x", Run: func(Selection) Effect {
		return Effect{Command: func() error { return nil }, Refresh: true}
	}}}
	model := modelForBoard(t, board)
	before := source.loadCount()

	plugintest.Press(t, model, "x")

	if source.loadCount() != before+1 {
		t.Fatalf("loaded %d times after %d", source.loadCount(), before)
	}
}

func TestEffectWithoutRefreshLeavesTheBoardAsItIs(t *testing.T) {
	source := newRowSource(groceries()...)
	board := groceryBoard(source)
	board.Actions = []Action{{Key: "x", Run: func(Selection) Effect {
		return Effect{Command: func() error { return nil }}
	}}}
	model := modelForBoard(t, board)
	before := source.loadCount()

	plugintest.Press(t, model, "x")

	if source.loadCount() != before {
		t.Fatalf("loaded %d times after %d", source.loadCount(), before)
	}
}

func TestEffectConfirmAsksBeforeRunningAndRunsOnYesOrEnter(t *testing.T) {
	for _, answer := range []string{"y", "enter"} {
		count := &counter{}
		question := "Really?"
		model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Confirm: question, Command: count.command} }))

		plugintest.Press(t, model, "x")
		asked, ranBeforeAnswer := statusLine(model), count.runs
		plugintest.Press(t, model, answer)

		if !strings.Contains(asked, question) || !strings.Contains(asked, "y yes") || ranBeforeAnswer != 0 || count.runs != 1 {
			t.Fatalf("asked %q, ran %d times before answering and %d after %s", asked, ranBeforeAnswer, count.runs, answer)
		}
	}
}

func TestEffectConfirmRunsNothingOnNoOrEscape(t *testing.T) {
	for _, answer := range []string{"n", "esc"} {
		count := &counter{}
		model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Confirm: "Really?", Command: count.command} }))

		plugintest.Press(t, model, "x", answer)

		if count.runs != 0 || strings.Contains(statusLine(model), "Really?") {
			t.Fatalf("ran %d times after %s, status = %q", count.runs, answer, statusLine(model))
		}
	}
}

func TestEffectConfirmIgnoresOtherKeysWhileAsking(t *testing.T) {
	count := &counter{}
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Confirm: "Really?", Command: count.command} }))

	plugintest.Press(t, model, "x", "j", "x", "f")

	if !strings.Contains(statusLine(model), "Really?") || count.runs != 0 {
		t.Fatalf("status = %q, ran %d times", statusLine(model), count.runs)
	}
}

func TestConfirmationHidesTheHelpLine(t *testing.T) {
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Confirm: "Really?", Command: func() error { return nil }} }))

	plugintest.Press(t, model, "x")

	if help := helpLine(model); help != "" {
		t.Fatalf("help line = %q", help)
	}
}

func TestEffectNoticeReplacesWorkingWhenAConfirmedEffectRuns(t *testing.T) {
	notice := "Merging…"
	withNotice := modelForBoard(t, boardWithEffect(func(fakeRow) Effect {
		return Effect{Confirm: "Really?", Notice: notice, Command: func() error { return nil }}
	}))
	withoutNotice := modelForBoard(t, boardWithEffect(func(fakeRow) Effect {
		return Effect{Confirm: "Really?", Command: func() error { return nil }}
	}))
	plugintest.Press(t, withNotice, "x")
	plugintest.Press(t, withoutNotice, "x")

	_, first := withNotice.Update(plugintest.KeyMessage("y"))
	_, second := withoutNotice.Update(plugintest.KeyMessage("y"))

	if !strings.Contains(statusLine(withNotice), notice) || !strings.Contains(statusLine(withoutNotice), "Working…") {
		t.Fatalf("statuses = %q and %q", statusLine(withNotice), statusLine(withoutNotice))
	}
	plugintest.Settle(t, withNotice, first)
	plugintest.Settle(t, withoutNotice, second)
}

func TestEffectScreenOpensAboveTheBoardAndItsInitRuns(t *testing.T) {
	started := false
	screen := &fakeScreen{title: "details"}
	screen.init = func() tea.Msg {
		started = true
		return nil
	}
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Screen: screen} }))

	plugintest.Press(t, model, "x")

	if screenText(model) != "screen "+screen.title || !started {
		t.Fatalf("view = %q, init ran = %v", screenText(model), started)
	}
}

func TestEffectScreenAfterConfirmationOpensOnlyOnceAnswered(t *testing.T) {
	screen := &fakeScreen{title: "details"}
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Confirm: "Open it?", Screen: screen} }))

	plugintest.Press(t, model, "x")
	before := len(model.screens)
	plugintest.Press(t, model, "y")

	if before != 0 || len(model.screens) != 1 {
		t.Fatalf("screens open: %d before answering, %d after", before, len(model.screens))
	}
}
