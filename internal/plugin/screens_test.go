package plugin

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// modelWithScreen opens the screen from an action on "x".
func modelWithScreen(t *testing.T, screen Screen) *Model {
	t.Helper()
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Screen: screen} }))
	plugintest.Press(t, model, "x")
	return model
}

func TestOpenScreenShowsTheScreenInPlaceOfTheBoardAndPassesTheWindowSize(t *testing.T) {
	screen := &fakeScreen{title: "details"}
	model := modelWithScreen(t, screen)

	plugintest.Resize(t, model, 90, 25)

	if screenText(model) != "screen "+screen.title || screen.width != 90 || screen.height != 25 {
		t.Fatalf("view = %q, the screen was drawn at %dx%d", screenText(model), screen.width, screen.height)
	}
}

func TestScreenGetsEveryKeyIncludingTheBoardsAndTheQuitKey(t *testing.T) {
	screen := &fakeScreen{title: "details"}
	model := modelWithScreen(t, screen)

	plugintest.Press(t, model, "q", "j", "f", "r")

	if !reflect.DeepEqual(screen.receivedKeys(), []string{"q", "j", "f", "r"}) {
		t.Fatalf("the screen received %v", screen.receivedKeys())
	}
	if plugintest.Quits(model, "q") {
		t.Fatalf("q should go to the screen, not quit")
	}
}

func TestBoardDoesNotSeeKeysWhileAScreenIsOpen(t *testing.T) {
	source := newRowSource(groceries()...)
	board := groceryBoard(source)
	board.Actions = []Action{{Key: "x", Run: func(Selection) Effect { return Effect{Screen: &fakeScreen{title: "details"}} }}}
	model := modelForBoard(t, board)
	plugintest.Press(t, model, "x")
	before := source.loadCount()

	plugintest.Press(t, model, "r", "down")

	cursor := model.current().cursor
	if source.loadCount() != before || cursor != 0 {
		t.Fatalf("reloads = %d, cursor = %d", source.loadCount()-before, cursor)
	}
}

func TestCtrlCQuitsEvenWhileAScreenIsOpen(t *testing.T) {
	screen := &fakeScreen{title: "details"}
	model := modelWithScreen(t, screen)

	if !plugintest.Quits(model, "ctrl+c") || len(screen.receivedKeys()) != 0 {
		t.Fatalf("ctrl+c should quit without reaching the screen, which received %v", screen.receivedKeys())
	}
}

func TestScreenClosesWhenItsUpdateReturnsNil(t *testing.T) {
	model := modelWithScreen(t, &fakeScreen{title: "details"})

	plugintest.Press(t, model, "esc")

	if len(model.screens) != 0 || !strings.Contains(screenText(model), "Name") {
		t.Fatalf("the board should be back:\n%s", screenText(model))
	}
}

func TestScreenReplacedByItsUpdateStaysOnTheStackInPlace(t *testing.T) {
	replacement := &fakeScreen{title: "replacement"}
	original := &fakeScreen{title: "original", onKey: func(key string) (Screen, tea.Cmd, bool) {
		return replacement, nil, key == "n"
	}}
	model := modelWithScreen(t, original)

	plugintest.Press(t, model, "n")

	if screenText(model) != "screen "+replacement.title || len(model.screens) != 1 {
		t.Fatalf("view = %q with %d screens", screenText(model), len(model.screens))
	}
}

func TestScreenCanOpenAnotherScreenOnTopOfItselfAndReturnToIt(t *testing.T) {
	top := &fakeScreen{title: "top"}
	bottom := &fakeScreen{title: "bottom"}
	bottom.onKey = func(key string) (Screen, tea.Cmd, bool) {
		return bottom, OpenScreen(top), key == "p"
	}
	model := modelWithScreen(t, bottom)

	plugintest.Press(t, model, "p")
	opened := screenText(model)
	plugintest.Press(t, model, "esc")

	if opened != "screen "+top.title || screenText(model) != "screen "+bottom.title {
		t.Fatalf("opened view = %q, view after closing the top screen = %q", opened, screenText(model))
	}
}

func TestOpenedScreenRunsItsInit(t *testing.T) {
	started := false
	top := &fakeScreen{title: "top", init: func() tea.Msg {
		started = true
		return nil
	}}
	model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))

	plugintest.Send(t, model, openScreenMessage{screen: top})

	if !started || len(model.screens) != 1 {
		t.Fatalf("init ran = %v, screens = %d", started, len(model.screens))
	}
}

// orderedScreen reports to a shared list, in the order screens are updated, that it received
// a message.
type orderedScreen struct {
	fakeScreen
	order *[]string
}

func (screen *orderedScreen) Update(message tea.Msg) (Screen, tea.Cmd) {
	if _, isOurs := message.(screenMessage); isOurs {
		*screen.order = append(*screen.order, screen.title)
	}
	return screen, nil
}

func TestMessageTheHostDoesNotKnowReachesEveryScreenTopFirst(t *testing.T) {
	var order []string
	bottom := &orderedScreen{fakeScreen: fakeScreen{title: "bottom"}, order: &order}
	top := &orderedScreen{fakeScreen: fakeScreen{title: "top"}, order: &order}
	model := modelWithScreen(t, bottom)
	plugintest.Send(t, model, openScreenMessage{screen: top})

	plugintest.Send(t, model, screenMessage{})

	if !reflect.DeepEqual(order, []string{top.title, bottom.title}) {
		t.Fatalf("screens updated in the order %v", order)
	}
}

func TestScreenBelowTheTopOneClosesItselfWhenItsOwnMessageArrives(t *testing.T) {
	top := &fakeScreen{title: "top"}
	model := modelWithScreen(t, &closingOnMessageScreen{})
	plugintest.Send(t, model, openScreenMessage{screen: top})

	plugintest.Send(t, model, bottomClosesMessage{})

	if len(model.screens) != 1 || model.screens[0] != Screen(top) {
		t.Fatalf("screens = %v, expected only the top screen to remain", model.screens)
	}
}

type bottomClosesMessage struct{}

// closingOnMessageScreen closes itself when it receives bottomClosesMessage.
type closingOnMessageScreen struct{}

func (*closingOnMessageScreen) Init() tea.Cmd { return nil }

func (screen *closingOnMessageScreen) Update(message tea.Msg) (Screen, tea.Cmd) {
	if _, isClosing := message.(bottomClosesMessage); isClosing {
		return nil, nil
	}
	return screen, nil
}

func (*closingOnMessageScreen) View(int, int) string { return "closing screen" }

func TestMessageIsNotDeliveredToAnyScreenWhenNoneIsOpen(t *testing.T) {
	model := modelForBoard(t, groceryBoard(newRowSource(groceries()...)))

	_, command := model.Update(screenMessage{target: "nobody"})

	if command != nil {
		t.Fatalf("a message with no screen open should do nothing")
	}
}

func TestScreenCommandsRunAndTheirMessagesReturnToTheScreen(t *testing.T) {
	var screen *fakeScreen
	answer := screenMessage{target: "details"}
	screen = &fakeScreen{title: "details", onKey: func(key string) (Screen, tea.Cmd, bool) {
		return screen, func() tea.Msg { return answer }, key == "g"
	}}
	model := modelWithScreen(t, screen)

	plugintest.Press(t, model, "g")

	if len(screen.received) != 1 || screen.received[0] != answer {
		t.Fatalf("the screen received %v", screen.received)
	}
}

func TestNotifyShowsANoticeOnTheBoardOnceTheScreenIsGone(t *testing.T) {
	notice := "Diff failed: it broke"
	screen := &fakeScreen{title: "details"}
	screen.onKey = func(key string) (Screen, tea.Cmd, bool) { return nil, Notify(notice), key == "n" }
	model := modelWithScreen(t, screen)

	plugintest.Press(t, model, "n")

	if !strings.Contains(statusLine(model), notice) {
		t.Fatalf("status = %q", statusLine(model))
	}
}
