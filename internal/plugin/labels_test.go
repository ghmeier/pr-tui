package plugin

import (
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// labelledBoard has an action whose label depends on the row, which is empty for vegetables, and
// an action without a label function.
func labelledBoard(source *rowSource, ran *[]string) Board {
	board := groceryBoard(source)
	board.Actions = []Action{
		{Key: "x", Label: "fixed", Run: func(Selection) Effect { *ran = append(*ran, "x"); return Effect{} }},
		{Key: "y", LabelFor: func(row Row) string {
			if row.(fakeRow).group == "vegetable" {
				return ""
			}
			return "eat " + row.(fakeRow).name
		}, Run: func(Selection) Effect { *ran = append(*ran, "y"); return Effect{} }},
	}
	return board
}

func TestLabelForReplacesTheLabelWithTheOneForTheCursorRow(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, labelledBoard(newRowSource(rows...), new([]string)))

	onFruit := helpLine(model)
	plugintest.Press(t, model, "down", "down")
	onOtherFruit := helpLine(model)

	for help, row := range map[string]fakeRow{onFruit: rows[0].(fakeRow), onOtherFruit: rows[2].(fakeRow)} {
		if !strings.Contains(help, "y eat "+row.name) {
			t.Fatalf("help = %q, expected the label for %s", help, row.name)
		}
	}
}

func TestAnEmptyLabelForHidesTheActionAndItsKeyDoesNothing(t *testing.T) {
	var ran []string
	model := modelForBoard(t, labelledBoard(newRowSource(groceries()...), &ran))

	plugintest.Press(t, model, "down")
	hidden := helpLine(model)
	plugintest.Press(t, model, "y")

	if strings.Contains(hidden, "y ") {
		t.Fatalf("help = %q", hidden)
	}
	if len(ran) != 0 {
		t.Fatalf("the action ran for a row it does not apply to: %v", ran)
	}
}

func TestAnActionWithLabelForRunsOnARowItAppliesTo(t *testing.T) {
	var ran []string
	model := modelForBoard(t, labelledBoard(newRowSource(groceries()...), &ran))

	plugintest.Press(t, model, "y")

	if len(ran) != 1 || ran[0] != "y" {
		t.Fatalf("ran = %v", ran)
	}
}

func TestAnActionWithoutLabelForKeepsItsLabelOnEveryRow(t *testing.T) {
	var ran []string
	model := modelForBoard(t, labelledBoard(newRowSource(groceries()...), &ran))

	plugintest.Press(t, model, "down", "x")

	if help := helpLine(model); !strings.Contains(help, "x fixed") {
		t.Fatalf("help = %q", help)
	}
	if len(ran) != 1 || ran[0] != "x" {
		t.Fatalf("ran = %v", ran)
	}
}

func TestSelectionRunActionDoesNothingForAnActionThatDoesNotApply(t *testing.T) {
	var ran []string
	var opened Selection
	board := labelledBoard(newRowSource(groceries()...), &ran)
	board.Actions = append(board.Actions, Action{Key: "z", Label: "open", Run: func(selection Selection) Effect {
		opened = selection
		return Effect{}
	}})
	model := modelForBoard(t, board)
	plugintest.Press(t, model, "down", "z")

	plugintest.Settle(t, model, opened.RunAction("y"))
	plugintest.Settle(t, model, opened.RunAction("x"))

	if len(ran) != 1 || ran[0] != "x" {
		t.Fatalf("ran = %v", ran)
	}
}

// appliesBoard has an action on "y" that applies only to fruit, and records when it runs.
func appliesBoard(ran *[]string) Board {
	board := groceryBoard(newRowSource(groceries()...))
	board.Actions = []Action{{
		Key: "y", Label: "eat",
		Applies: func(selection Selection) bool { return selection.Row.(fakeRow).group == "fruit" },
		Run:     func(Selection) Effect { *ran = append(*ran, "y"); return Effect{} },
	}}
	return board
}

func TestAKeyedActionWhoseAppliesIsFalseIsHiddenAndItsKeyDoesNothing(t *testing.T) {
	var ran []string
	model := modelForBoard(t, appliesBoard(&ran))

	plugintest.Press(t, model, "down")
	hidden := helpLine(model)
	plugintest.Press(t, model, "y")

	if strings.Contains(hidden, "y eat") || len(ran) != 0 {
		t.Fatalf("help = %q, ran = %v", hidden, ran)
	}
}

func TestAKeyedActionWhoseAppliesIsTrueIsShownAndRuns(t *testing.T) {
	var ran []string
	model := modelForBoard(t, appliesBoard(&ran))

	shown := helpLine(model)
	plugintest.Press(t, model, "y")

	if !strings.Contains(shown, "y eat") || len(ran) != 1 {
		t.Fatalf("help = %q, ran = %v", shown, ran)
	}
}

func TestLabelOfAppliesOnlyToActionsWithoutLabelForOrAppliesWhenThereIsNoRow(t *testing.T) {
	fixed := Action{Label: "fixed"}
	labelled := Action{LabelFor: func(Row) string { return "label" }}
	conditional := Action{Label: "conditional", Applies: func(Selection) bool { return true }}

	fixedLabel, fixedApplies := fixed.LabelOf(Selection{})
	_, labelledApplies := labelled.LabelOf(Selection{})
	_, conditionalApplies := conditional.LabelOf(Selection{})

	if fixedLabel != "fixed" || !fixedApplies || labelledApplies || conditionalApplies {
		t.Fatalf("fixed = %q, %v; labelled applies = %v; conditional applies = %v", fixedLabel, fixedApplies, labelledApplies, conditionalApplies)
	}
}

// boardCapturingSelection has an action on "x" that keeps the selection it ran on and reloads the
// row, and an action on "z" that only keeps the selection.
func boardCapturingSelection(source *rowSource, reload *reloader, captured *Selection) Board {
	board := boardRefreshingRow(source, reload, succeeds)
	board.Actions = append(board.Actions,
		Action{Key: "z", Label: "keep", Run: func(selection Selection) Effect {
			*captured = selection
			return Effect{Command: succeeds, RefreshRow: selection.Row}
		}},
	)
	return board
}

func TestSelectionLatestReturnsTheReloadedRowAfterARowRefresh(t *testing.T) {
	rows := groceries()
	renamed := fruit(rows[0].Key(), "green apple")
	reload := &reloader{current: map[string]Row{renamed.key: renamed}}
	var captured Selection
	model := modelForBoard(t, boardCapturingSelection(newRowSource(rows...), reload, &captured))

	plugintest.Press(t, model, "z")
	latest := captured.Latest()

	if latest.Row.(fakeRow).name != renamed.name {
		t.Fatalf("latest row = %v, expected %v", latest.Row, renamed)
	}
	if captured.Row.(fakeRow).name != rows[0].(fakeRow).name {
		t.Fatalf("the selection's own row changed to %v", captured.Row)
	}
}

func TestSelectionLatestKeepsItsOwnRowOnceTheRowIsGone(t *testing.T) {
	reload := &reloader{current: map[string]Row{}}
	var captured Selection
	model := modelForBoard(t, boardCapturingSelection(newRowSource(groceries()...), reload, &captured))

	plugintest.Press(t, model, "z")
	latest := captured.Latest()

	if latest.Row != captured.Row {
		t.Fatalf("latest row = %v, expected the selection's own %v", latest.Row, captured.Row)
	}
}
