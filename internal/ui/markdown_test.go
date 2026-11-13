package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderMarkdownShowsThePlaceholderForABlankBody(t *testing.T) {
	lines := RenderMarkdown("  \n", 80)

	if len(lines) != 1 || ansi.Strip(lines[0]) != "No description." {
		t.Fatalf("lines = %q", lines)
	}
}

func TestRenderMarkdownRendersTheBodyText(t *testing.T) {
	rendered := ansi.Strip(strings.Join(RenderMarkdown("# Heading\n\nSome words", 80), "\n"))

	if !strings.Contains(rendered, "Heading") || !strings.Contains(rendered, "Some words") {
		t.Fatalf("rendered = %q", rendered)
	}
}

func TestRenderMarkdownRemovesEscapeCharacters(t *testing.T) {
	rendered := strings.Join(RenderMarkdown("plain \x1b[31mred", 80), "\n")

	if strings.Contains(rendered, "\x1b[31m") {
		t.Fatalf("rendered = %q", rendered)
	}
}
