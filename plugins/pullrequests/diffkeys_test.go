package pullrequests

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

func mouse(button tea.MouseButton, action tea.MouseAction) tea.MouseMsg {
	return tea.MouseMsg{Button: button, Action: action}
}

// cursorIsOnTheAddedLine reports whether the diff's cursor marker is on the line "added".
func cursorIsOnTheAddedLine(t *testing.T, model *plugin.Model) bool {
	t.Helper()
	return strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker)
}

func TestDiffViewDownMovesTheLineCursorWithinTheFile(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	press(t, model, "down")

	if cursorIsOnTheAddedLine(t, model) || !strings.Contains(view(model), "file 1 of 2") {
		t.Fatalf("down did not move the cursor within the first file:\n%s", view(model))
	}
}

func TestDiffViewUpMovesTheLineCursorBack(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)
	press(t, model, "down")

	press(t, model, "up")

	if !cursorIsOnTheAddedLine(t, model) {
		t.Fatalf("up did not move the cursor back:\n%s", view(model))
	}
}

func TestDiffViewJSelectsTheNextFile(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	press(t, model, "j")

	if !strings.Contains(view(model), "file 2 of 2") {
		t.Fatalf("j did not select the next file:\n%s", view(model))
	}
}

func TestDiffViewKSelectsThePreviousFileWithTheLineCursorAtItsTop(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)
	press(t, model, "j")

	press(t, model, "k")

	if !strings.Contains(view(model), "file 1 of 2") || cursorIsOnTheAddedLine(t, model) {
		t.Fatalf("k did not return to the first file with the cursor at its top:\n%s", view(model))
	}
}

func TestDiffViewMouseWheelDownMovesTheLineCursorDown(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	plugintest.Send(t, model, mouse(tea.MouseButtonWheelDown, tea.MouseActionPress))

	if cursorIsOnTheAddedLine(t, model) {
		t.Fatalf("the wheel did not move the cursor down:\n%s", view(model))
	}
}

func TestDiffViewMouseWheelUpMovesTheLineCursorUp(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)
	press(t, model, "down")

	plugintest.Send(t, model, mouse(tea.MouseButtonWheelUp, tea.MouseActionPress))

	if !cursorIsOnTheAddedLine(t, model) {
		t.Fatalf("the wheel did not move the cursor up:\n%s", view(model))
	}
}

func TestDiffViewIgnoresMouseEventsThatAreNotWheelPresses(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	plugintest.Send(t, model, mouse(tea.MouseButtonLeft, tea.MouseActionPress))
	plugintest.Send(t, model, mouse(tea.MouseButtonWheelDown, tea.MouseActionRelease))

	if !cursorIsOnTheAddedLine(t, model) {
		t.Fatalf("the cursor moved:\n%s", view(model))
	}
}

func TestDiffViewSpaceAndBDoNotPage(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	press(t, model, " ", "b")

	if !cursorIsOnTheAddedLine(t, model) {
		t.Fatalf("the cursor moved:\n%s", view(model))
	}
}

func TestDiffViewPageDownMovesTheLineCursor(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	press(t, model, "pgdown")

	if cursorIsOnTheAddedLine(t, model) {
		t.Fatalf("pgdown did not move the cursor:\n%s", view(model))
	}
}

func TestDiffViewHelpLineListsTheLineFileAndPageKeys(t *testing.T) {
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
