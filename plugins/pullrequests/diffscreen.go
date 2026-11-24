package pullrequests

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

const commentRows = 4

type (
	diffLoadedMessage struct {
		pullRequest PullRequest
		files       []diffFile
		err         error
	}
	commentsLoadedMessage struct {
		url      string
		comments []ReviewComment
		err      error
	}
	commentPostedMessage struct {
		url    string
		target CommentTarget
		err    error
	}
)

// diffCache holds the last diff loaded for each pull request, to show while a newer one loads.
// It outlives the diff screens, and is written by the command that loads a diff, which may
// finish after its screen was closed. The mutex is what makes that safe.
type diffCache struct {
	mutex   sync.Mutex
	entries map[string][]diffFile
}

func newDiffCache() *diffCache {
	return &diffCache{entries: map[string][]diffFile{}}
}

func (cache *diffCache) lookup(pr PullRequest) ([]diffFile, bool) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	files, found := cache.entries[pr.URL]
	return files, found
}

func (cache *diffCache) store(pr PullRequest, files []diffFile) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.entries[pr.URL] = files
}

func (board *pullRequestBoard) loadDiff(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		text, err := board.provider.Diff(pr)
		files := parseDiff(text)
		if err == nil {
			board.diffs.store(pr, files)
		}
		return diffLoadedMessage{pullRequest: pr, files: files, err: err}
	}
}

func (board *pullRequestBoard) loadComments(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		comments, err := board.provider.Comments(pr)
		return commentsLoadedMessage{url: pr.URL, comments: comments, err: err}
	}
}

func (board *pullRequestBoard) sendComment(pr PullRequest, target CommentTarget, body string) tea.Cmd {
	return func() tea.Msg {
		return commentPostedMessage{url: pr.URL, target: target, err: board.provider.PostComment(pr, target, body)}
	}
}

// diffScreen shows a pull request's diff, file by file, with its review comments and a box to
// write a new one. It learns its size from View, which is drawn after every update, so a key
// that depends on the size uses the size of the last drawing.
type diffScreen struct {
	board       *pullRequestBoard
	pullRequest PullRequest
	// selection is what the screen was opened from, and how it hands effects to the board.
	selection plugin.Selection
	// lineCount is the loaded value of the Lines column, or nil.
	lineCount any
	width     int
	height    int
	files     []diffFile
	loading   bool
	// refreshing is true while a newer diff loads behind the cached one on screen.
	refreshing bool
	fileCursor int
	fileOffset int
	diffOffset int
	// splitDiff is the layout preference for files with two sides.
	splitDiff bool
	// splitOneSided holds the paths of one-sided files the person chose to see split.
	splitOneSided map[string]bool
	diffCursor    int
	threads       map[threadKey][]*commentThread
	notice        string

	// rows is the selected file laid out for the current layout and width. It is rebuilt only
	// when rowsKey changes, because wrapping every line of a large file on each key would be slow.
	rows      []viewRow
	rowsKey   rowsKey
	rowsBuilt bool
	// dataVersion changes whenever the files or the comment threads are replaced.
	dataVersion int

	commenting       bool
	commentOnOldSide bool
	commentTarget    CommentTarget
	commentInput     textarea.Model
	commentPosting   bool
	commentError     string
}

func (board *pullRequestBoard) newDiffScreen(selection plugin.Selection) *diffScreen {
	pr := pullRequestOf(selection)
	screen := &diffScreen{
		board: board, pullRequest: pr, selection: selection, lineCount: selection.Data(linesHeader),
		splitDiff: true, width: 120, height: 30,
	}
	if files, cached := board.diffs.lookup(pr); cached {
		screen.files, screen.refreshing = files, true
	} else {
		screen.loading = true
	}
	return screen
}

func (screen *diffScreen) Init() tea.Cmd {
	return tea.Batch(screen.board.loadDiff(screen.pullRequest), screen.board.loadComments(screen.pullRequest))
}

// selectedFile is the file being shown, if there is one.
func (screen *diffScreen) selectedFile() (diffFile, bool) {
	if screen.fileCursor < 0 || screen.fileCursor >= len(screen.files) {
		return diffFile{}, false
	}
	return screen.files[screen.fileCursor], true
}

// showsSplit is whether the file being shown is laid out side by side. A one-sided file
// is unified unless the person asked otherwise for that file.
func (screen *diffScreen) showsSplit() bool {
	file, found := screen.selectedFile()
	if found && file.oneSided {
		return screen.splitOneSided[file.path]
	}
	return screen.splitDiff
}

func (screen *diffScreen) toggleLayout() {
	file, found := screen.selectedFile()
	switch {
	case !found:
	case !file.oneSided:
		screen.splitDiff = !screen.splitDiff
	case screen.splitOneSided[file.path]:
		delete(screen.splitOneSided, file.path)
	default:
		if screen.splitOneSided == nil {
			screen.splitOneSided = map[string]bool{}
		}
		screen.splitOneSided[file.path] = true
	}
}

// replaceFiles shows a newly loaded diff. It stays on the same file, and on the same line
// of it where that line still exists.
func (screen *diffScreen) replaceFiles(files []diffFile) {
	var keptPath string
	var keptLine *diffLine
	if file, found := screen.selectedFile(); found {
		keptPath = file.path
		if index := screen.cursorLine(); index >= 0 && file.lines[index].kind >= contextLine {
			keptLine = &file.lines[index]
		}
	}
	screen.files = files
	screen.dataVersion++
	fileIndex := slices.IndexFunc(files, func(file diffFile) bool { return file.path == keptPath })
	if fileIndex < 0 {
		screen.fileCursor, screen.diffCursor, screen.diffOffset = 0, 0, 0
		return
	}
	screen.fileCursor = fileIndex
	if keptLine == nil {
		return
	}
	screen.rebuildRows()
	lineIndex := slices.IndexFunc(files[fileIndex].lines, func(line diffLine) bool {
		return line.kind == keptLine.kind && line.oldNumber == keptLine.oldNumber && line.newNumber == keptLine.newNumber
	})
	if lineIndex >= 0 {
		screen.diffCursor = screen.rowForLine(lineIndex)
	}
}

func (screen *diffScreen) diffRows() int {
	if screen.commenting {
		return max(screen.height-3-commentRows, 1)
	}
	return max(screen.height-3, 1)
}

func (screen *diffScreen) fileListWidth() int {
	widest := 0
	for _, file := range screen.files {
		widest = max(widest, lipgloss.Width(ui.PlainText(file.listSegments(screen.width))))
	}
	if screen.showsSplit() {
		return min(widest, max(20, screen.width/4))
	}
	return min(widest, max(20, screen.width*2/5))
}

// rowsKey is everything the rows depend on that can change while the screen is open.
type rowsKey struct {
	fileCursor  int
	splitDiff   bool
	width       int
	dataVersion int
}

func (screen *diffScreen) diffWidth() int {
	return max(screen.width-screen.fileListWidth()-3, 1)
}

func (screen *diffScreen) diffRowCount() int {
	return len(screen.rows)
}

// rebuildRows lays out the selected file for the current layout and width, with each
// comment thread directly under the line it is attached to. A line too long for the width
// takes a row for each of its wrapped parts.
func (screen *diffScreen) rebuildRows() {
	key := rowsKey{screen.fileCursor, screen.showsSplit(), screen.width, screen.dataVersion}
	if screen.rowsBuilt && key == screen.rowsKey {
		return
	}
	// A resize moves the rows around, so the cursor follows its line. Any other change starts
	// from wherever the caller put the cursor.
	keptLine := -1
	if screen.rowsBuilt && key.width != screen.rowsKey.width {
		previous := screen.rowsKey
		previous.width = key.width
		if previous == key {
			keptLine = screen.cursorLine()
		}
	}
	screen.rowsKey, screen.rowsBuilt = key, true
	screen.rows = nil
	defer func() {
		if keptLine >= 0 {
			screen.diffCursor = screen.rowForLine(keptLine)
		}
	}()
	if len(screen.files) == 0 {
		return
	}
	diffWidth := screen.diffWidth()
	file := screen.files[screen.fileCursor]
	commentWidth := max(screen.width-screen.fileListWidth()-7, 20)
	appendThreads := func(lineIndex int) {
		for _, key := range file.lines[lineIndex].threadKeys(file.path) {
			for _, thread := range screen.threads[key] {
				for _, text := range thread.render(commentWidth) {
					screen.rows = append(screen.rows, viewRow{left: lineIndex, right: lineIndex, thread: thread, comment: text})
				}
			}
		}
	}
	if !screen.showsSplit() {
		for index, line := range file.lines {
			for part := range len(line.parts(diffWidth)) {
				screen.rows = append(screen.rows, viewRow{left: index, right: index, part: part})
			}
			appendThreads(index)
		}
		return
	}
	for _, row := range file.splitRows {
		for part := range file.splitRowHeight(row, diffWidth) {
			screen.rows = append(screen.rows, viewRow{left: row.left, right: row.right, part: part})
		}
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
func (screen *diffScreen) cursorLine() int {
	if screen.diffCursor < 0 || screen.diffCursor >= len(screen.rows) {
		return -1
	}
	row := screen.rows[screen.diffCursor]
	if row.right < 0 || (screen.commentOnOldSide && row.left >= 0) {
		return row.left
	}
	return row.right
}

func (screen *diffScreen) rowForLine(line int) int {
	for index, row := range screen.rows {
		if row.thread == nil && (row.left == line || row.right == line) {
			return index
		}
	}
	return 0
}

func (screen *diffScreen) cursorCommentTarget() (CommentTarget, bool) {
	index := screen.cursorLine()
	if index < 0 {
		return CommentTarget{}, false
	}
	if thread := screen.rows[screen.diffCursor].thread; thread != nil {
		return thread.replyTarget(), true
	}
	file := screen.files[screen.fileCursor]
	switch line := file.lines[index]; line.kind {
	case removedLine:
		return CommentTarget{Path: file.path, Line: line.oldNumber, Side: CommentSideOld}, true
	case addedLine, contextLine:
		return CommentTarget{Path: file.path, Line: line.newNumber, Side: CommentSideNew}, true
	}
	return CommentTarget{}, false
}

func (screen *diffScreen) startComment() tea.Cmd {
	target, isCodeLine := screen.cursorCommentTarget()
	if !isCodeLine {
		screen.notice = "Move to a code line to comment on it."
		return nil
	}
	screen.commenting = true
	screen.commentTarget, screen.commentPosting, screen.commentError = target, false, ""
	screen.commentInput = textarea.New()
	screen.commentInput.Placeholder = "Write a comment…"
	screen.commentInput.ShowLineNumbers = false
	screen.commentInput.CharLimit = 0
	screen.commentInput.SetWidth(screen.width)
	screen.commentInput.SetHeight(commentRows)
	screen.commentInput.Cursor.SetMode(cursor.CursorStatic)
	return screen.commentInput.Focus()
}

func (screen *diffScreen) handleCommentKey(message tea.KeyMsg) tea.Cmd {
	switch message.String() {
	case "esc":
		if !screen.commentPosting {
			screen.commenting = false
		}
		return nil
	case "ctrl+s":
		body := strings.TrimSpace(screen.commentInput.Value())
		if screen.commentPosting || body == "" {
			return nil
		}
		screen.commentPosting, screen.commentError = true, ""
		return screen.board.sendComment(screen.pullRequest, screen.commentTarget, body)
	}
	if screen.commentPosting {
		return nil
	}
	var command tea.Cmd
	screen.commentInput, command = screen.commentInput.Update(message)
	return command
}

func (screen *diffScreen) cursorIsOnNewSide(index int) bool {
	row := screen.rows[index]
	return screen.showsSplit() && row.thread == nil && row.left != row.right && screen.cursorLine() == row.right
}

func (screen *diffScreen) renderDiffRow(index, width int) string {
	file := screen.files[screen.fileCursor]
	row := screen.rows[index]
	switch {
	case row.thread != nil:
		return ansi.Truncate("  "+row.comment, width, "…")
	case !screen.showsSplit():
		return partOf(file.lines[row.left].renderLines(width), row.part)
	}
	separator := splitSeparator
	if index == screen.diffCursor && screen.cursorIsOnNewSide(index) {
		separator = " " + ui.CursorMarker + " "
	}
	return partOf(file.renderSplitRow(splitRow{row.left, row.right}, width, separator), row.part)
}

func partOf(lines []string, part int) string {
	if part >= len(lines) {
		return ""
	}
	return lines[part]
}

// diffSeparator sits between the file list and the diff. On the cursor row it carries the
// cursor, unless the cursor is on the new side of a side-by-side row. The cursor is always on the
// first part of a wrapped line, so the parts after it never carry the marker.
func (screen *diffScreen) diffSeparator(index int) string {
	if index != screen.diffCursor || index >= len(screen.rows) || screen.cursorIsOnNewSide(index) {
		return splitSeparator
	}
	return " " + ui.CursorMarker + " "
}

func (screen *diffScreen) keepPositionVisible() {
	screen.fileCursor = max(0, min(screen.fileCursor, len(screen.files)-1))
	screen.fileOffset = min(screen.fileOffset, screen.fileCursor)
	screen.fileOffset = max(screen.fileOffset, screen.fileCursor-screen.diffRows()+1)
	screen.rebuildRows()
	screen.diffCursor = max(0, min(screen.diffCursor, screen.diffRowCount()-1))
	for screen.diffCursor > 0 && screen.rows[screen.diffCursor].part > 0 {
		screen.diffCursor--
	}
	// Scroll far enough to show as many parts of the cursor's line as fit.
	lastPart := screen.diffCursor
	for lastPart+1 < len(screen.rows) && lastPart+1 < screen.diffCursor+screen.diffRows() && screen.rows[lastPart+1].part > 0 {
		lastPart++
	}
	screen.diffOffset = min(screen.diffOffset, screen.diffCursor)
	screen.diffOffset = max(screen.diffOffset, lastPart-screen.diffRows()+1, 0)
}

// nextLineRow is the first row after the given one that starts a line, or the given row when
// there is none.
func (screen *diffScreen) nextLineRow(from int) int {
	for index := from + 1; index < len(screen.rows); index++ {
		if screen.rows[index].part == 0 {
			return index
		}
	}
	return from
}

func (screen *diffScreen) previousLineRow(from int) int {
	for index := from - 1; index >= 0; index-- {
		if screen.rows[index].part == 0 {
			return index
		}
	}
	return from
}

// Update ignores messages about other pull requests, because screens are given every message
// that the host does not handle.
func (screen *diffScreen) Update(message tea.Msg) (plugin.Screen, tea.Cmd) {
	next, command := screen.update(message)
	if next != nil {
		screen.keepPositionVisible()
	}
	return next, command
}

func (screen *diffScreen) update(message tea.Msg) (plugin.Screen, tea.Cmd) {
	switch message := message.(type) {
	case diffLoadedMessage:
		if message.pullRequest.URL != screen.pullRequest.URL {
			return screen, nil
		}
		if message.err != nil {
			if screen.loading {
				return nil, screen.selection.Run(plugin.Effect{Notice: "Diff failed: " + ui.FirstLine(message.err)})
			}
			screen.refreshing = false
			screen.notice = "Diff failed: " + ui.FirstLine(message.err)
			return screen, nil
		}
		screen.loading, screen.refreshing = false, false
		screen.replaceFiles(message.files)
	case commentPostedMessage:
		if message.url != screen.pullRequest.URL {
			return screen, nil
		}
		screen.commentPosting = false
		if message.err != nil {
			screen.commentError = ui.FirstLine(message.err)
			return screen, nil
		}
		screen.commenting = false
		screen.notice = "Comment posted on " + message.target.label() + "."
		return screen, screen.board.loadComments(screen.pullRequest)
	case commentsLoadedMessage:
		if message.url != screen.pullRequest.URL {
			return screen, nil
		}
		if message.err != nil {
			screen.notice = "Comments failed to load: " + ui.FirstLine(message.err)
			return screen, nil
		}
		screen.threads = groupThreads(message.comments)
		screen.dataVersion++
	case tea.KeyMsg:
		if screen.commenting {
			return screen, screen.handleCommentKey(message)
		}
		return screen.handleKey(message.String())
	default:
		if screen.commenting {
			var command tea.Cmd
			screen.commentInput, command = screen.commentInput.Update(message)
			return screen, command
		}
	}
	return screen, nil
}

func (screen *diffScreen) handleKey(key string) (plugin.Screen, tea.Cmd) {
	screen.notice = ""
	switch key {
	case "esc", "q":
		return nil, nil
	case "up", "left":
		screen.fileCursor--
		screen.diffCursor, screen.diffOffset = 0, 0
	case "down", "right":
		screen.fileCursor++
		screen.diffCursor, screen.diffOffset = 0, 0
	case "s":
		if len(screen.files) > 0 {
			line := screen.cursorLine()
			screen.toggleLayout()
			screen.rebuildRows()
			screen.diffCursor = screen.rowForLine(line)
		}
	case "tab":
		screen.commentOnOldSide = !screen.commentOnOldSide
	case "c":
		return screen, screen.startComment()
	case "k":
		screen.diffCursor = screen.previousLineRow(screen.diffCursor)
	case "j":
		screen.diffCursor = screen.nextLineRow(screen.diffCursor)
	case "pgup", "b":
		screen.diffCursor -= screen.diffRows()
	case "pgdown", " ":
		screen.diffCursor += screen.diffRows()
	case "home":
		screen.diffCursor = 0
	case "end":
		screen.diffCursor = screen.diffRowCount()
	case "o":
		return screen, commandToOpenInBrowser(screen.pullRequest.URL)
	case "v":
		return screen, screen.board.commandToOpenInEditor(screen.selection)
	case "p":
		return screen, screen.selection.Run(plugin.Effect{Screen: screen.board.newDescriptionScreen(screen.selection)})
	default:
		if command, isAction := screen.board.runScreenAction(screen.selection, key); isAction {
			return screen, command
		}
	}
	return screen, nil
}

func (screen *diffScreen) body() []string {
	if screen.loading {
		return []string{ui.DimStyle.Render("Loading diff…")}
	}
	if len(screen.files) == 0 {
		return []string{ui.DimStyle.Render("No changes.")}
	}
	listWidth := screen.fileListWidth()
	diffWidth := screen.diffWidth()
	rowCount := screen.diffRowCount()
	var body []string
	for row := range screen.diffRows() {
		left := strings.Repeat(" ", listWidth)
		if index := screen.fileOffset + row; index < len(screen.files) {
			segments := screen.files[index].listSegments(listWidth)
			if index == screen.fileCursor {
				left = ui.SelectedStyle.Render(ui.Fit(ui.PlainText(segments), listWidth))
			} else {
				left = ui.Fit(ui.StyledText(segments), listWidth)
			}
		}
		right := ""
		if index := screen.diffOffset + row; index < rowCount {
			right = screen.renderDiffRow(index, diffWidth)
		}
		body = append(body, left+screen.diffSeparator(screen.diffOffset+row)+right)
	}
	return body
}

func (screen *diffScreen) commentPanel() []string {
	title := ui.BoldStyle.Render("Comment on " + screen.commentTarget.label())
	if screen.commentTarget.ReplyTo != 0 {
		title = ui.BoldStyle.Render("Reply to " + screen.commentTarget.ReplyAuthor + " on " + screen.commentTarget.label())
	}
	switch {
	case screen.commentPosting:
		title += ui.DimStyle.Render("  Posting…")
	case screen.commentError != "":
		title += "  " + ui.RedStyle.Render("Failed: "+screen.commentError)
	}
	return []string{
		ansi.Truncate(title, screen.width, "…"),
		screen.commentInput.View(),
		ui.HelpLine("ctrl+s", "post", "esc", "cancel"),
	}
}

func (screen *diffScreen) help() string {
	position := ""
	if len(screen.files) > 0 {
		position = ui.DimStyle.Render(fmt.Sprintf("file %d of %d · ", screen.fileCursor+1, len(screen.files)))
	}
	layout := "split"
	if screen.showsSplit() {
		layout = "unified"
	}
	keysAndLabels := []string{
		"↑/↓", "file", "j/k", "line", "space/b", "page", "c", "comment", "tab", "old/new side", "s", layout,
		"p", "description", "o", "open", "v", screen.board.editor.Name,
	}
	keysAndLabels = append(keysAndLabels, screen.board.screenActionHelp()...)
	return position + ui.HelpLine(append(keysAndLabels, "esc", "back")...)
}

// View adopts the size it is drawn at, so that what follows a resize is laid out for it.
func (screen *diffScreen) View(width, height int) string {
	if width != screen.width && screen.commenting {
		screen.commentInput.SetWidth(width)
	}
	screen.width, screen.height = width, height
	screen.keepPositionVisible()
	pr := screen.pullRequest
	header := ui.BoldStyle.Render(pr.label()) + "  " + pr.Title
	if screen.refreshing {
		header += ui.DimStyle.Render("  Refreshing…")
	}
	header += "  " + ui.StyledText(lineCountCellSegments(screen.lineCount))
	lines := append([]string{ansi.Truncate(header, screen.width, "…")}, screen.body()...)
	for len(lines) < screen.diffRows()+1 {
		lines = append(lines, "")
	}
	if screen.commenting {
		lines = append(lines, screen.commentPanel()...)
	} else {
		lines = append(lines, ansi.Truncate(screen.notice, screen.width, "…"), ansi.Truncate(screen.help(), screen.width, "…"))
	}
	return strings.Join(lines, "\n")
}
