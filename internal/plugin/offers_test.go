package plugin

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

const offeredKind = "note"

// offering is a board action that takes a value of the kind and records what it ran with.
type offering struct {
	label   string
	kind    string
	applies func(selection Selection) bool
	ranWith []any
	effect  Effect
}

func (offered *offering) action() Action {
	return Action{
		Takes:   offered.kind,
		Label:   offered.label,
		Applies: offered.applies,
		Run: func(selection Selection) Effect {
			offered.ranWith = append(offered.ranWith, selection.Value)
			return offered.effect
		},
	}
}

func newOffering(label string) *offering {
	return &offering{label: label, kind: offeredKind}
}

// offerScreen is a screen that records the labels the selection offers when it receives "g", and
// runs the offer with the given label for the value when it receives "o".
func offerScreen(selection *Selection, offers *[]string, label string, value any) *fakeScreen {
	screen := &fakeScreen{title: "details"}
	screen.onKey = func(pressed string) (Screen, tea.Cmd, bool) {
		switch pressed {
		case "g":
			*offers = selection.Offers(offeredKind)
			return screen, nil, true
		case "o":
			return screen, selection.RunOffer(label, value), true
		}
		return screen, nil, false
	}
	return screen
}

// modelWithOfferScreen opens, from the action on "x", a screen on a board that also has the
// actions that take values.
func modelWithOfferScreen(t *testing.T, label string, value any, offered ...*offering) (*Model, *[]string) {
	t.Helper()
	var selection Selection
	var offers []string
	board := boardOpeningScreen(&selection, offerScreen(&selection, &offers, label, value))
	for _, each := range offered {
		board.Actions = append(board.Actions, each.action())
	}
	model := modelForBoard(t, board)
	plugintest.Press(t, model, "x")
	return model, &offers
}

func TestOffersListsTheActionsThatTakeTheKindInTheOrderTheyWereAdded(t *testing.T) {
	first, second := newOffering("first"), newOffering("second")
	model, offers := modelWithOfferScreen(t, "", nil, first, second)

	plugintest.Press(t, model, "g")

	if expected := []string{first.label, second.label}; !slices.Equal(*offers, expected) {
		t.Fatalf("offers = %v, expected %v", *offers, expected)
	}
}

func TestOffersLeavesOutAnActionWhoseAppliesIsFalseForTheSelection(t *testing.T) {
	applies, declines := newOffering("applies"), newOffering("declines")
	declines.applies = func(Selection) bool { return false }
	model, offers := modelWithOfferScreen(t, "", nil, declines, applies)

	plugintest.Press(t, model, "g")

	if expected := []string{applies.label}; !slices.Equal(*offers, expected) {
		t.Fatalf("offers = %v, expected %v", *offers, expected)
	}
}

func TestOffersLeavesOutAnActionThatTakesAnotherKind(t *testing.T) {
	matching, other := newOffering("matching"), newOffering("other")
	other.kind = "something else"
	model, offers := modelWithOfferScreen(t, "", nil, other, matching)

	plugintest.Press(t, model, "g")

	if expected := []string{matching.label}; !slices.Equal(*offers, expected) {
		t.Fatalf("offers = %v, expected %v", *offers, expected)
	}
}

func TestRunOfferRunsTheActionWithTheValueAndAppliesItsEffect(t *testing.T) {
	count := &counter{}
	sending := newOffering("send")
	sending.effect = Effect{Notice: "Sending", Command: count.command}
	value := struct{ text string }{text: "hello"}
	model, _ := modelWithOfferScreen(t, sending.label, value, sending)

	plugintest.Press(t, model, "o")

	if len(sending.ranWith) != 1 || sending.ranWith[0] != value {
		t.Fatalf("the action ran with %v, expected %v", sending.ranWith, value)
	}
	if count.runs != 1 {
		t.Fatalf("command runs = %d", count.runs)
	}
}

func TestRunOfferDoesNothingWhenTheActionDoesNotApply(t *testing.T) {
	declining := newOffering("declining")
	declining.applies = func(Selection) bool { return false }
	model, _ := modelWithOfferScreen(t, declining.label, "value", declining)

	plugintest.Press(t, model, "o")

	if len(declining.ranWith) != 0 {
		t.Fatalf("the action ran with %v", declining.ranWith)
	}
}

func TestRunOfferAsksTheQuestionOfAnEffectThatChooses(t *testing.T) {
	counters, pick := commandsByOption()
	choosing := newOffering("choosing")
	choosing.effect = choosingSize("", pick)
	model, _ := modelWithOfferScreen(t, choosing.label, "value", choosing)

	plugintest.Press(t, model, "o", "3")

	if counters[sizes[2]].runs != 1 || len(model.screens) != 1 {
		t.Fatalf("runs = %d, screens = %d", counters[sizes[2]].runs, len(model.screens))
	}
}

func TestAnActionThatTakesAKindIsNotInTheHelpLine(t *testing.T) {
	offered := newOffering("only offered")
	board := groceryBoard(newRowSource(groceries()...))
	board.Actions = []Action{offered.action(), {Key: "x", Label: "do it", Run: doNothing}}
	model := modelForBoard(t, board)

	help := lastLine(model)

	if !strings.Contains(help, "do it") || strings.Contains(help, offered.label) {
		t.Fatalf("help = %q", help)
	}
}

func TestAnActionThatTakesAKindDoesNotRunFromAnEmptyKey(t *testing.T) {
	offered := newOffering("only offered")
	board := groceryBoard(newRowSource(groceries()...))
	board.Actions = []Action{offered.action()}
	model := modelForBoard(t, board)
	selection, _ := model.current().selection()

	model.runBoardAction(0, selection, "")

	if len(offered.ranWith) != 0 {
		t.Fatalf("the action ran with %v", offered.ranWith)
	}
}

func TestAnActionThatTakesAKindCannotHaveAKey(t *testing.T) {
	offered := newOffering("keyed").action()
	offered.Key = "k1"
	board := groceryBoard(newRowSource())
	board.Actions = []Action{offered}

	_, err := buildHost(t, `plugins = ["owner"]`, boardPlugin("owner", board))

	assertRejectedNamingTheFileOnce(t, err, offered.Label, offered.Key)
}

func TestAnActionThatTakesAKindNeedsALabel(t *testing.T) {
	offered := newOffering("").action()
	board := groceryBoard(newRowSource())
	board.Actions = []Action{offered}

	_, err := buildHost(t, `plugins = ["owner"]`, boardPlugin("owner", board))

	assertRejectedNamingTheFileOnce(t, err, offered.Takes, "Label")
}

func TestAnActionThatTakesAKindMayBeAddedByAnotherPlugin(t *testing.T) {
	offered := newOffering("added")
	adder := fakePlugin{name: "adder", setup: func(host *Host) error {
		host.AddAction("groceries", offered.action())
		return nil
	}}

	_, err := buildHost(t, `plugins = ["owner", "adder"]`, boardPlugin("owner", groceryBoard(newRowSource())), adder)

	if err != nil {
		t.Fatal(err)
	}
}

func TestChoiceEscapeAppliesTheEffectOfCancel(t *testing.T) {
	count := &counter{}
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect {
		effect := choosingSize("", func(string) Effect { return Effect{} })
		effect.Choice.Cancel = func() Effect { return Effect{Command: count.command} }
		return effect
	}))

	plugintest.Press(t, model, "x", "esc")

	if count.runs != 1 || model.mode != boardMode {
		t.Fatalf("cancel ran %d times, mode = %v", count.runs, model.mode)
	}
}

func TestChoicePickDoesNotApplyCancel(t *testing.T) {
	count := &counter{}
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect {
		effect := choosingSize("", func(string) Effect { return Effect{} })
		effect.Choice.Cancel = func() Effect { return Effect{Command: count.command} }
		return effect
	}))

	plugintest.Press(t, model, "x", "1")

	if count.runs != 0 {
		t.Fatalf("cancel ran %d times", count.runs)
	}
}

type thenMessage struct{ text string }

func TestEffectThenDeliversItsMessageToTheOpenScreens(t *testing.T) {
	var selection Selection
	screen := &fakeScreen{title: "details"}
	board := boardOpeningScreen(&selection, screen)
	model := modelForBoard(t, board)
	plugintest.Press(t, model, "x")
	sent := thenMessage{text: "done"}

	plugintest.Settle(t, model, selection.Run(Effect{Then: func() tea.Msg { return sent }}))

	if !slices.Contains(screen.received, tea.Msg(sent)) {
		t.Fatalf("the screen received %v, expected %v", screen.received, sent)
	}
}

// modelWithOfferScreenOnStock is like modelWithOfferScreen on a board that also has a Stock
// column, whose data the stock delivers once it is switched on, and the source the rows come from.
func modelWithOfferScreenOnStock(t *testing.T, stock *switchableStock, label string, offered *offering) (*Model, *[]string, *rowSource) {
	t.Helper()
	var selection Selection
	var offers []string
	source := newRowSource(groceries()...)
	board := boardOpeningScreen(&selection, offerScreen(&selection, &offers, label, "value"))
	board.Load = source.load
	board.Columns = append(board.Columns, stockColumn(stock.load))
	board.Actions = append(board.Actions, offered.action())
	model := modelForBoard(t, board)
	plugintest.Press(t, model, "x")
	return model, &offers, source
}

func hasStock(selection Selection) bool { return selection.Data("Stock") != nil }

func TestOffersIncludesAnActionWhoseAppliesNeedsDataThatLoadedAfterTheScreenOpened(t *testing.T) {
	stock := &switchableStock{key: groceries()[0].Key()}
	needsStock := newOffering("needs stock")
	needsStock.applies = hasStock
	model, offers, _ := modelWithOfferScreenOnStock(t, stock, needsStock.label, needsStock)
	plugintest.Press(t, model, "g")
	offeredBeforeLoad := slices.Clone(*offers)
	stock.switchTo(true)

	plugintest.Settle(t, model, model.load(model.active))
	plugintest.Press(t, model, "g")

	if len(offeredBeforeLoad) != 0 {
		t.Fatalf("offers before the data loaded = %v", offeredBeforeLoad)
	}
	if expected := []string{needsStock.label}; !slices.Equal(*offers, expected) {
		t.Fatalf("offers = %v, expected %v", *offers, expected)
	}
}

func TestOffersAreEmptyWhenTheBoardNoLongerHasTheRow(t *testing.T) {
	stock := &switchableStock{}
	offered := newOffering("offered")
	model, offers, source := modelWithOfferScreenOnStock(t, stock, offered.label, offered)
	source.set(groceries()[1:]...)

	plugintest.Settle(t, model, model.load(model.active))
	plugintest.Press(t, model, "g")

	if len(*offers) != 0 {
		t.Fatalf("offers = %v", *offers)
	}
}

func TestRunOfferSaysTheActionNoLongerAppliesWhenAppliesTurnedFalseAfterItWasOffered(t *testing.T) {
	stillApplies := true
	offered := newOffering("offered")
	offered.applies = func(Selection) bool { return stillApplies }
	model, offers := modelWithOfferScreen(t, offered.label, "value", offered)
	plugintest.Press(t, model, "g")
	stillApplies = false

	plugintest.Press(t, model, "o")

	if expected := []string{offered.label}; !slices.Equal(*offers, expected) {
		t.Fatalf("offers = %v, expected %v", *offers, expected)
	}
	if len(offered.ranWith) != 0 {
		t.Fatalf("the action ran with %v", offered.ranWith)
	}
	if expected := offered.label + " no longer applies"; model.current().notice != expected {
		t.Fatalf("notice = %q, expected %q", model.current().notice, expected)
	}
}

func TestRunOfferSaysNothingWhenTheActionStillApplies(t *testing.T) {
	offered := newOffering("offered")
	model, _ := modelWithOfferScreen(t, offered.label, "value", offered)

	plugintest.Press(t, model, "o")

	if len(offered.ranWith) != 1 || model.current().notice != "" {
		t.Fatalf("ran with %v, notice = %q", offered.ranWith, model.current().notice)
	}
}

func TestRunOfferSaysTheActionNoLongerAppliesWhenTheBoardLostTheRow(t *testing.T) {
	stock := &switchableStock{}
	offered := newOffering("offered")
	model, _, source := modelWithOfferScreenOnStock(t, stock, offered.label, offered)
	source.set(groceries()[1:]...)

	plugintest.Settle(t, model, model.load(model.active))
	plugintest.Press(t, model, "o")

	if len(offered.ranWith) != 0 || model.current().notice != offered.label+" no longer applies" {
		t.Fatalf("ran with %v, notice = %q", offered.ranWith, model.current().notice)
	}
}
