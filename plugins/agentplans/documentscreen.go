package agentplans

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

type documentLoadedMessage struct {
	key  string
	body string
	err  error
}

// documentScreen shows a document's text as rendered markdown.
type documentScreen struct {
	settings settings
	row      Row
	// selection is what the screen was opened from, and how it hands effects to the board.
	selection plugin.Selection
	width     int
	height    int
	body      string
	lines     []string
	// renderedWidth is the width the lines were wrapped for.
	renderedWidth int
	loading       bool
	notice        string
	offset        int
}

func newDocumentScreen(settings settings, selection plugin.Selection) *documentScreen {
	return &documentScreen{
		settings: settings, row: selection.Row.(Row), selection: selection, width: 120, height: 30, loading: true,
	}
}

func (screen *documentScreen) Init() tea.Cmd {
	row := screen.row
	return func() tea.Msg {
		body, err := os.ReadFile(row.Path)
		return documentLoadedMessage{key: row.Key(), body: string(body), err: err}
	}
}

func (screen *documentScreen) rows() int {
	return max(screen.height-3, 1)
}

func (screen *documentScreen) keepVisible() {
	screen.offset = max(0, min(screen.offset, len(screen.lines)-screen.rows()))
}

func (screen *documentScreen) show(body string) {
	screen.body = body
	screen.lines = ui.RenderMarkdown(body, screen.width)
	screen.renderedWidth = screen.width
}

// Update ignores messages about other documents, because screens are given every message that
// the host does not handle.
func (screen *documentScreen) Update(message tea.Msg) (plugin.Screen, tea.Cmd) {
	next, command := screen.update(message)
	screen.keepVisible()
	return next, command
}

func (screen *documentScreen) update(message tea.Msg) (plugin.Screen, tea.Cmd) {
	switch message := message.(type) {
	case documentLoadedMessage:
		if message.key != screen.row.Key() {
			return screen, nil
		}
		screen.loading = false
		if message.err != nil {
			screen.notice = "Failed: " + ui.FirstLine(message.err)
			return screen, nil
		}
		screen.show(withoutFrontMatter(message.body))
	case tea.KeyMsg:
		return screen.handleKey(message.String())
	}
	return screen, nil
}

func (screen *documentScreen) handleKey(key string) (plugin.Screen, tea.Cmd) {
	switch key {
	case "esc", "q":
		return nil, nil
	case "up", "k":
		screen.offset--
	case "down", "j":
		screen.offset++
	case "pgup", "b":
		screen.offset -= screen.rows()
	case "pgdown", " ":
		screen.offset += screen.rows()
	case "home":
		screen.offset = 0
	case "end":
		screen.offset = len(screen.lines)
	case "v":
		return screen, screen.selection.Run(screen.settings.openInEditor(screen.row))
	}
	return screen, nil
}

// View adopts the size it is drawn at, and wraps the markdown again when the width changed.
func (screen *documentScreen) View(width, height int) string {
	screen.width, screen.height = width, height
	if !screen.loading && screen.body != "" && screen.renderedWidth != width {
		screen.show(screen.body)
	}
	screen.keepVisible()
	header := ansi.Truncate(ui.BoldStyle.Render(screen.row.Repository+" "+screen.row.Doc.File)+"  "+screen.row.Doc.Title, screen.width, "…")
	body := []string{ui.DimStyle.Render("Loading…")}
	if !screen.loading {
		body = screen.lines[min(screen.offset, len(screen.lines)):]
		body = body[:min(len(body), screen.rows())]
	}
	lines := append([]string{header}, body...)
	for len(lines) < screen.rows()+1 {
		lines = append(lines, "")
	}
	help := ui.HelpLine("↑/↓", "scroll", "space/b", "page", "v", screen.settings.editor.Name, "esc", "back")
	lines = append(lines, ansi.Truncate(screen.notice, screen.width, "…"), ansi.Truncate(help, screen.width, "…"))
	return strings.Join(lines, "\n")
}
