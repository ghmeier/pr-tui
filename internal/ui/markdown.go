package ui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// RenderMarkdown renders markdown as terminal lines wrapped for the width. A blank body gives
// no lines, so each caller words its own empty state. Escape characters are removed so a document cannot move
// the cursor or restyle the screen.
func RenderMarkdown(body string, width int) []string {
	body = strings.ReplaceAll(body, "\x1b", "")
	if strings.TrimSpace(body) == "" {
		return nil
	}
	theme := "light"
	if DarkBackground {
		theme = "dark"
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle(theme), glamour.WithWordWrap(max(width-4, 20)))
	if err == nil {
		if rendered, err := renderer.Render(body); err == nil {
			return strings.Split(strings.Trim(rendered, "\n"), "\n")
		}
	}
	return strings.Split(body, "\n")
}
