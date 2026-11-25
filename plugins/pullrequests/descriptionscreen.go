package pullrequests

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

type (
	descriptionLoadedMessage struct {
		pullRequest PullRequest
		description Description
		err         error
	}
	conversationLoadedMessage struct {
		pullRequest PullRequest
		entries     []ConversationEntry
		err         error
	}
	descriptionSavedMessage struct {
		pullRequest PullRequest
		description Description
		err         error
	}
)

func (board *pullRequestBoard) loadDescription(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		description, err := board.provider.Description(pr)
		return descriptionLoadedMessage{pullRequest: pr, description: description, err: err}
	}
}

func (board *pullRequestBoard) loadConversation(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		entries, err := board.provider.Conversation(pr)
		return conversationLoadedMessage{pullRequest: pr, entries: entries, err: err}
	}
}

// updateDescription saves the title and the body. A failure is also put on the board as a notice,
// because the screen that would have shown it may have been closed by the time it arrives.
func (board *pullRequestBoard) updateDescription(selection plugin.Selection, description Description) tea.Cmd {
	pr := pullRequestOf(selection)
	return func() tea.Msg {
		err := board.provider.SaveDescription(pr, description)
		saved := func() tea.Msg {
			return descriptionSavedMessage{pullRequest: pr, description: description, err: err}
		}
		if err == nil {
			return saved()
		}
		return tea.BatchMsg{saved, selection.Run(plugin.Effect{Notice: "Failed: " + ui.FirstLine(err)})}
	}
}

// descriptionScreen shows a pull request's description as rendered markdown, and lets the title
// and the description be edited together in the user's editor.
type descriptionScreen struct {
	board       *pullRequestBoard
	pullRequest PullRequest
	// selection is what the screen was opened from, and how it hands effects to the board.
	selection plugin.Selection
	width     int
	height    int
	// title is the title as loaded or last saved. It is empty until the description has loaded.
	title string
	body  string
	// failedEdit is the text of an edit that could not be saved, which the next edit starts from
	// so that it is not lost. It is empty when there is none.
	failedEdit string
	// descriptionLines is nil until the description has loaded. The conversation is laid out after
	// it in lines, so that the comments can arrive without moving what is being read above them.
	descriptionLines []string
	lines            []string
	// conversation is empty until it has loaded, and is kept while it loads again.
	conversation        []ConversationEntry
	conversationLoaded  bool
	conversationFailure string
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
	return tea.Batch(screen.board.loadDescription(screen.pullRequest), screen.board.loadConversation(screen.pullRequest))
}

func (screen *descriptionScreen) rows() int {
	return max(screen.height-3, 1)
}

func (screen *descriptionScreen) keepVisible() {
	screen.offset = max(0, min(screen.offset, len(screen.lines)-screen.rows()))
}

func (screen *descriptionScreen) show(body string) {
	screen.body = body
	screen.descriptionLines = ui.RenderMarkdown(body, screen.width)
	if len(screen.descriptionLines) == 0 {
		screen.descriptionLines = []string{ui.DimStyle.Render("No description.")}
	}
	screen.renderedWidth = screen.width
	screen.layOut()
}

func (screen *descriptionScreen) showConversation(entries []ConversationEntry, err error) {
	screen.conversationFailure = ""
	if err != nil {
		screen.conversationFailure = ui.FirstLine(err)
	} else {
		screen.conversation, screen.conversationLoaded = entries, true
	}
	screen.layOut()
}

func (screen *descriptionScreen) layOut() {
	screen.lines = append(slices.Clone(screen.descriptionLines), screen.renderConversation()...)
}

// renderConversation lists the comments under a heading that also says when there are none, when
// they are still loading, or when they could not be loaded.
func (screen *descriptionScreen) renderConversation() []string {
	heading := ui.BoldStyle.Render("Comments")
	switch {
	case screen.conversationFailure != "":
		return []string{"", heading, ui.DimStyle.Render("Comments failed to load: " + screen.conversationFailure)}
	case !screen.conversationLoaded:
		return []string{"", heading, ui.DimStyle.Render("Loading comments…")}
	case len(screen.conversation) == 0:
		return []string{"", heading, ui.DimStyle.Render("No comments")}
	}
	lines := []string{"", ui.BoldStyle.Render(fmt.Sprintf("Comments (%d)", len(screen.conversation)))}
	for _, entry := range screen.conversation {
		lines = append(lines, "", entry.header())
		lines = append(lines, ui.RenderMarkdown(entry.Body, screen.width)...)
	}
	return lines
}

func (entry ConversationEntry) header() string {
	if label := entry.Review.label(); label != "" {
		return authorHeading(entry.Author, entry.Created, label)
	}
	return authorHeading(entry.Author, entry.Created)
}

// label is how a review's state reads next to its author, or "" for a plain comment.
func (state ReviewState) label() string {
	switch state {
	case ReviewStateApproved:
		return "approved"
	case ReviewStateChangesRequested:
		return "requested changes"
	case ReviewStateCommented:
		return "reviewed"
	}
	return ""
}

// edit opens the title and the description in the editor laid out like a commit message, or
// the last edit that could not be saved.
func (screen *descriptionScreen) edit() tea.Cmd {
	text := cmp.Or(screen.failedEdit, formatEditableDescription(Description{Title: screen.title, Body: screen.body}))
	return screen.selection.Run(plugin.Effect{Edit: &plugin.Edit{Text: text, Done: screen.finishEditing}})
}

// formatEditableDescription lays the title and the body out as the editor shows them. It is
// the inverse of parseEditedDescription.
func formatEditableDescription(description Description) string {
	return description.Title + "\n\n" + description.Body
}

// parseEditedDescription reads the first line as the title and the rest as the body.
func parseEditedDescription(text string) Description {
	title, body, _ := strings.Cut(text, "\n")
	return Description{Title: strings.TrimSpace(title), Body: strings.TrimSpace(body)}
}

func (screen *descriptionScreen) finishEditing(edited string) tea.Cmd {
	screen.failedEdit = ""
	description := parseEditedDescription(edited)
	if description.Title == "" {
		screen.notice = "A pull request needs a title."
		return nil
	}
	if description.Title == screen.title && description.Body == strings.TrimSpace(screen.body) {
		screen.notice = "Description unchanged."
		return nil
	}
	screen.notice = "Updating description…"
	return screen.board.updateDescription(screen.selection, description)
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
		screen.title = message.description.Title
		screen.show(message.description.Body)
	case conversationLoadedMessage:
		if message.pullRequest.URL != screen.pullRequest.URL {
			return screen, nil
		}
		screen.showConversation(message.entries, message.err)
	case descriptionSavedMessage:
		if message.pullRequest.URL != screen.pullRequest.URL {
			return screen, nil
		}
		if message.err != nil {
			screen.failedEdit = formatEditableDescription(message.description)
			screen.notice = "Failed: " + ui.FirstLine(message.err) + " · press e to edit again"
			return screen, nil
		}
		screen.failedEdit = ""
		screen.notice = "Description updated."
		screen.title = message.description.Title
		screen.show(message.description.Body)
		return screen, tea.Batch(
			screen.selection.Run(plugin.Effect{RefreshRow: screen.selection.Row}), screen.board.loadConversation(screen.pullRequest),
		)
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
	if !screen.loading && screen.renderedWidth != width {
		if screen.descriptionLines != nil {
			screen.show(screen.body)
		} else {
			screen.renderedWidth = width
			screen.layOut()
		}
	}
	screen.keepVisible()
	pr := screen.pullRequest
	header := ansi.Truncate(ui.BoldStyle.Render(pr.label())+"  "+cmp.Or(screen.title, pr.Title), screen.width, "…")
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
	keysAndLabels = append(keysAndLabels, screen.board.screenActionHelp(screen.selection)...)
	help := ui.HelpLine(append(keysAndLabels, "esc", "back")...)
	lines = append(lines, ansi.Truncate(screen.notice, screen.width, "…"), ansi.Truncate(help, screen.width, "…"))
	return strings.Join(lines, "\n")
}
