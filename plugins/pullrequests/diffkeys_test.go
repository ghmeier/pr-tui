package pullrequests

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

func wheel(button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{Button: button, Action: tea.MouseActionPress}
}

func TestDiffViewUpAndDownMoveTheLineCursorWithinTheFile(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)
	onTheAddedLine := strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker)

	press(t, model, "down")

	if !onTheAddedLine || strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("down did not move the cursor off the added line:\n%s", view(model))
	}
	if !strings.Contains(view(model), "file 1 of 2") {
		t.Fatalf("down changed the file:\n%s", view(model))
	}

	press(t, model, "up")

	if !strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("up did not move the cursor back:\n%s", view(model))
	}
}

func TestDiffViewJAndKChangeTheFileAndLeaveTheLineCursorAtItsTop(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	press(t, model, "j")

	if !strings.Contains(view(model), "file 2 of 2") {
		t.Fatalf("j did not select the next file:\n%s", view(model))
	}

	press(t, model, "k")

	if !strings.Contains(view(model), "file 1 of 2") || strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("k did not return to the first file with the cursor at its top:\n%s", view(model))
	}
}

func TestDiffViewMouseWheelMovesTheLineCursorOneLinePerNotch(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	plugintest.Send(t, model, wheel(tea.MouseButtonWheelDown))

	if strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("the wheel did not move the cursor down:\n%s", view(model))
	}

	plugintest.Send(t, model, wheel(tea.MouseButtonWheelUp))

	if !strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("the wheel did not move the cursor up:\n%s", view(model))
	}
}

func TestDiffViewIgnoresOtherMouseEvents(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	plugintest.Send(t, model, tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	plugintest.Send(t, model, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionRelease})

	if !strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("the cursor moved:\n%s", view(model))
	}
}

func TestDiffViewSpaceAndBNoLongerPage(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	press(t, model, " ", "b")

	if !strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("the cursor moved:\n%s", view(model))
	}
}

func TestDiffViewPageDownStillPages(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	press(t, model, "pgdown")

	if strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("pgdown did not move the cursor:\n%s", view(model))
	}
}

func TestDiffViewHelpLineListsLineFilePageKeys(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter")

	help := helpLine(model)

	for _, expected := range []string{"↑/↓ line", "j/k file", "pgup/pgdn page"} {
		if !strings.Contains(help, expected) {
			t.Fatalf("help line is missing %q: %s", expected, help)
		}
	}
	if strings.Contains(help, "space/b") {
		t.Fatalf("help line still lists space/b: %s", help)
	}
}
