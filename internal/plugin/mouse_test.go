package plugin

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// replaceClipboard has the host hand copied text to the returned recorder, which fails with
// the given error, until the test ends.
func replaceClipboard(t *testing.T, err error) *[]string {
	t.Helper()
	var copied []string
	original := shell.WriteClipboard
	shell.WriteClipboard = func(text string) error {
		copied = append(copied, text)
		return err
	}
	t.Cleanup(func() { shell.WriteClipboard = original })
	return &copied
}

// replaceClock makes the host's clock the returned time, which a test moves forward, until the
// test ends.
func replaceClock(t *testing.T) *time.Time {
	t.Helper()
	current := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	original := now
	now = func() time.Time { return current }
	t.Cleanup(func() { now = original })
	return &current
}

// cellPosition is where a cell of the loaded board is drawn, from the same layout the host
// draws and hit-tests with.
func cellPosition(model *Model, rowIndex, column int) (x, y int) {
	board := model.current()
	widths := columnWidths(board.spec.Columns, board.cells(), model.width)
	for _, width := range widths[:column] {
		x += width + len(columnGap)
	}
	return x, model.paneOf(model.active).headerLine() + 1 + rowIndex - board.offset
}

// headerPosition is where the table's header is drawn.
func headerPosition(model *Model) (x, y int) {
	return 0, model.paneOf(model.active).headerLine()
}

func click(t *testing.T, model *Model, x, y int) {
	t.Helper()
	plugintest.Send(t, model, tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
}

func clickCell(t *testing.T, model *Model, rowIndex, column int) {
	t.Helper()
	x, y := cellPosition(model, rowIndex, column)
	click(t, model, x, y)
}

func wheel(t *testing.T, model *Model, button tea.MouseButton) {
	t.Helper()
	plugintest.Send(t, model, tea.MouseMsg{X: 0, Y: 5, Button: button, Action: tea.MouseActionPress})
}

func loadedGroceries(t *testing.T) *Model {
	t.Helper()
	return modelForBoard(t, groceryBoard(newRowSource(groceries()...)))
}

func TestDoubleClickOnACellCopiesItsTextAndShowsItInTheStatusLine(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	model := loadedGroceries(t)
	expected := model.current().visible[1].(fakeRow).group

	clickCell(t, model, 1, 1)
	clickCell(t, model, 1, 1)

	if len(*copied) != 1 || (*copied)[0] != expected {
		t.Fatalf("copied = %v, expected %q", *copied, expected)
	}
	if status := statusLine(model); !strings.Contains(status, "Copied "+expected) {
		t.Fatalf("status = %q", status)
	}
}

func TestDoubleClickOnACellCutByMaxWidthCopiesTheUncutText(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	board := groceryBoard(newRowSource(groceries()...))
	board.Columns[1].MaxWidth = 3
	model := modelForBoard(t, board)
	expected := model.current().visible[0].(fakeRow).group

	clickCell(t, model, 0, 1)
	clickCell(t, model, 0, 1)

	if !strings.Contains(screenText(model), "fr…") || len(*copied) != 1 || (*copied)[0] != expected {
		t.Fatalf("copied = %v, expected %q\n%s", *copied, expected, screenText(model))
	}
}

func TestDoubleClickOnAnEmptyCellCopiesNothing(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	board := groceryBoard(newRowSource(groceries()...))
	board.Columns = append(board.Columns, Column{Header: "Note", Cell: func(Row, any) []ui.Segment { return nil }})
	model := modelForBoard(t, board)
	last := len(board.Columns) - 1

	clickCell(t, model, 0, last)
	clickCell(t, model, 0, last)

	if len(*copied) != 0 {
		t.Fatalf("copied = %v", *copied)
	}
}

func TestThirdClickRightAfterADoubleClickCopiesNothingMore(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	model := loadedGroceries(t)

	clickCell(t, model, 0, 0)
	clickCell(t, model, 0, 0)
	clickCell(t, model, 0, 0)

	if len(*copied) != 1 {
		t.Fatalf("copied = %v", *copied)
	}
}

func TestTwoClicksOnACellFurtherApartThanTheDoubleClickIntervalCopyNothing(t *testing.T) {
	copied := replaceClipboard(t, nil)
	clock := replaceClock(t)
	model := loadedGroceries(t)

	clickCell(t, model, 0, 0)
	*clock = clock.Add(doubleClickInterval + time.Millisecond)
	clickCell(t, model, 0, 0)

	if len(*copied) != 0 {
		t.Fatalf("copied = %v", *copied)
	}
}

func TestTwoClicksJustInsideTheDoubleClickIntervalCopyTheCell(t *testing.T) {
	copied := replaceClipboard(t, nil)
	clock := replaceClock(t)
	model := loadedGroceries(t)

	clickCell(t, model, 0, 0)
	*clock = clock.Add(doubleClickInterval)
	clickCell(t, model, 0, 0)

	if len(*copied) != 1 {
		t.Fatalf("copied = %v", *copied)
	}
}

func TestTwoQuickClicksOnDifferentColumnsOfARowCopyNothing(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	model := loadedGroceries(t)

	clickCell(t, model, 0, 0)
	clickCell(t, model, 0, 1)

	if len(*copied) != 0 {
		t.Fatalf("copied = %v", *copied)
	}
}

func TestTwoQuickClicksOnDifferentRowsOfAColumnCopyNothing(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	model := loadedGroceries(t)

	clickCell(t, model, 0, 0)
	clickCell(t, model, 1, 0)

	if len(*copied) != 0 {
		t.Fatalf("copied = %v", *copied)
	}
}

func TestSingleClickOnARowMovesTheCursorToIt(t *testing.T) {
	replaceClipboard(t, nil)
	replaceClock(t)
	model := loadedGroceries(t)
	expected := model.current().visible[2].(fakeRow).name

	clickCell(t, model, 2, 0)

	if selectedName(t, model) != expected {
		t.Fatalf("selected = %q, expected %q", selectedName(t, model), expected)
	}
}

func TestClickOnTheHeaderLeavesTheCursorWhereItWas(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	model := loadedGroceries(t)
	plugintest.Press(t, model, "down")
	expected := selectedName(t, model)
	x, y := headerPosition(model)

	click(t, model, x, y)
	click(t, model, x, y)

	if selectedName(t, model) != expected || len(*copied) != 0 {
		t.Fatalf("selected = %q, expected %q, copied = %v", selectedName(t, model), expected, *copied)
	}
}

func TestClicksOnTheGapBetweenColumnsAndBelowTheLastRowDoNothing(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	model := loadedGroceries(t)
	expected := selectedName(t, model)
	gapX, gapY := cellPosition(model, 1, 1)
	gapX -= len(columnGap)
	belowX, belowY := cellPosition(model, len(model.current().visible), 0)

	click(t, model, gapX, gapY)
	click(t, model, gapX, gapY)
	click(t, model, belowX, belowY)
	click(t, model, belowX, belowY)

	if selectedName(t, model) != expected || len(*copied) != 0 {
		t.Fatalf("selected = %q, expected %q, copied = %v", selectedName(t, model), expected, *copied)
	}
}

func TestMouseWheelMovesTheCursorOneRowAtATime(t *testing.T) {
	model := loadedGroceries(t)
	rows := model.current().visible

	wheel(t, model, tea.MouseButtonWheelDown)
	afterDown := selectedName(t, model)
	wheel(t, model, tea.MouseButtonWheelDown)
	wheel(t, model, tea.MouseButtonWheelUp)

	if afterDown != rows[1].(fakeRow).name || selectedName(t, model) != afterDown {
		t.Fatalf("after one wheel down = %q, after two down and one up = %q", afterDown, selectedName(t, model))
	}
}

func TestMouseWheelUpAtTheFirstRowKeepsTheCursorThere(t *testing.T) {
	model := loadedGroceries(t)
	expected := selectedName(t, model)

	wheel(t, model, tea.MouseButtonWheelUp)

	if selectedName(t, model) != expected {
		t.Fatalf("selected = %q, expected %q", selectedName(t, model), expected)
	}
}

func TestClicksWhileTheFilterMenuIsOpenCopyNothingAndLeaveTheCursor(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	model := loadedGroceries(t)
	expected := selectedName(t, model)
	x, y := cellPosition(model, 1, 0)
	plugintest.Press(t, model, "f")

	click(t, model, x, y)
	click(t, model, x, y)
	wheel(t, model, tea.MouseButtonWheelDown)

	if selectedName(t, model) != expected || len(*copied) != 0 {
		t.Fatalf("selected = %q, expected %q, copied = %v", selectedName(t, model), expected, *copied)
	}
}

func TestClicksWhileAScreenIsOpenReachTheScreenAndNotTheBoard(t *testing.T) {
	copied := replaceClipboard(t, nil)
	replaceClock(t)
	screen := &fakeScreen{title: "details"}
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Screen: screen} }))
	expected := selectedName(t, model)
	x, y := cellPosition(model, 1, 0)
	plugintest.Press(t, model, "x")

	click(t, model, x, y)
	click(t, model, x, y)

	var mouseMessages int
	for _, message := range screen.received {
		if _, isMouse := message.(tea.MouseMsg); isMouse {
			mouseMessages++
		}
	}
	if mouseMessages != 2 || selectedName(t, model) != expected || len(*copied) != 0 {
		t.Fatalf("screen got %d mouse messages, selected = %q, copied = %v", mouseMessages, selectedName(t, model), *copied)
	}
}

func TestFailedClipboardWriteShowsFailedWithTheFirstLineOnly(t *testing.T) {
	replaceClipboard(t, errorWithTwoLines)
	replaceClock(t)
	model := loadedGroceries(t)

	clickCell(t, model, 0, 0)
	clickCell(t, model, 0, 0)

	status := statusLine(model)
	if !strings.Contains(status, "Failed: first line") || strings.Contains(status, "second line") || strings.Contains(status, "Copied") {
		t.Fatalf("status = %q", status)
	}
}

func TestMouseMessagesReachOnlyTheTopScreen(t *testing.T) {
	lower := &fakeScreen{title: "lower"}
	upper := &fakeScreen{title: "upper"}
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect { return Effect{Screen: lower} }))
	plugintest.Press(t, model, "x")
	plugintest.Send(t, model, effectMessage{board: 0, effect: Effect{Screen: upper}})

	wheel(t, model, tea.MouseButtonWheelDown)

	mouseMessages := func(screen *fakeScreen) (count int) {
		for _, message := range screen.received {
			if _, isMouse := message.(tea.MouseMsg); isMouse {
				count++
			}
		}
		return count
	}
	if mouseMessages(upper) != 1 || mouseMessages(lower) != 0 {
		t.Fatalf("upper got %d mouse messages and lower got %d", mouseMessages(upper), mouseMessages(lower))
	}
}
