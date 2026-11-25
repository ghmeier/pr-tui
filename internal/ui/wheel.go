package ui

import tea "github.com/charmbracelet/bubbletea"

// WheelNotchLines is how many lines of a scrolling view one notch of the mouse wheel moves.
const WheelNotchLines = 3

// WheelScroll is the number of lines a mouse event scrolls a view by: negative for up, positive
// for down, and zero for anything that is not a wheel notch.
func WheelScroll(event tea.MouseMsg) int {
	if event.Action != tea.MouseActionPress {
		return 0
	}
	switch event.Button {
	case tea.MouseButtonWheelUp:
		return -WheelNotchLines
	case tea.MouseButtonWheelDown:
		return WheelNotchLines
	}
	return 0
}
