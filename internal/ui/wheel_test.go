package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWheelScrollMovesOneNotchPerPressAndIgnoresEverythingElse(t *testing.T) {
	up := tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}
	down := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	click := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	release := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionRelease}

	if WheelScroll(up) != -WheelNotchLines || WheelScroll(down) != WheelNotchLines {
		t.Fatalf("up %d, down %d", WheelScroll(up), WheelScroll(down))
	}
	if WheelScroll(click) != 0 || WheelScroll(release) != 0 {
		t.Fatalf("click %d, release %d", WheelScroll(click), WheelScroll(release))
	}
}
