package pullrequests

import (
	"cmp"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// editInEditor hands the terminal to the user's editor. Tests replace it.
var editInEditor = func(path string, finished func(error) tea.Msg) tea.Cmd {
	editor := strings.Fields(cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"))
	return tea.ExecProcess(exec.Command(editor[0], append(editor[1:], path)...), finished)
}

type (
	descriptionLoadedMessage struct {
		pullRequest PullRequest
		body        string
		err         error
	}
	descriptionEditedMessage struct {
		pullRequest PullRequest
		path        string
		err         error
	}
	descriptionSavedMessage struct {
		pullRequest PullRequest
		body        string
		path        string
		err         error
	}
)

func (board *pullRequestBoard) loadDescription(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		body, err := board.provider.Description(pr)
		return descriptionLoadedMessage{pullRequest: pr, body: body, err: err}
	}
}

// updateDescription removes the edit's temporary file once it is saved, here rather than when
// the message arrives, because the screen may have been closed by then. A failure is also put
// on the board as a notice, for the same reason: the screen that would have shown where the
// edit was kept may be gone.
func (board *pullRequestBoard) updateDescription(selection plugin.Selection, body, path string) tea.Cmd {
	pr := pullRequestOf(selection)
	return func() tea.Msg {
		err := board.provider.SaveDescription(pr, body)
		saved := func() tea.Msg {
			return descriptionSavedMessage{pullRequest: pr, body: body, path: path, err: err}
		}
		if err == nil {
			os.Remove(path)
			return saved()
		}
		return tea.BatchMsg{saved, selection.Run(plugin.Effect{Notice: keptEditNotice(err, path)})}
	}
}

func keptEditNotice(err error, path string) string {
	return "Failed: " + ui.FirstLine(err) + " · your edit is kept at " + path
}

// descriptionScreen shows a pull request's description as rendered markdown, and lets it be
// edited in the user's editor.
type descriptionScreen struct {
	board       *pullRequestBoard
	pullRequest PullRequest
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

func (board *pullRequestBoard) newDescriptionScreen(selection plugin.Selection) *descriptionScreen {
	return &descriptionScreen{
		board: board, pullRequest: pullRequestOf(selection), selection: selection, width: 120, height: 30, loading: true,
	}
}

func (screen *descriptionScreen) Init() tea.Cmd {
	return screen.board.loadDescription(screen.pullRequest)
}

func (screen *descriptionScreen) rows() int {
	return max(screen.height-3, 1)
}

func (screen *descriptionScreen) keepVisible() {
	screen.offset = max(0, min(screen.offset, len(screen.lines)-screen.rows()))
}

func (screen *descriptionScreen) show(body string) {
	screen.body = body
	screen.lines = ui.RenderMarkdown(body, screen.width)
	screen.renderedWidth = screen.width
}

// startEditing writes the description to a temporary file for the editor. The file is kept if
// the update fails, so the edit is not lost.
func (screen *descriptionScreen) startEditing() tea.Cmd {
	pr := screen.pullRequest
	file, err := os.CreateTemp("", "prs-description-*.md")
	if err == nil {
		_, err = file.WriteString(screen.body + "\n")
		err = cmp.Or(err, file.Close())
	}
	if err != nil {
		screen.notice = "Failed: " + ui.FirstLine(err)
		return nil
	}
	return editInEditor(file.Name(), func(err error) tea.Msg {
		return descriptionEditedMessage{pullRequest: pr, path: file.Name(), err: err}
	})
}

func (screen *descriptionScreen) finishEditing(message descriptionEditedMessage) tea.Cmd {
	if message.err != nil {
		screen.notice = "Editor failed: " + ui.FirstLine(message.err)
		return nil
	}
	edited, err := os.ReadFile(message.path)
	if err != nil {
		screen.notice = "Failed: " + ui.FirstLine(err)
		return nil
	}
	body := strings.TrimSpace(string(edited))
	if body == "" || body == strings.TrimSpace(screen.body) {
		os.Remove(message.path)
		screen.notice = "Description unchanged."
		return nil
	}
	screen.notice = "Updating description…"
	return screen.board.updateDescription(screen.selection, body, message.path)
}

// Update ignores messages about other pull requests, because screens are given every message
// that the host does not handle.
func (screen *descriptionScreen) Update(message tea.Msg) (plugin.Screen, tea.Cmd) {
	next, command := screen.update(message)
	screen.keepVisible()
	return next, command
}

func (screen *descriptionScreen) update(message tea.Msg) (plugin.Screen, tea.Cmd) {
	switch message := message.(type) {
	case descriptionLoadedMessage:
		if message.pullRequest.URL != screen.pullRequest.URL {
			return screen, nil
		}
		screen.loading = false
		if message.err != nil {
			screen.notice = "Failed: " + ui.FirstLine(message.err)
			return screen, nil
		}
		screen.show(message.body)
	case descriptionEditedMessage:
		if message.pullRequest.URL != screen.pullRequest.URL {
			return screen, nil
		}
		return screen, screen.finishEditing(message)
	case descriptionSavedMessage:
		if message.pullRequest.URL != screen.pullRequest.URL {
			return screen, nil
		}
		if message.err != nil {
			screen.notice = keptEditNotice(message.err, message.path)
			return screen, nil
		}
		screen.notice = "Description updated."
		screen.show(message.body)
	case tea.KeyMsg:
		return screen.handleKey(message.String())
	}
	return screen, nil
}

func (screen *descriptionScreen) handleKey(key string) (plugin.Screen, tea.Cmd) {
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
	case "e":
		if !screen.loading {
			return screen, screen.startEditing()
		}
	case "o":
		return screen, commandToOpenInBrowser(screen.pullRequest.URL)
	case "v":
		return screen, screen.board.commandToOpenInEditor(screen.selection)
	}
	return screen, nil
}

// View adopts the size it is drawn at, and wraps the markdown again when the width changed.
func (screen *descriptionScreen) View(width, height int) string {
	screen.width, screen.height = width, height
	if !screen.loading && screen.body != "" && screen.renderedWidth != width {
		screen.show(screen.body)
	}
	screen.keepVisible()
	pr := screen.pullRequest
	header := ansi.Truncate(ui.BoldStyle.Render(pr.label())+"  "+pr.Title, screen.width, "…")
	body := []string{ui.DimStyle.Render("Loading description…")}
	if !screen.loading {
		body = screen.lines[min(screen.offset, len(screen.lines)):]
		body = body[:min(len(body), screen.rows())]
	}
	lines := append([]string{header}, body...)
	for len(lines) < screen.rows()+1 {
		lines = append(lines, "")
	}
	help := ui.HelpLine("↑/↓", "scroll", "space/b", "page", "e", "edit", "o", "open", "v", screen.board.editor.Name, "esc", "back")
	lines = append(lines, ansi.Truncate(screen.notice, screen.width, "…"), ansi.Truncate(help, screen.width, "…"))
	return strings.Join(lines, "\n")
}
