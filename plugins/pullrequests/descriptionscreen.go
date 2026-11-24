package pullrequests

import (
	"cmp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

type (
	descriptionLoadedMessage struct {
		pullRequest PullRequest
		body        string
		err         error
	}
	descriptionSavedMessage struct {
		pullRequest PullRequest
		body        string
		err         error
	}
)

func (board *pullRequestBoard) loadDescription(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		body, err := board.provider.Description(pr)
		return descriptionLoadedMessage{pullRequest: pr, body: body, err: err}
	}
}

// updateDescription saves the description. A failure is also put on the board as a notice,
// because the screen that would have shown it may have been closed by the time it arrives.
func (board *pullRequestBoard) updateDescription(selection plugin.Selection, body string) tea.Cmd {
	pr := pullRequestOf(selection)
	return func() tea.Msg {
		err := board.provider.SaveDescription(pr, body)
		saved := func() tea.Msg {
			return descriptionSavedMessage{pullRequest: pr, body: body, err: err}
		}
		if err == nil {
			return saved()
		}
		return tea.BatchMsg{saved, selection.Run(plugin.Effect{Notice: "Failed: " + ui.FirstLine(err)})}
	}
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
	// failedEdit is the text of an edit that could not be saved, which the next edit starts from
	// so that it is not lost. It is empty when there is none.
	failedEdit string
	lines      []string
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
	if len(screen.lines) == 0 {
		screen.lines = []string{ui.DimStyle.Render("No description.")}
	}
	screen.renderedWidth = screen.width
}

// edit opens the description in the editor, or the last edit that could not be saved.
func (screen *descriptionScreen) edit() tea.Cmd {
	text := cmp.Or(screen.failedEdit, screen.body)
	return screen.selection.Run(plugin.Effect{Edit: &plugin.Edit{Text: text, Done: screen.finishEditing}})
}

func (screen *descriptionScreen) finishEditing(edited string) tea.Cmd {
	screen.failedEdit = ""
	if edited == "" || edited == strings.TrimSpace(screen.body) {
		screen.notice = "Description unchanged."
		return nil
	}
	screen.notice = "Updating description…"
	return screen.board.updateDescription(screen.selection, edited)
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
	case descriptionSavedMessage:
		if message.pullRequest.URL != screen.pullRequest.URL {
			return screen, nil
		}
		if message.err != nil {
			screen.failedEdit = message.body
			screen.notice = "Failed: " + ui.FirstLine(message.err) + " · press e to edit again"
			return screen, nil
		}
		screen.failedEdit = ""
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
			return screen, screen.edit()
		}
	case "o":
		return screen, commandToOpenInBrowser(screen.pullRequest.URL)
	case "v":
		return screen, screen.board.commandToOpenInEditor(screen.selection)
	default:
		if command, isAction := screen.board.runScreenAction(screen.selection, key); isAction {
			return screen, command
		}
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
	keysAndLabels := []string{"↑/↓", "scroll", "space/b", "page", "e", "edit", "o", "open", "v", screen.board.editor.Name}
	keysAndLabels = append(keysAndLabels, screen.board.screenActionHelp(currentRow(screen.selection))...)
	help := ui.HelpLine(append(keysAndLabels, "esc", "back")...)
	lines = append(lines, ansi.Truncate(screen.notice, screen.width, "…"), ansi.Truncate(help, screen.width, "…"))
	return strings.Join(lines, "\n")
}
