package pullrequests

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// reverseVideo is what the terminal receives to draw selected rows.
const reverseVideo = "\x1b[7m"

// replaceClipboard records what the board copies, until the test ends.
func replaceClipboard(t *testing.T) *[]string {
	t.Helper()
	var copied []string
	original := shell.WriteClipboard
	shell.WriteClipboard = func(text string) error {
		copied = append(copied, text)
		return nil
	}
	t.Cleanup(func() { shell.WriteClipboard = original })
	return &copied
}

// showColors makes styled text carry its escape sequences, which tests drawn without a terminal
// leave out, until the test ends.
func showColors(t *testing.T) {
	t.Helper()
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })
}

// positionOf is the terminal cell where the first occurrence of the text is drawn, which is
// where the pointer goes to be on it.
func positionOf(t *testing.T, model *plugin.Model, text string) (x, y int) {
	t.Helper()
	for y, line := range strings.Split(view(model), "\n") {
		if index := strings.Index(line, text); index >= 0 {
			return lipgloss.Width(line[:index]), y
		}
	}
	t.Fatalf("nothing shows %q:\n%s", text, view(model))
	return 0, 0
}

func sendMouse(t *testing.T, model *plugin.Model, button tea.MouseButton, action tea.MouseAction, x, y int) {
	t.Helper()
	plugintest.Send(t, model, tea.MouseMsg{X: x, Y: y, Button: button, Action: action})
}

func clickAt(t *testing.T, model *plugin.Model, x, y int) {
	t.Helper()
	sendMouse(t, model, tea.MouseButtonLeft, tea.MouseActionPress, x, y)
	sendMouse(t, model, tea.MouseButtonLeft, tea.MouseActionRelease, x, y)
}

// startDrag presses at the first position and moves with the button held to the second.
func startDrag(t *testing.T, model *plugin.Model, fromX, fromY, toX, toY int) {
	t.Helper()
	sendMouse(t, model, tea.MouseButtonLeft, tea.MouseActionPress, fromX, fromY)
	sendMouse(t, model, tea.MouseButtonLeft, tea.MouseActionMotion, toX, toY)
}

func dragBetween(t *testing.T, model *plugin.Model, fromX, fromY, toX, toY int) {
	t.Helper()
	startDrag(t, model, fromX, fromY, toX, toY)
	sendMouse(t, model, tea.MouseButtonLeft, tea.MouseActionRelease, toX, toY)
}

// codeOfKind is the code of the first fixture file's lines of the kind, in order, as it is
// shown after the marker.
func codeOfKind(kind lineKind) []string {
	var code []string
	for _, line := range parseDiff(fixtureDiff)[0].lines {
		if line.kind == kind {
			code = append(code, line.text[1:])
		}
	}
	return code
}

func TestClickingADiffLineMovesTheCursorToIt(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter")
	x, y := positionOf(t, model, "2 +"+codeOfKind(addedLine)[0])

	clickAt(t, model, x, y)

	if !cursorIsOnTheAddedLine(t, model) {
		t.Fatalf("the cursor is not on the clicked line:\n%s", view(model))
	}
}

func TestClickingAFileInTheListShowsThatFile(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter")
	x, y := positionOf(t, model, "new_name.py")

	clickAt(t, model, x, y)

	if !strings.Contains(view(model), "file 2 of 2") {
		t.Fatalf("the clicked file is not shown:\n%s", view(model))
	}
}

func TestClickingTheHeaderDoesNotMoveTheCursor(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)

	clickAt(t, model, 0, 0)

	if !cursorIsOnTheAddedLine(t, model) {
		t.Fatalf("the cursor moved:\n%s", view(model))
	}
}

func TestDraggingOverTwoAddedLinesCopiesTheirCodeFromTheNewSide(t *testing.T) {
	copied := replaceClipboard(t)
	model := loadedBoard(t)
	press(t, model, "enter")
	added := codeOfKind(addedLine)
	fromX, fromY := positionOf(t, model, "+"+added[0])
	toX, toY := positionOf(t, model, "+"+added[1])

	dragBetween(t, model, fromX, fromY, toX, toY)

	if expected := strings.Join(added, "\n"); len(*copied) != 1 || (*copied)[0] != expected {
		t.Fatalf("copied = %q, expected %q", *copied, expected)
	}
	if !strings.Contains(view(model), "Copied 2 lines") {
		t.Fatalf("the copy is not reported:\n%s", view(model))
	}
}

func TestDraggingOnTheOldSideCopiesTheRemovedCodeAndSkipsTheEmptySide(t *testing.T) {
	copied := replaceClipboard(t)
	model := loadedBoard(t)
	press(t, model, "enter")
	removed := codeOfKind(removedLine)
	fromX, fromY := positionOf(t, model, "-"+removed[0])
	_, toY := positionOf(t, model, "+"+codeOfKind(addedLine)[1])

	dragBetween(t, model, fromX, fromY, fromX, toY)

	if len(*copied) != 1 || (*copied)[0] != removed[0] {
		t.Fatalf("copied = %q, expected %q", *copied, removed[0])
	}
}

func TestDraggingFromTheOldSideDoesNotCopyTheAddedCode(t *testing.T) {
	copied := replaceClipboard(t)
	model := loadedBoard(t)
	press(t, model, "enter")
	fromX, fromY := positionOf(t, model, "-"+codeOfKind(removedLine)[0])
	_, toY := positionOf(t, model, "+"+codeOfKind(addedLine)[1])

	dragBetween(t, model, fromX, fromY, fromX, toY)

	for _, text := range *copied {
		for _, added := range codeOfKind(addedLine) {
			if strings.Contains(text, added) {
				t.Fatalf("copied %q, which has the added line %q", text, added)
			}
		}
	}
}

func TestDraggingOverALineThatWrapsCopiesItOnce(t *testing.T) {
	copied := replaceClipboard(t)
	provider := newFakeProvider()
	provider.diff = wrapDiff()
	model := loadedBoardWith(t, provider)
	plugintest.Resize(t, model, 50, 30)
	press(t, model, "enter")
	x, firstRow := positionOf(t, model, "-"+longCode[:10])

	dragBetween(t, model, x, firstRow, x, firstRow+1)

	if len(*copied) != 1 || (*copied)[0] != longCode {
		t.Fatalf("copied = %q, expected %q once:\n%s", *copied, longCode, view(model))
	}
}

func TestDraggingOverCommentsAndTheHunkHeaderCopiesOnlyCode(t *testing.T) {
	copied := replaceClipboard(t)
	model := loadedBoard(t)
	press(t, model, "enter")
	added := codeOfKind(addedLine)
	toX, toY := positionOf(t, model, "+"+added[1])
	_, hunkRow := positionOf(t, model, "@@")

	dragBetween(t, model, toX, hunkRow, toX, toY)

	expected := strings.Join(append(codeOfKind(contextLine), added...), "\n")
	if len(*copied) != 1 || (*copied)[0] != expected {
		t.Fatalf("copied = %q, expected %q", *copied, expected)
	}
	if !strings.Contains(view(model), "Why this change?") {
		t.Fatalf("the fixture's comment is not in the range:\n%s", view(model))
	}
}

func TestClickingWithoutDraggingCopiesNothing(t *testing.T) {
	copied := replaceClipboard(t)
	model := loadedBoard(t)
	press(t, model, "enter")
	x, y := positionOf(t, model, "+"+codeOfKind(addedLine)[0])

	clickAt(t, model, x, y)

	if len(*copied) != 0 {
		t.Fatalf("copied = %q", *copied)
	}
}

func TestPressingOnTheOldSideChoosesThatSideForComments(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter")
	oldX, y := positionOf(t, model, "-"+codeOfKind(removedLine)[0])
	newX, _ := positionOf(t, model, "+"+codeOfKind(addedLine)[0])

	removedCode := "-" + codeOfKind(removedLine)[0]
	// The cursor marker sits left of the old side's code, or between the two sides.
	markerIsBeforeTheOldSide := func() bool {
		line := lineWith(t, model, removedCode)
		return strings.Contains(line, ui.CursorMarker) && strings.Index(line, ui.CursorMarker) < strings.Index(line, removedCode)
	}

	clickAt(t, model, oldX, y)
	afterPressingTheOldSide := markerIsBeforeTheOldSide()
	clickAt(t, model, newX, y)
	afterPressingTheNewSide := markerIsBeforeTheOldSide()

	if !afterPressingTheOldSide || afterPressingTheNewSide {
		t.Fatalf("the cursor marker is before the old side: %v after pressing it, %v after pressing the new side:\n%s",
			afterPressingTheOldSide, afterPressingTheNewSide, view(model))
	}
}

func TestSelectedRowsAreHighlightedOnTheChosenSideOnly(t *testing.T) {
	showColors(t)
	model := loadedBoard(t)
	press(t, model, "enter")
	removed, added := codeOfKind(removedLine)[0], codeOfKind(addedLine)[0]
	fromX, fromY := positionOf(t, model, "-"+removed)
	_, toY := positionOf(t, model, "+"+codeOfKind(addedLine)[1])

	startDrag(t, model, fromX, fromY, fromX, toY)

	var pairRow string
	for _, line := range strings.Split(model.View(), "\n") {
		if strings.Contains(line, removed) && strings.Contains(line, added) {
			pairRow = line
		}
	}
	highlightedAt := strings.LastIndex(pairRow, reverseVideo)
	if highlightedAt < 0 || highlightedAt > strings.Index(pairRow, removed) || strings.Count(pairRow, reverseVideo) != 1 {
		t.Fatalf("the old side is not the only highlight in %q", pairRow)
	}
}

func TestTheHighlightStaysAfterReleaseAndClearsOnTheNextKey(t *testing.T) {
	showColors(t)
	replaceClipboard(t)
	model := loadedBoard(t)
	press(t, model, "enter")
	added := codeOfKind(addedLine)
	fromX, fromY := positionOf(t, model, "+"+added[0])
	toX, toY := positionOf(t, model, "+"+added[1])

	highlightsBefore := strings.Count(model.View(), reverseVideo)

	dragBetween(t, model, fromX, fromY, toX, toY)
	highlightsAfterRelease := strings.Count(model.View(), reverseVideo)
	press(t, model, "down")

	if highlightsAfterRelease <= highlightsBefore || strings.Count(model.View(), reverseVideo) != highlightsBefore {
		t.Fatalf("highlights: %d before, %d after the release, %d after a key", highlightsBefore, highlightsAfterRelease, strings.Count(model.View(), reverseVideo))
	}
}

func TestPressingElsewhereClearsTheHighlight(t *testing.T) {
	showColors(t)
	replaceClipboard(t)
	model := loadedBoard(t)
	press(t, model, "enter")
	added := codeOfKind(addedLine)
	fromX, fromY := positionOf(t, model, "+"+added[0])
	toX, toY := positionOf(t, model, "+"+added[1])
	highlightsBefore := strings.Count(model.View(), reverseVideo)
	dragBetween(t, model, fromX, fromY, toX, toY)

	clickAt(t, model, fromX, fromY)

	if highlights := strings.Count(model.View(), reverseVideo); highlights != highlightsBefore {
		t.Fatalf("%d highlights after a click, expected the %d before the drag", highlights, highlightsBefore)
	}
}

func TestDraggingInTheUnifiedLayoutCopiesEveryCodeLineCovered(t *testing.T) {
	copied := replaceClipboard(t)
	model := loadedBoard(t)
	press(t, model, "enter", "s")
	removed, added := codeOfKind(removedLine), codeOfKind(addedLine)
	x, fromY := positionOf(t, model, "-"+removed[0])
	_, toY := positionOf(t, model, "+"+added[1])

	dragBetween(t, model, x, fromY, x, toY)

	expected := strings.Join(append(removed, added...), "\n")
	if len(*copied) != 1 || (*copied)[0] != expected {
		t.Fatalf("copied = %q, expected %q", *copied, expected)
	}
}
