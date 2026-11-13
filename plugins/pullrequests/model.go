package pullrequests

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/internal/ui"
)

const commentRows = 4

func renderMarkdown(body string, width int) []string {
	body = strings.ReplaceAll(body, "\x1b", "")
	if strings.TrimSpace(body) == "" {
		return []string{ui.DimStyle.Render("No description.")}
	}
	theme := "light"
	if ui.DarkBackground {
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

// editInEditor hands the terminal to the user's editor. Tests replace it.
var editInEditor = func(path string, finished func(error) tea.Msg) tea.Cmd {
	editor := strings.Fields(cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"))
	return tea.ExecProcess(exec.Command(editor[0], append(editor[1:], path)...), finished)
}

type mode int

const (
	boardMode mode = iota
	filterMode
	confirmMode
	diffMode
	descriptionMode
	commentMode
)

type (
	loadedMessage struct {
		pullRequests []PullRequest
		viewer       string
		err          error
	}
	lineCountsLoadedMessage struct{ counts map[string]LineCount }
	ciStatesLoadedMessage   struct{ states map[string]CIState }
	editorOpenedMessage     struct{ err error }
	commentsLoadedMessage   struct {
		url      string
		comments []ReviewComment
		err      error
	}
	commentPostedMessage struct {
		target CommentTarget
		err    error
	}
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
	commandFinishedMessage struct{ err error }
	diffLoadedMessage      struct {
		pullRequest PullRequest
		files       []diffFile
		err         error
	}
	refreshTickMessage struct{}
)

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		pullRequests, viewer, err := m.provider.PullRequests()
		return loadedMessage{pullRequests: pullRequests, viewer: viewer, err: err}
	}
}

func (m Model) detailCommands() []tea.Cmd {
	var commands []tea.Cmd
	for _, source := range m.provider.Repositories() {
		commands = append(commands, func() tea.Msg {
			states, _ := m.provider.CIStates(source)
			return ciStatesLoadedMessage{states: states}
		})
	}
	for _, source := range m.provider.Repositories() {
		commands = append(commands, func() tea.Msg {
			counts, _ := m.provider.LineCounts(source)
			return lineCountsLoadedMessage{counts: counts}
		})
	}
	return commands
}

func scheduleRefresh(refreshInterval time.Duration) tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return refreshTickMessage{} })
}

// runAction runs a change to a pull request, then the board reloads to show it.
func runAction(action func() error) tea.Cmd {
	return func() tea.Msg {
		return commandFinishedMessage{err: action()}
	}
}

func (m Model) toggleDraft(pr PullRequest) func() error {
	return func() error { return m.provider.SetDraft(pr, !pr.IsDraft) }
}

func (m Model) toggleAutoMerge(pr PullRequest) func() error {
	return func() error { return m.provider.SetAutoMerge(pr, !pr.AutoMerge) }
}

// requestTeamReview marks a draft ready first, because a review request on a draft does not
// ask anyone to look yet. If marking it ready fails, no review is requested.
func (m Model) requestTeamReview(pr PullRequest) func() error {
	return func() error {
		if pr.IsDraft {
			if err := m.provider.SetDraft(pr, false); err != nil {
				return err
			}
		}
		return m.provider.RequestTeamReview(pr)
	}
}

func (m Model) loadDiff(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		text, err := m.provider.Diff(pr)
		return diffLoadedMessage{pullRequest: pr, files: parseDiff(text), err: err}
	}
}

// cachedDiff is reused only while the pull request's head commit is unchanged.
type cachedDiff struct {
	headCommit string
	files      []diffFile
}

func (m Model) loadComments(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		comments, err := m.provider.Comments(pr)
		return commentsLoadedMessage{url: pr.URL, comments: comments, err: err}
	}
}

func (m Model) sendComment(pr PullRequest, target CommentTarget, body string) tea.Cmd {
	return func() tea.Msg {
		return commentPostedMessage{target: target, err: m.provider.PostComment(pr, target, body)}
	}
}

func (m Model) loadDescription(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		body, err := m.provider.Description(pr)
		return descriptionLoadedMessage{pullRequest: pr, body: body, err: err}
	}
}

func (m Model) updateDescription(pr PullRequest, body, path string) tea.Cmd {
	return func() tea.Msg {
		err := m.provider.SaveDescription(pr, body)
		return descriptionSavedMessage{pullRequest: pr, body: body, path: path, err: err}
	}
}

// openInEditor still opens the worktree when the changed files cannot be listed.
func (m Model) openInEditor(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		files, _ := m.provider.ChangedFiles(pr)
		_, err := shell.Run(pr.openInEditorCommand(m.settings.Editor.Command, m.settings.CloneRoot, files)...)
		return editorOpenedMessage{err: err}
	}
}

func openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		shell.Run("open", url)
		return nil
	}
}

// openInEditorCommand has worktrunk find or create the pull request's worktree in the local
// clone at <cloneRoot>/<repository>, then open that worktree and the given files in the editor.
// Worktrunk passes everything after "--" straight to the program given to -x, so the editor's
// own arguments go there too, ahead of the worktree path.
func (pr PullRequest) openInEditorCommand(editorCommand []string, cloneRoot string, files []string) []string {
	command := []string{
		"wt", "-C", filepath.Join(cloneRoot, pr.Repository.Name), "switch", fmt.Sprintf("pr:%d", pr.Number),
		"--no-cd", "-x", editorCommand[0], "--",
	}
	command = append(command, editorCommand[1:]...)
	command = append(command, ".")
	return append(command, files...)
}

func (pr PullRequest) toggleDraftNotice() string {
	if pr.IsDraft {
		return "Marking " + pr.label() + " ready for review…"
	}
	return "Converting " + pr.label() + " back to draft…"
}

func (pr PullRequest) toggleAutoMergeQuestion() string {
	if pr.AutoMerge {
		return "Disable auto-merge on " + pr.label() + "?"
	}
	return "Enable auto-merge (squash) on " + pr.label() + "? It merges as soon as its requirements are met."
}

func (pr PullRequest) requestTeamReviewNotice(reviewTeam string) string {
	if pr.IsDraft {
		return "Marking " + pr.label() + " ready and requesting review from " + reviewTeam + "…"
	}
	return "Requesting review from " + reviewTeam + " on " + pr.label() + "…"
}

type Model struct {
	settings        Settings
	provider        Provider
	filters         filters
	viewer          string
	pullRequests    []PullRequest
	lineCounts      map[string]LineCount
	ciStates        map[string]CIState
	diffCache       map[string]cachedDiff
	visible         []PullRequest
	loadedAt        time.Time
	cursor          int
	offset          int
	width           int
	height          int
	mode            mode
	notice          string
	question        string
	pendingAction   func() error
	diffPullRequest PullRequest
	diffFiles       []diffFile
	diffLoading     bool
	fileCursor      int
	fileOffset      int
	diffOffset      int
	splitDiff       bool
	diffCursor      int
	diffThreads     map[threadKey][]*commentThread
	rows            []viewRow
	diffNotice      string

	commentOnOldSide bool
	commentTarget    CommentTarget
	commentInput     textarea.Model
	commentPosting   bool
	commentError     string

	descriptionPullRequest PullRequest
	descriptionBody        string
	descriptionLines       []string
	descriptionLoading     bool
	descriptionNotice      string
	descriptionOffset      int
	descriptionReturnsTo   mode
}

func newModel(settings Settings, provider Provider, f filters) Model {
	return Model{settings: settings, provider: provider, filters: f, lineCounts: map[string]LineCount{}, ciStates: map[string]CIState{}, diffCache: map[string]cachedDiff{}, splitDiff: true, width: 120, height: 30}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), scheduleRefresh(m.settings.RefreshInterval))
}

func (m Model) reviewTeam() string {
	return m.provider.ReviewTeam()
}

func (m Model) selected() (PullRequest, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return PullRequest{}, false
	}
	return m.visible[m.cursor], true
}

// applyFilters recomputes the visible rows and keeps the cursor on the same pull request.
func (m *Model) applyFilters() {
	selected, hadSelection := m.selected()
	for index, pr := range m.pullRequests {
		if count, known := m.lineCounts[pr.URL]; known {
			m.pullRequests[index].LineCount = &count
		}
		if state, known := m.ciStates[pr.URL]; known {
			m.pullRequests[index].CIState = &state
		}
	}
	m.visible = m.filters.visible(m.pullRequests)
	if !hadSelection {
		return
	}
	for index, pr := range m.visible {
		if pr.URL == selected.URL {
			m.cursor = index
			return
		}
	}
}

func (m Model) tableRows() int {
	rows := m.height - 4
	if m.mode == filterMode {
		rows -= lipgloss.Height(m.filters.menu())
	}
	return max(rows, 1)
}

func (m *Model) keepCursorVisible() {
	m.cursor = max(0, min(m.cursor, len(m.visible)-1))
	m.offset = min(m.offset, m.cursor)
	m.offset = max(m.offset, m.cursor-m.tableRows()+1)
}

func (m Model) diffRows() int {
	if m.mode == commentMode {
		return max(m.height-3-commentRows, 1)
	}
	return max(m.height-3, 1)
}

func (m Model) fileListWidth() int {
	widest := 0
	for _, file := range m.diffFiles {
		widest = max(widest, lipgloss.Width(ui.PlainText(file.listSegments(m.width))))
	}
	if m.splitDiff {
		return min(widest, max(20, m.width/4))
	}
	return min(widest, max(20, m.width*2/5))
}

func (m Model) diffRowCount() int {
	return len(m.rows)
}

// rebuildRows lays out the selected file for the current layout and width, with each
// comment thread directly under the line it is attached to.
func (m *Model) rebuildRows() {
	m.rows = nil
	if len(m.diffFiles) == 0 {
		return
	}
	file := m.diffFiles[m.fileCursor]
	commentWidth := max(m.width-m.fileListWidth()-7, 20)
	appendThreads := func(lineIndex int) {
		for _, key := range file.lines[lineIndex].threadKeys(file.path) {
			for _, thread := range m.diffThreads[key] {
				for _, text := range thread.render(commentWidth) {
					m.rows = append(m.rows, viewRow{left: lineIndex, right: lineIndex, thread: thread, comment: text})
				}
			}
		}
	}
	if !m.splitDiff {
		for index := range file.lines {
			m.rows = append(m.rows, viewRow{left: index, right: index})
			appendThreads(index)
		}
		return
	}
	for _, row := range file.splitRows {
		m.rows = append(m.rows, viewRow{left: row.left, right: row.right})
		if row.left >= 0 {
			appendThreads(row.left)
		}
		if row.right >= 0 && row.right != row.left {
			appendThreads(row.right)
		}
	}
}

// cursorLine is the index of the diff line under the cursor, or -1. A side-by-side row
// with a line on each side resolves to the new line unless the old side is chosen.
func (m Model) cursorLine() int {
	if m.diffCursor < 0 || m.diffCursor >= len(m.rows) {
		return -1
	}
	row := m.rows[m.diffCursor]
	if row.right < 0 || (m.commentOnOldSide && row.left >= 0) {
		return row.left
	}
	return row.right
}

func (m Model) rowForLine(line int) int {
	for index, row := range m.rows {
		if row.thread == nil && (row.left == line || row.right == line) {
			return index
		}
	}
	return 0
}

func (m Model) cursorCommentTarget() (CommentTarget, bool) {
	index := m.cursorLine()
	if index < 0 {
		return CommentTarget{}, false
	}
	if thread := m.rows[m.diffCursor].thread; thread != nil {
		return thread.replyTarget(), true
	}
	file := m.diffFiles[m.fileCursor]
	switch line := file.lines[index]; line.kind {
	case removedLine:
		return CommentTarget{Path: file.path, Line: line.oldNumber, Side: CommentSideOld}, true
	case addedLine, contextLine:
		return CommentTarget{Path: file.path, Line: line.newNumber, Side: CommentSideNew}, true
	}
	return CommentTarget{}, false
}

func (m *Model) startComment() tea.Cmd {
	target, isCodeLine := m.cursorCommentTarget()
	if !isCodeLine {
		m.diffNotice = "Move to a code line to comment on it."
		return nil
	}
	m.mode = commentMode
	m.commentTarget, m.commentPosting, m.commentError = target, false, ""
	m.commentInput = textarea.New()
	m.commentInput.Placeholder = "Write a comment…"
	m.commentInput.ShowLineNumbers = false
	m.commentInput.CharLimit = 0
	m.commentInput.SetWidth(m.width)
	m.commentInput.SetHeight(commentRows)
	m.commentInput.Cursor.SetMode(cursor.CursorStatic)
	return m.commentInput.Focus()
}

func (m Model) handleCommentKey(message tea.KeyMsg) (Model, tea.Cmd) {
	switch message.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if !m.commentPosting {
			m.mode = diffMode
		}
		return m, nil
	case "ctrl+s":
		body := strings.TrimSpace(m.commentInput.Value())
		if m.commentPosting || body == "" {
			return m, nil
		}
		m.commentPosting, m.commentError = true, ""
		return m, m.sendComment(m.diffPullRequest, m.commentTarget, body)
	}
	if m.commentPosting {
		return m, nil
	}
	var command tea.Cmd
	m.commentInput, command = m.commentInput.Update(message)
	return m, command
}

func (m Model) cursorIsOnNewSide(index int) bool {
	row := m.rows[index]
	return m.splitDiff && row.thread == nil && row.left != row.right && m.cursorLine() == row.right
}

func (m Model) renderDiffRow(index, width int) string {
	file := m.diffFiles[m.fileCursor]
	row := m.rows[index]
	switch {
	case row.thread != nil:
		return ansi.Truncate("  "+row.comment, width, "…")
	case !m.splitDiff:
		return file.lines[row.left].render(width)
	}
	separator := ui.DimStyle.Render(" │ ")
	if index == m.diffCursor && m.cursorIsOnNewSide(index) {
		separator = " " + ui.CursorMarker + " "
	}
	return file.renderSplitRow(splitRow{row.left, row.right}, width, separator)
}

// diffSeparator sits between the file list and the diff. On the cursor row it carries the
// cursor, unless the cursor is on the new side of a side-by-side row.
func (m Model) diffSeparator(index int) string {
	if index != m.diffCursor || index >= len(m.rows) || m.cursorIsOnNewSide(index) {
		return ui.DimStyle.Render(" │ ")
	}
	return " " + ui.CursorMarker + " "
}

func (m *Model) keepDiffPositionVisible() {
	m.fileCursor = max(0, min(m.fileCursor, len(m.diffFiles)-1))
	m.fileOffset = min(m.fileOffset, m.fileCursor)
	m.fileOffset = max(m.fileOffset, m.fileCursor-m.diffRows()+1)
	m.rebuildRows()
	m.diffCursor = max(0, min(m.diffCursor, m.diffRowCount()-1))
	m.diffOffset = min(m.diffOffset, m.diffCursor)
	m.diffOffset = max(m.diffOffset, m.diffCursor-m.diffRows()+1, 0)
}

func (m Model) descriptionRows() int {
	return max(m.height-3, 1)
}

func (m *Model) keepDescriptionVisible() {
	m.descriptionOffset = max(0, min(m.descriptionOffset, len(m.descriptionLines)-m.descriptionRows()))
}

func (m *Model) openDescription(pr PullRequest) tea.Cmd {
	m.descriptionReturnsTo = m.mode
	m.mode = descriptionMode
	m.descriptionPullRequest = pr
	m.descriptionBody, m.descriptionLines = "", nil
	m.descriptionLoading, m.descriptionNotice, m.descriptionOffset = true, "", 0
	return m.loadDescription(pr)
}

func (m *Model) showDescription(body string) {
	m.descriptionBody = body
	m.descriptionLines = renderMarkdown(body, m.width)
}

// startEditingDescription writes the description to a temporary file for the editor. The
// file is kept if the update fails, so the edit is not lost.
func (m *Model) startEditingDescription() tea.Cmd {
	pr := m.descriptionPullRequest
	file, err := os.CreateTemp("", "prs-description-*.md")
	if err == nil {
		_, err = file.WriteString(m.descriptionBody + "\n")
		err = cmp.Or(err, file.Close())
	}
	if err != nil {
		m.descriptionNotice = "Failed: " + ui.FirstLine(err)
		return nil
	}
	return editInEditor(file.Name(), func(err error) tea.Msg {
		return descriptionEditedMessage{pullRequest: pr, path: file.Name(), err: err}
	})
}

func (m *Model) finishEditingDescription(message descriptionEditedMessage) tea.Cmd {
	if message.err != nil {
		m.descriptionNotice = "Editor failed: " + ui.FirstLine(message.err)
		return nil
	}
	edited, err := os.ReadFile(message.path)
	if err != nil {
		m.descriptionNotice = "Failed: " + ui.FirstLine(err)
		return nil
	}
	body := strings.TrimSpace(string(edited))
	if body == "" || body == strings.TrimSpace(m.descriptionBody) {
		os.Remove(message.path)
		m.descriptionNotice = "Description unchanged."
		return nil
	}
	m.descriptionNotice = "Updating description…"
	return m.updateDescription(message.pullRequest, body, message.path)
}

func (m *Model) closeDiff() {
	m.mode = boardMode
	m.diffFiles, m.diffLoading, m.diffThreads = nil, false, nil
	m.fileCursor, m.fileOffset, m.diffOffset, m.diffCursor, m.diffNotice = 0, 0, 0, 0, ""
}

func (m *Model) ask(question string, action func() error) {
	m.mode = confirmMode
	m.question = question
	m.pendingAction = action
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	next, command := m.update(message)
	next.keepCursorVisible()
	next.keepDiffPositionVisible()
	next.keepDescriptionVisible()
	return next, command
}

func (m Model) update(message tea.Msg) (Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
		if m.mode == commentMode {
			m.commentInput.SetWidth(m.width)
		}
		if m.mode == descriptionMode && !m.descriptionLoading {
			m.showDescription(m.descriptionBody)
		}
	case descriptionLoadedMessage:
		if m.mode != descriptionMode || message.pullRequest.URL != m.descriptionPullRequest.URL {
			return m, nil
		}
		m.descriptionLoading = false
		if message.err != nil {
			m.descriptionNotice = "Failed: " + ui.FirstLine(message.err)
			return m, nil
		}
		m.showDescription(message.body)
	case descriptionEditedMessage:
		return m, m.finishEditingDescription(message)
	case descriptionSavedMessage:
		if message.err != nil {
			m.descriptionNotice = "Failed: " + ui.FirstLine(message.err) + " · your edit is kept at " + message.path
			return m, nil
		}
		os.Remove(message.path)
		m.descriptionNotice = "Description updated."
		if message.pullRequest.URL == m.descriptionPullRequest.URL {
			m.showDescription(message.body)
		}
	case loadedMessage:
		if message.err != nil {
			m.notice = "Refresh failed: " + ui.FirstLine(message.err)
			return m, nil
		}
		m.notice = ""
		m.viewer = message.viewer
		m.pullRequests = message.pullRequests
		m.loadedAt = time.Now()
		m.applyFilters()
		return m, tea.Batch(m.detailCommands()...)
	case ciStatesLoadedMessage:
		for url, state := range message.states {
			m.ciStates[url] = state
		}
		m.applyFilters()
	case lineCountsLoadedMessage:
		for url, count := range message.counts {
			m.lineCounts[url] = count
		}
		m.applyFilters()
	case commandFinishedMessage:
		if message.err != nil {
			m.notice = "Failed: " + ui.FirstLine(message.err)
			return m, nil
		}
		return m, m.load()
	case diffLoadedMessage:
		if message.err == nil {
			m.diffCache[message.pullRequest.URL] = cachedDiff{message.pullRequest.HeadCommit, message.files}
		}
		if m.mode != diffMode || message.pullRequest.URL != m.diffPullRequest.URL {
			return m, nil
		}
		if message.err != nil {
			m.closeDiff()
			m.notice = "Diff failed: " + ui.FirstLine(message.err)
			return m, nil
		}
		m.diffFiles, m.diffLoading = message.files, false
	case editorOpenedMessage:
		m.notice = ""
		if message.err != nil {
			m.notice = "Failed: " + ui.FirstLine(message.err)
		}
	case refreshTickMessage:
		return m, tea.Batch(m.load(), scheduleRefresh(m.settings.RefreshInterval))
	case commentPostedMessage:
		m.commentPosting = false
		if message.err != nil {
			m.commentError = ui.FirstLine(message.err)
			return m, nil
		}
		if m.mode == commentMode {
			m.mode = diffMode
		}
		m.diffNotice = "Comment posted on " + message.target.label() + "."
		return m, m.loadComments(m.diffPullRequest)
	case commentsLoadedMessage:
		if message.url != m.diffPullRequest.URL || (m.mode != diffMode && m.mode != commentMode) {
			return m, nil
		}
		if message.err != nil {
			m.diffNotice = "Comments failed to load: " + ui.FirstLine(message.err)
			return m, nil
		}
		m.diffThreads = groupThreads(message.comments)
	case tea.KeyMsg:
		if m.mode == commentMode {
			return m.handleCommentKey(message)
		}
		return m.handleKey(message.String())
	default:
		if m.mode == commentMode {
			var command tea.Cmd
			m.commentInput, command = m.commentInput.Update(message)
			return m, command
		}
	}
	return m, nil
}

func (m Model) handleKey(key string) (Model, tea.Cmd) {
	if key == "ctrl+c" || (key == "q" && m.mode != diffMode && m.mode != descriptionMode) {
		return m, tea.Quit
	}
	switch m.mode {
	case descriptionMode:
		return m.handleDescriptionKey(key)
	case diffMode:
		return m.handleDiffKey(key)
	case filterMode:
		return m.handleFilterKey(key), nil
	case confirmMode:
		return m.handleConfirmKey(key)
	}

	pr, hasSelection := m.selected()
	switch key {
	case "up", "k":
		m.cursor--
	case "down", "j":
		m.cursor++
	case "pgup":
		m.cursor -= m.tableRows()
	case "pgdown":
		m.cursor += m.tableRows()
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = len(m.visible) - 1
	case "f":
		m.mode = filterMode
	case "r":
		m.notice = "Refreshing…"
		return m, m.load()
	case "o":
		if hasSelection {
			return m, openInBrowser(pr.URL)
		}
	case "enter":
		if hasSelection {
			m.mode = diffMode
			m.diffPullRequest = pr
			if cached, isCached := m.diffCache[pr.URL]; isCached && cached.headCommit == pr.HeadCommit {
				m.diffFiles = cached.files
				return m, m.loadComments(pr)
			}
			m.diffLoading = true
			return m, tea.Batch(m.loadDiff(pr), m.loadComments(pr))
		}
	case "p":
		if hasSelection {
			return m, m.openDescription(pr)
		}
	case "v":
		if hasSelection {
			m.notice = "Opening " + pr.label() + " in " + m.settings.Editor.Name + "…"
			return m, m.openInEditor(pr)
		}
	case "d":
		if hasSelection {
			m.notice = pr.toggleDraftNotice()
			return m, runAction(m.toggleDraft(pr))
		}
	case "m":
		if hasSelection {
			m.ask(pr.toggleAutoMergeQuestion(), m.toggleAutoMerge(pr))
		}
	case "e":
		if reviewTeam := m.reviewTeam(); hasSelection && reviewTeam != "" {
			m.notice = pr.requestTeamReviewNotice(reviewTeam)
			return m, runAction(m.requestTeamReview(pr))
		}
	}
	return m, nil
}

func (m Model) handleDiffKey(key string) (Model, tea.Cmd) {
	m.diffNotice = ""
	switch key {
	case "esc", "q":
		m.closeDiff()
	case "up", "left":
		m.fileCursor--
		m.diffCursor, m.diffOffset = 0, 0
	case "down", "right":
		m.fileCursor++
		m.diffCursor, m.diffOffset = 0, 0
	case "s":
		if len(m.diffFiles) > 0 {
			line := m.cursorLine()
			m.splitDiff = !m.splitDiff
			m.rebuildRows()
			m.diffCursor = m.rowForLine(line)
		}
	case "tab":
		m.commentOnOldSide = !m.commentOnOldSide
	case "c":
		return m, m.startComment()
	case "k":
		m.diffCursor--
	case "j":
		m.diffCursor++
	case "pgup", "b":
		m.diffCursor -= m.diffRows()
	case "pgdown", " ":
		m.diffCursor += m.diffRows()
	case "home":
		m.diffCursor = 0
	case "end":
		m.diffCursor = m.diffRowCount()
	case "o":
		return m, openInBrowser(m.diffPullRequest.URL)
	case "v":
		return m, m.openInEditor(m.diffPullRequest)
	case "p":
		return m, m.openDescription(m.diffPullRequest)
	}
	return m, nil
}

func (m Model) handleDescriptionKey(key string) (Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.mode = m.descriptionReturnsTo
		m.descriptionLines, m.descriptionNotice = nil, ""
	case "up", "k":
		m.descriptionOffset--
	case "down", "j":
		m.descriptionOffset++
	case "pgup", "b":
		m.descriptionOffset -= m.descriptionRows()
	case "pgdown", " ":
		m.descriptionOffset += m.descriptionRows()
	case "home":
		m.descriptionOffset = 0
	case "end":
		m.descriptionOffset = len(m.descriptionLines)
	case "e":
		if !m.descriptionLoading {
			return m, m.startEditingDescription()
		}
	case "o":
		return m, openInBrowser(m.descriptionPullRequest.URL)
	case "v":
		return m, m.openInEditor(m.descriptionPullRequest)
	}
	return m, nil
}

func (m *Model) toggleRepository(index int) {
	name := m.filters.available[index].FullName()
	m.filters.repositories[name] = !m.filters.repositories[name]
}

func (m Model) handleFilterKey(key string) Model {
	switch key {
	case "f", "esc", "enter":
		m.mode = boardMode
		return m
	case "b":
		m.filters.showBots = !m.filters.showBots
	case "up", "k":
		m.filters.cursor = max(m.filters.cursor-1, 0)
	case "down", "j":
		m.filters.cursor = min(m.filters.cursor+1, len(m.filters.available)-1)
	case " ":
		m.toggleRepository(m.filters.cursor)
	default:
		number, err := strconv.Atoi(key)
		if err == nil && number >= 1 && number <= min(numberKeyCount, len(m.filters.available)) {
			m.toggleRepository(number - 1)
		}
	}
	m.applyFilters()
	return m
}

func (m Model) handleConfirmKey(key string) (Model, tea.Cmd) {
	switch key {
	case "y", "enter":
		action := m.pendingAction
		m.mode, m.question, m.pendingAction = boardMode, "", nil
		m.notice = "Working…"
		return m, runAction(action)
	case "n", "esc":
		m.mode, m.question, m.pendingAction = boardMode, "", nil
	}
	return m, nil
}

func (m Model) statusLine() string {
	switch {
	case m.mode == confirmMode:
		return ui.KeyStyle.Render(m.question) + "  " + ui.HelpLine("y", "yes", "n", "no")
	case m.notice != "":
		return m.notice
	case m.loadedAt.IsZero():
		return ui.DimStyle.Render("Loading…")
	}
	return ui.DimStyle.Render(fmt.Sprintf(
		"%d of %d open pull requests · updated %s · refreshes every %s",
		len(m.visible), len(m.pullRequests), m.loadedAt.Format("15:04"), intervalText(m.settings.RefreshInterval),
	))
}

// intervalText says a refresh interval in whole minutes where it can, as in "3 minutes". A
// single minute or second has no number, so the status line reads "every second".
func intervalText(interval time.Duration) string {
	switch {
	case interval == time.Minute:
		return "minute"
	case interval == time.Second:
		return "second"
	case interval%time.Minute == 0:
		return fmt.Sprintf("%d minutes", interval/time.Minute)
	}
	return fmt.Sprintf("%d seconds", int(interval.Round(time.Second)/time.Second))
}

func (m Model) help() string {
	switch m.mode {
	case filterMode:
		return ui.HelpLine("esc", "close filters", "q", "quit")
	case confirmMode:
		return ""
	}
	keysAndLabels := []string{"enter", "diff", "p", "description", "o", "open", "v", m.settings.Editor.Name, "d", "draft / ready"}
	if reviewTeam := m.reviewTeam(); reviewTeam != "" {
		keysAndLabels = append(keysAndLabels, "e", "request "+reviewTeam)
	}
	return ui.HelpLine(append(keysAndLabels, "m", "auto-merge", "f", "filters", "r", "refresh", "q", "quit")...)
}

func (m Model) diffBody() []string {
	if m.diffLoading {
		return []string{ui.DimStyle.Render("Loading diff…")}
	}
	if len(m.diffFiles) == 0 {
		return []string{ui.DimStyle.Render("No changes.")}
	}
	listWidth := m.fileListWidth()
	diffWidth := max(m.width-listWidth-3, 1)
	rowCount := m.diffRowCount()
	var body []string
	for row := range m.diffRows() {
		left := strings.Repeat(" ", listWidth)
		if index := m.fileOffset + row; index < len(m.diffFiles) {
			segments := m.diffFiles[index].listSegments(listWidth)
			if index == m.fileCursor {
				left = ui.SelectedStyle.Render(ui.Fit(ui.PlainText(segments), listWidth))
			} else {
				left = ui.Fit(ui.StyledText(segments), listWidth)
			}
		}
		right := ""
		if index := m.diffOffset + row; index < rowCount {
			right = m.renderDiffRow(index, diffWidth)
		}
		body = append(body, left+m.diffSeparator(m.diffOffset+row)+right)
	}
	return body
}

func (m Model) commentPanel() []string {
	title := ui.BoldStyle.Render("Comment on " + m.commentTarget.label())
	if m.commentTarget.ReplyTo != 0 {
		title = ui.BoldStyle.Render("Reply to " + m.commentTarget.ReplyAuthor + " on " + m.commentTarget.label())
	}
	switch {
	case m.commentPosting:
		title += ui.DimStyle.Render("  Posting…")
	case m.commentError != "":
		title += "  " + ui.RedStyle.Render("Failed: "+m.commentError)
	}
	return []string{
		ansi.Truncate(title, m.width, "…"),
		m.commentInput.View(),
		ui.HelpLine("ctrl+s", "post", "esc", "cancel"),
	}
}

func (m Model) diffHelp() string {
	position := ""
	if len(m.diffFiles) > 0 {
		position = ui.DimStyle.Render(fmt.Sprintf("file %d of %d · ", m.fileCursor+1, len(m.diffFiles)))
	}
	layout := "split"
	if m.splitDiff {
		layout = "unified"
	}
	return position + ui.HelpLine(
		"↑/↓", "file", "j/k", "line", "space/b", "page", "c", "comment", "tab", "old/new side", "s", layout,
		"p", "description", "o", "open", "v", m.settings.Editor.Name, "esc", "back",
	)
}

func (m Model) diffView() string {
	pr := m.diffPullRequest
	header := ui.BoldStyle.Render(pr.label()) + "  " + pr.Title + "  " + ui.StyledText(pr.lineCountSegments())
	lines := append([]string{ansi.Truncate(header, m.width, "…")}, m.diffBody()...)
	for len(lines) < m.diffRows()+1 {
		lines = append(lines, "")
	}
	if m.mode == commentMode {
		lines = append(lines, m.commentPanel()...)
	} else {
		lines = append(lines, ansi.Truncate(m.diffNotice, m.width, "…"), ansi.Truncate(m.diffHelp(), m.width, "…"))
	}
	return strings.Join(lines, "\n")
}

func (m Model) descriptionView() string {
	pr := m.descriptionPullRequest
	header := ansi.Truncate(ui.BoldStyle.Render(pr.label())+"  "+pr.Title, m.width, "…")
	body := []string{ui.DimStyle.Render("Loading description…")}
	if !m.descriptionLoading {
		body = m.descriptionLines[min(m.descriptionOffset, len(m.descriptionLines)):]
		body = body[:min(len(body), m.descriptionRows())]
	}
	lines := append([]string{header}, body...)
	for len(lines) < m.descriptionRows()+1 {
		lines = append(lines, "")
	}
	help := ui.HelpLine("↑/↓", "scroll", "space/b", "page", "e", "edit", "o", "open", "v", m.settings.Editor.Name, "esc", "back")
	lines = append(lines, ansi.Truncate(m.descriptionNotice, m.width, "…"), ansi.Truncate(help, m.width, "…"))
	return strings.Join(lines, "\n")
}

func (m Model) View() string {
	if m.mode == diffMode || m.mode == commentMode {
		return m.diffView()
	}
	if m.mode == descriptionMode {
		return m.descriptionView()
	}
	sections := []string{m.filters.summary()}
	if m.mode == filterMode {
		sections = append(sections, m.filters.menu())
	}
	rows := m.tableRows()
	table := renderTable(m.visible, m.viewer, m.reviewTeam(), m.width, m.cursor, m.offset, rows)
	blankLines := rows + 1 - lipgloss.Height(table)
	sections = append(sections, table+strings.Repeat("\n", max(blankLines, 0)))
	sections = append(sections, ansi.Truncate(m.statusLine(), m.width, "…"), ansi.Truncate(m.help(), m.width, "…"))
	return strings.Join(sections, "\n")
}
