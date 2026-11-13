package ui

import (
	"errors"
	"testing"
	"time"

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
