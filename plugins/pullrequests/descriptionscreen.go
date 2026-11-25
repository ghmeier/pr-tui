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
	commentOutbox
	width  int
	height int
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
	offset        int
	// selected is the index of the conversation entry that a comment would reply to, or -1 for
	// none. headerLines holds the index in lines of each entry's header, so that selecting an entry
	// can mark it and scroll to it without laying the whole conversation out again.
	selected    int
	headerLines []int
}

func (board *pullRequestBoard) newDescriptionScreen(selection plugin.Selection) *descriptionScreen {
	return &descriptionScreen{
		commentOutbox: newCommentOutbox(board, selection), width: 120, height: 30, loading: true, selected: -1,
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
		screen.selected = min(screen.selected, len(entries)-1)
	}
	screen.layOut()
}

func (screen *descriptionScreen) layOut() {
	conversationLines, headers := screen.renderConversation()
	screen.lines = append(slices.Clone(screen.descriptionLines), conversationLines...)
	screen.headerLines = nil
	for _, header := range headers {
		screen.headerLines = append(screen.headerLines, len(screen.descriptionLines)+header)
	}
}

// renderConversation lists the comments under a heading that also says when there are none, when
// they are still loading, or when they could not be loaded. It also returns where each entry's
// header is among the lines.
func (screen *descriptionScreen) renderConversation() (lines []string, headers []int) {
	heading := ui.BoldStyle.Render("Comments")
	switch {
	case screen.conversationFailure != "":
		return []string{"", heading, ui.DimStyle.Render("Comments failed to load: " + screen.conversationFailure)}, nil
	case !screen.conversationLoaded:
		return []string{"", heading, ui.DimStyle.Render("Loading comments…")}, nil
	case len(screen.conversation) == 0:
		return []string{"", heading, ui.DimStyle.Render("No comments")}, nil
	}
	lines = []string{"", ui.BoldStyle.Render(fmt.Sprintf("Comments (%d)", len(screen.conversation)))}
	for index, entry := range screen.conversation {
		lines = append(lines, "")
		headers = append(headers, len(lines))
		lines = append(lines, screen.entryHeader(index))
		lines = append(lines, ui.RenderMarkdown(entry.Body, screen.width)...)
	}
	return lines, headers
}

// entryHeader is the entry's header, marked when the entry is selected. Every header leaves room
// for the marker so that selecting does not move the text.
func (screen *descriptionScreen) entryHeader(index int) string {
	marker := "  "
	if index == screen.selected {
		marker = ui.CursorMarker + " "
	}
	return marker + screen.conversation[index].header()
}

// selectEntry moves the selection by one entry, from none to the first and back, and scrolls to it.
func (screen *descriptionScreen) selectEntry(step int) {
	if len(screen.headerLines) == 0 {
		return
	}
	previous := screen.selected
	screen.selected = max(-1, min(previous+step, len(screen.conversation)-1))
	for _, index := range []int{previous, screen.selected} {
		if index >= 0 {
			screen.lines[screen.headerLines[index]] = screen.entryHeader(index)
		}
	}
	if screen.selected >= 0 {
		screen.scrollToEntry(screen.selected)
	}
}

// scrollToEntry shows the whole header of the entry and as much of the rest of it as fits,
// moving the view as little as possible.
func (screen *descriptionScreen) scrollToEntry(index int) {
	header, last := screen.headerLines[index], len(screen.lines)-1
	if index+1 < len(screen.headerLines) {
		last = screen.headerLines[index+1] - 2
	}
	switch {
	case header < screen.offset:
		screen.offset = header
	case last >= screen.offset+screen.rows():
		screen.offset = min(header, last-screen.rows()+1)
	}
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

const conversationCommentQuoteLines = 5

// conversationTarget is where a comment goes and what the editor opens with: a reply to the
// selected entry that quotes the start of it, or an empty comment when none is selected.
func (screen *descriptionScreen) conversationTarget() (target CommentTarget, prefill string) {
	target = CommentTarget{Conversation: true}
	if screen.selected < 0 {
		return target, ""
	}
	entry := screen.conversation[screen.selected]
	target.ReplyAuthor = entry.Author
	var quoted []string
	for _, line := range strings.Split(entry.Body, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		if len(quoted) == conversationCommentQuoteLines {
			quoted = append(quoted, "> …")
			break
		}
		quoted = append(quoted, "> "+line)
	}
	return target, "@" + entry.Author + "\n\n" + strings.Join(quoted, "\n") + "\n\n"
}

// comment writes a comment on the conversation in the editor, starting from a quote of the
// selected entry or from the text kept from the last try.
func (screen *descriptionScreen) comment() tea.Cmd {
	target, prefill := screen.conversationTarget()
	return screen.editComment(cmp.Or(screen.keptComments[target], prefill), func(text string) tea.Cmd {
		if text == "" || text == strings.TrimSpace(prefill) {
			delete(screen.keptComments, target)
			screen.notice = "Comment discarded."
			return nil
		}
		return screen.askWhere(target, text, nil)
	})
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
	case commentPostedMessage:
		if message.url != screen.pullRequest.URL || !message.target.Conversation {
			return screen, nil
		}
		if !screen.finishPost(message) {
			return screen, nil
		}
		screen.notice = "Comment posted."
		return screen, screen.board.loadConversation(screen.pullRequest)
	case plugin.CommandFinished:
		screen.finishOffer(message.Err)
	case tea.MouseMsg:
		screen.offset += ui.WheelScroll(message)
	case tea.KeyMsg:
		return screen.handleKey(message.String())
	}
	return screen, nil
}

func (screen *descriptionScreen) handleKey(key string) (plugin.Screen, tea.Cmd) {
	switch key {
	case "esc", "q":
		return nil, nil
	case "up":
		screen.offset--
	case "down":
		screen.offset++
	case "j":
		screen.selectEntry(1)
	case "k":
		screen.selectEntry(-1)
	case "c":
		return screen, screen.comment()
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
	commentLabel := "comment"
	if screen.selected >= 0 {
		commentLabel = "reply"
	}
	keysAndLabels := []string{"j/k", "comment", "↑/↓", "scroll", "space/b", "page", "c", commentLabel, "e", "edit", "o", "open", "v", screen.board.editor.Name}
	keysAndLabels = append(keysAndLabels, screen.board.screenActionHelp(screen.selection)...)
	help := ui.HelpLine(append(keysAndLabels, "esc", "back")...)
	lines = append(lines, ansi.Truncate(screen.notice, screen.width, "…"), ansi.Truncate(help, screen.width, "…"))
	return strings.Join(lines, "\n")
}
