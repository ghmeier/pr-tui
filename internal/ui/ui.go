// Package ui holds the terminal styling and text helpers shared by the views.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// DarkBackground is read once at startup, because asking the terminal for its background
// colour while the board is reading keys would swallow the terminal's reply.
var DarkBackground = lipgloss.HasDarkBackground()

var (
	AddedBackground   = lipgloss.AdaptiveColor{Light: "#dafbe1", Dark: "#12261e"}
	RemovedBackground = lipgloss.AdaptiveColor{Light: "#ffebe9", Dark: "#2d1517"}
)

var (
	PlainStyle    = lipgloss.NewStyle()
	BoldStyle     = lipgloss.NewStyle().Bold(true)
	DimStyle      = lipgloss.NewStyle().Faint(true)
	GreenStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	RedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	YellowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	CyanStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	KeyStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	SelectedStyle = lipgloss.NewStyle().Reverse(true)
	CursorMarker  = KeyStyle.Render("▌")
	MenuStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2)
)

// Segment is a piece of text with one style, so a cell can be measured as plain text and
// drawn as styled text.
type Segment struct {
	Text  string
	Style lipgloss.Style
}

// PlainSegments is text in the default style, as a one-segment cell.
func PlainSegments(text string) []Segment {
	return []Segment{{Text: text, Style: PlainStyle}}
}

func PlainText(segments []Segment) string {
	var text strings.Builder
	for _, part := range segments {
		text.WriteString(part.Text)
	}
	return text.String()
}

func StyledText(segments []Segment) string {
	var text strings.Builder
	for _, part := range segments {
		text.WriteString(part.Style.Render(part.Text))
	}
	return text.String()
}

// WrapSegments splits styled text into lines of at most width cells, cutting a segment where the
// line fills and keeping each piece's style. A character wider than the width stays alone on its
// line, so no input is dropped.
func WrapSegments(segments []Segment, width int) [][]Segment {
	width = max(width, 1)
	lines := [][]Segment{{}}
	used := 0
	for _, segment := range segments {
		var piece strings.Builder
		flush := func() {
			if piece.Len() > 0 {
				last := len(lines) - 1
				lines[last] = append(lines[last], Segment{Text: piece.String(), Style: segment.Style})
				piece.Reset()
			}
		}
		for rest := segment.Text; rest != ""; {
			var cluster string
			var cells int
			cluster, rest, cells, _ = uniseg.FirstGraphemeClusterInString(rest, -1)
			if used > 0 && used+cells > width {
				flush()
				lines = append(lines, []Segment{})
				used = 0
			}
			piece.WriteString(cluster)
			used += cells
		}
		flush()
	}
	return lines
}

// Fit truncates the text to the width, or pads it with spaces up to the width.
func Fit(text string, width int) string {
	text = ansi.Truncate(text, width, "…")
	return text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}

func HelpLine(keysAndLabels ...string) string {
	var parts []string
	for index := 0; index < len(keysAndLabels); index += 2 {
		parts = append(parts, KeyStyle.Render(keysAndLabels[index])+" "+DimStyle.Render(keysAndLabels[index+1]))
	}
	return strings.Join(parts, DimStyle.Render(" · "))
}

func AgeText(since time.Time) string {
	minutes := int(time.Since(since).Minutes())
	switch {
	case minutes < 60:
		return fmt.Sprintf("%dm", minutes)
	case minutes < 60*24:
		return fmt.Sprintf("%dh", minutes/60)
	}
	return fmt.Sprintf("%dd", minutes/(60*24))
}

// FirstLine keeps error messages to one line, since the status line has room for one.
func FirstLine(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}
