package ui

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestFitPadsShortTextToTheWidth(t *testing.T) {
	if fitted := Fit("abc", 6); fitted != "abc   " {
		t.Fatalf("fitted = %q", fitted)
	}
}

func TestFitTruncatesLongTextWithAnEllipsis(t *testing.T) {
	if fitted := Fit("abcdefgh", 5); fitted != "abcd…" {
		t.Fatalf("fitted = %q", fitted)
	}
}

func TestFitCountsDisplayWidthNotBytes(t *testing.T) {
	if fitted := Fit("日本", 6); fitted != "日本  " {
		t.Fatalf("fitted = %q", fitted)
	}
}

func TestPlainTextJoinsSegmentsWithoutStyling(t *testing.T) {
	segments := []Segment{{Text: "+3", Style: GreenStyle}, {Text: " ", Style: PlainStyle}, {Text: "−1", Style: RedStyle}}

	if text := PlainText(segments); text != "+3 −1" {
		t.Fatalf("plain text = %q", text)
	}
}

func TestStyledTextKeepsTheSameWordsAsPlainText(t *testing.T) {
	segments := []Segment{{Text: "+3", Style: GreenStyle}, {Text: " ", Style: PlainStyle}, {Text: "−1", Style: RedStyle}}

	if text := ansi.Strip(StyledText(segments)); text != PlainText(segments) {
		t.Fatalf("styled text without styling = %q, plain text = %q", text, PlainText(segments))
	}
}

func TestHelpLinePairsEachKeyWithItsLabel(t *testing.T) {
	if line := ansi.Strip(HelpLine("enter", "diff", "q", "quit")); line != "enter diff · q quit" {
		t.Fatalf("help line = %q", line)
	}
}

func TestAgeTextUsesMinutesThenHoursThenDays(t *testing.T) {
	cases := map[time.Duration]string{
		5 * time.Minute:  "5m",
		59 * time.Minute: "59m",
		60 * time.Minute: "1h",
		23 * time.Hour:   "23h",
		24 * time.Hour:   "1d",
		72 * time.Hour:   "3d",
	}

	for age, expected := range cases {
		if text := AgeText(time.Now().Add(-age - time.Second)); text != expected {
			t.Errorf("age text for %v = %q, expected %q", age, text, expected)
		}
	}
}

func TestFirstLineKeepsOnlyTheFirstLineOfAnError(t *testing.T) {
	if line := FirstLine(errors.New("first\nsecond")); line != "first" {
		t.Fatalf("first line = %q", line)
	}
}

func TestFirstLineLeavesASingleLineErrorAlone(t *testing.T) {
	if line := FirstLine(errors.New("only")); line != "only" {
		t.Fatalf("first line = %q", line)
	}
}

func wrappedText(lines [][]Segment) []string {
	text := []string{}
	for _, line := range lines {
		text = append(text, PlainText(line))
	}
	return text
}

func TestWrapSegmentsSplitsAtTheWidth(t *testing.T) {
	lines := wrappedText(WrapSegments(PlainSegments("abcdefgh"), 3))

	if !reflect.DeepEqual(lines, []string{"abc", "def", "gh"}) {
		t.Fatalf("lines = %q", lines)
	}
}

func TestWrapSegmentsLeavesTextThatFitsOnOneLine(t *testing.T) {
	lines := wrappedText(WrapSegments(PlainSegments("abc"), 3))

	if !reflect.DeepEqual(lines, []string{"abc"}) {
		t.Fatalf("lines = %q", lines)
	}
}

func TestWrapSegmentsKeepsEachPiecesStyleAcrossASplit(t *testing.T) {
	segments := []Segment{{Text: "ab", Style: GreenStyle}, {Text: "cdef", Style: RedStyle}}

	lines := WrapSegments(segments, 3)

	if len(lines) != 2 || len(lines[0]) != 2 || len(lines[1]) != 1 {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[0][1].Text != "c" || lines[0][1].Style.GetForeground() != RedStyle.GetForeground() {
		t.Fatalf("first line's second piece = %+v", lines[0][1])
	}
	if lines[1][0].Text != "def" || lines[1][0].Style.GetForeground() != RedStyle.GetForeground() {
		t.Fatalf("second line = %+v", lines[1])
	}
	if lines[0][0].Style.GetForeground() != GreenStyle.GetForeground() {
		t.Fatalf("first piece lost its style: %+v", lines[0][0])
	}
}

func TestWrapSegmentsMovesAWideCharacterThatDoesNotFitToTheNextLine(t *testing.T) {
	lines := WrapSegments(PlainSegments("ab日cd"), 3)

	for _, line := range lines {
		if lipgloss.Width(PlainText(line)) > 3 {
			t.Fatalf("line %q is wider than 3 cells", PlainText(line))
		}
	}
	if !reflect.DeepEqual(wrappedText(lines), []string{"ab", "日c", "d"}) {
		t.Fatalf("lines = %q", wrappedText(lines))
	}
}

func TestWrapSegmentsGivesEmptyInputOneEmptyLine(t *testing.T) {
	lines := WrapSegments(nil, 5)

	if len(lines) != 1 || len(lines[0]) != 0 {
		t.Fatalf("lines = %+v", lines)
	}
}

func TestWrapSegmentsTreatsAWidthBelowOneAsOne(t *testing.T) {
	lines := wrappedText(WrapSegments(PlainSegments("abc"), 0))

	if !reflect.DeepEqual(lines, []string{"a", "b", "c"}) {
		t.Fatalf("lines = %q", lines)
	}
}
