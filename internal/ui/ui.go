// Package ui holds the terminal styling and text helpers shared by the views.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
