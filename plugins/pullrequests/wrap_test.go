package pullrequests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	longCode  = "alpha_bravo_charlie_delta_echo_foxtrot_golf_hotel_india_juliet"
	otherCode = "kilo_lima_mike_november_oscar_papa_quebec_romeo_sierra_tango"
)

// Indexes of the diff lines of wrapDiff, after the two file header lines and the hunk header.
const (
	firstLineIndex   = 3
	removedLineIndex = 4
	addedLineIndex   = 5
	lastLineIndex    = 6
	pairRowIndex     = 4
)

// wrapDiff has a short line, then a removed and an added line that are both too long for a
// narrow side, then a short line again.
func wrapDiff() string {
	return strings.Join([]string{
		"diff --git a/wrapped.txt b/wrapped.txt",
		"--- a/wrapped.txt",
		"+++ b/wrapped.txt",
		"@@ -1,3 +1,3 @@",
		" first",
		"-" + longCode,
		"+" + otherCode + "_and_more_text_to_make_this_side_taller",
		" last",
	}, "\n")
}

// wrappingScreen is a diff screen over the text with nothing else around it.
func wrappingScreen(text string, splitDiff bool, width, height int) *diffScreen {
	screen := &diffScreen{files: parseDiff(text), splitDiff: splitDiff, width: width, height: height}
	screen.keepPositionVisible()
	return screen
}

func (screen *diffScreen) renderedRows() []string {
	rendered := []string{}
	for index := range screen.rows {
		rendered = append(rendered, ansi.Strip(screen.renderDiffRow(index, screen.diffWidth())))
	}
	return rendered
}

func TestUnifiedViewShowsALongLineInFullAcrossSeveralLines(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)

	rows := screen.renderedRows()

	joined := strings.Join(rows, "")
	if !strings.Contains(strings.ReplaceAll(joined, " ", ""), longCode) {
		t.Fatalf("the long line is cut off:\n%s", strings.Join(rows, "\n"))
	}
	if strings.Contains(joined, "…") {
		t.Fatalf("nothing should be truncated:\n%s", strings.Join(rows, "\n"))
	}
}

func TestUnifiedViewGivesAWrappedLineMoreThanOneRow(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)

	rows := screen.renderedRows()

	if len(rows) <= len(screen.files[0].lines) {
		t.Fatalf("expected more rows than lines, got %d", len(rows))
	}
}

func TestUnifiedViewKeepsAShortLineOnOneRow(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)

	rowsOfFirstLine := 0
	for _, row := range screen.rows {
		if row.left == firstLineIndex {
			rowsOfFirstLine++
		}
	}

	if rowsOfFirstLine != 1 {
		t.Fatalf("a short line took %d rows", rowsOfFirstLine)
	}
}

func TestUnifiedViewIndentsWrappedPartsPastTheMarkerColumn(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)

	rows := screen.renderedRows()

	removedRow := screen.rowForLine(removedLineIndex)
	if !strings.HasPrefix(rows[removedRow], "-") || !strings.HasPrefix(rows[removedRow+1], " ") {
		t.Fatalf("rows = %q", rows[removedRow:removedRow+2])
	}
}

func TestSplitViewPadsEveryWrappedLineOfARowToTheFullWidth(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), true, 60, 30)
	file := screen.files[0]
	pair := file.splitRows[pairRowIndex]

	lines := file.renderSplitRow(pair, screen.diffWidth(), " │ ")

	if len(lines) < 2 {
		t.Fatalf("the fixture should wrap, got %d lines", len(lines))
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got != splitSideWidth(screen.diffWidth())*2+3 {
			t.Fatalf("line %q has %d cells", ansi.Strip(line), got)
		}
	}
}

func TestSplitViewShowsEachLineNumberOnTheFirstWrappedLineOnly(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), true, 60, 30)
	file := screen.files[0]
	pair := file.splitRows[pairRowIndex]
	left, right := file.lines[pair.left], file.lines[pair.right]

	lines := file.renderSplitRow(pair, screen.diffWidth(), " │ ")

	first := ansi.Strip(lines[0])
	if !strings.Contains(first, fmt.Sprintf("%d ", left.oldNumber)) || !strings.Contains(first, fmt.Sprintf("%d ", right.newNumber)) {
		t.Fatalf("the first line should carry both numbers: %q", first)
	}
	for _, continuation := range lines[1:] {
		if strings.ContainsAny(ansi.Strip(continuation), "0123456789") {
			t.Fatalf("a wrapped part repeats a line number: %q", ansi.Strip(continuation))
		}
	}
}

func TestSplitRowIsAsTallAsItsTallerSide(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), true, 60, 30)
	file := screen.files[0]
	pair := file.splitRows[pairRowIndex]
	codeWidth := file.sideCodeWidth(splitSideWidth(screen.diffWidth()))

	height := file.splitRowHeight(pair, screen.diffWidth())

	leftParts, rightParts := len(file.lines[pair.left].parts(codeWidth)), len(file.lines[pair.right].parts(codeWidth))
	if leftParts == rightParts {
		t.Fatalf("the fixture should wrap the sides differently, both have %d", leftParts)
	}
	if height != max(leftParts, rightParts) {
		t.Fatalf("height = %d, sides have %d and %d", height, leftParts, rightParts)
	}
}

func TestSplitViewWrapsAHunkHeaderAcrossTheFullWidth(t *testing.T) {
	text := strings.Replace(wrapDiff(), "@@ -1,3 +1,3 @@", "@@ -1,3 +1,3 @@ "+longCode+otherCode, 1)
	screen := wrappingScreen(text, true, 50, 30)

	rows := strings.Join(screen.renderedRows(), "")

	if !strings.Contains(rows, otherCode) || strings.Contains(rows, "…") {
		t.Fatalf("hunk header is cut off: %q", rows)
	}
}

func TestJMovesPastTheContinuationPartsOfAWrappedLine(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)
	removedRow := screen.rowForLine(removedLineIndex)
	screen.diffCursor = removedRow

	screen.handleKey("j")
	screen.keepPositionVisible()

	if screen.cursorLine() != addedLineIndex || screen.rows[screen.diffCursor].part != 0 {
		t.Fatalf("cursor is on line %d, part %d", screen.cursorLine(), screen.rows[screen.diffCursor].part)
	}
	if screen.diffCursor-removedRow < 2 {
		t.Fatalf("the cursor moved only %d rows", screen.diffCursor-removedRow)
	}
}

func TestKMovesBackToTheFirstPartOfAWrappedLine(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)
	screen.diffCursor = screen.rowForLine(addedLineIndex)

	screen.handleKey("k")
	screen.keepPositionVisible()

	if screen.diffCursor != screen.rowForLine(removedLineIndex) {
		t.Fatalf("cursor is on row %d, expected %d", screen.diffCursor, screen.rowForLine(removedLineIndex))
	}
}

func TestJOnTheLastLineStaysOnItsFirstPart(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)
	screen.diffCursor = screen.rowForLine(lastLineIndex)

	screen.handleKey("j")
	screen.keepPositionVisible()

	if screen.diffCursor != screen.rowForLine(lastLineIndex) {
		t.Fatalf("cursor moved to row %d", screen.diffCursor)
	}
}

func TestEndLandsOnTheFirstPartOfTheLastLine(t *testing.T) {
	text := strings.Replace(wrapDiff(), " last", " "+longCode+otherCode, 1)
	screen := wrappingScreen(text, false, 50, 30)

	screen.handleKey("end")
	screen.keepPositionVisible()

	if screen.rows[screen.diffCursor].part != 0 || screen.cursorLine() != lastLineIndex {
		t.Fatalf("cursor is on line %d, part %d", screen.cursorLine(), screen.rows[screen.diffCursor].part)
	}
}

func TestMovingDownOntoAWrappedLineScrollsItsPartsIntoView(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 8)
	screen.diffCursor = screen.rowForLine(firstLineIndex)
	screen.diffOffset = screen.diffCursor
	for range 2 {
		screen.handleKey("j")
		screen.keepPositionVisible()
	}

	lastPart := screen.diffCursor
	for lastPart+1 < len(screen.rows) && screen.rows[lastPart+1].part > 0 {
		lastPart++
	}

	if lastPart >= screen.diffOffset+screen.diffRows() {
		t.Fatalf("the last part is on row %d but rows %d to %d are visible", lastPart, screen.diffOffset, screen.diffOffset+screen.diffRows()-1)
	}
}

func TestCursorMarkerIsOnlyOnTheFirstPartOfAWrappedLine(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)
	screen.diffCursor = screen.rowForLine(removedLineIndex)

	first, second := screen.diffSeparator(screen.diffCursor), screen.diffSeparator(screen.diffCursor+1)

	if !strings.Contains(first, "▌") || strings.Contains(second, "▌") {
		t.Fatalf("separators = %q and %q", first, second)
	}
}

func TestCommentOnAWrappedLineTargetsThatLine(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)
	screen.diffCursor = screen.rowForLine(addedLineIndex)

	target, isCodeLine := screen.cursorCommentTarget()

	added := screen.files[0].lines[addedLineIndex]
	if !isCodeLine || target.Line != added.newNumber || target.Side != CommentSideNew {
		t.Fatalf("target = %+v", target)
	}
}

// markRows leaves a mark on the built rows, which a rebuild replaces.
func markRows(screen *diffScreen) {
	screen.rows[0].comment = "built before"
}

func TestRowsAreKeptWhenOnlyTheCursorMoves(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), true, 60, 30)
	markRows(screen)

	screen.handleKey("j")
	screen.keepPositionVisible()

	if screen.rows[0].comment != "built before" {
		t.Fatal("rows were rebuilt by a cursor move")
	}
}

func TestRowsAreRebuiltWhenTheWidthChanges(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), true, 60, 30)
	markRows(screen)

	screen.width = 80
	screen.keepPositionVisible()

	if screen.rows[0].comment != "" {
		t.Fatal("rows were not rebuilt for the new width")
	}
}

func TestRowsAreRebuiltWhenTheLayoutChanges(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), true, 60, 30)
	markRows(screen)

	screen.handleKey("s")
	screen.keepPositionVisible()

	if screen.rows[0].comment != "" {
		t.Fatal("rows were not rebuilt for the new layout")
	}
}

func TestResizingKeepsTheCursorOnItsLine(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)
	screen.diffCursor = screen.rowForLine(addedLineIndex)

	screen.width = 90
	screen.keepPositionVisible()

	if screen.cursorLine() != addedLineIndex {
		t.Fatalf("cursor is on line %d", screen.cursorLine())
	}
}
