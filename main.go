// prs is a terminal board of open pull requests across a configured set of repositories.
package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

const (
	columnGap    = "  "
	titleColumn  = 2
	minimumTitle = 20
	commentRows  = 4
)

var columnHeaders = []string{"Repo", "#", "Title", "Author", "Lines", "State", "Review requested", "Updated"}

const pullRequestFields = `
fragment pullRequestFields on Repository {
  pullRequests(states: OPEN, first: 100, orderBy: {field: UPDATED_AT, direction: DESC}) {
    nodes {
      number
      title
      url
      isDraft
      headRefOid
      autoMergeRequest { enabledAt }
      isInMergeQueue
      updatedAt
      reviewDecision
      author { login __typename }
      reviewRequests(first: 20) {
        nodes {
          requestedReviewer {
            __typename
            ... on User { login }
            ... on Team { slug }
          }
        }
      }
    }
  }
}
`

const lineCountsQuery = `
query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, first: 100) { nodes { url additions deletions } }
  }
}
`

const ciStatesQuery = `
query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, first: 100) {
      nodes { url commits(last: 1) { nodes { commit { statusCheckRollup { state } } } } }
    }
  }
}
`

// darkBackground is read once at startup, because asking the terminal for its background
// colour while the board is reading keys would swallow the terminal's reply.
var darkBackground = lipgloss.HasDarkBackground()

var syntaxTheme = func() *chroma.Style {
	if darkBackground {
		return styles.Get("github-dark")
	}
	return styles.Get("github")
}()

var (
	addedBackground   = lipgloss.AdaptiveColor{Light: "#dafbe1", Dark: "#12261e"}
	removedBackground = lipgloss.AdaptiveColor{Light: "#ffebe9", Dark: "#2d1517"}
)

var (
	plainStyle    = lipgloss.NewStyle()
	boldStyle     = lipgloss.NewStyle().Bold(true)
	dimStyle      = lipgloss.NewStyle().Faint(true)
	greenStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	redStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	cyanStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	youStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	teamStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
	keyStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	cursorMarker  = keyStyle.Render("▌")
	menuStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2)
)

// execute runs an external command and returns its standard output. Tests replace it.
var execute = func(arguments ...string) ([]byte, error) {
	var standardError bytes.Buffer
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Stderr = &standardError
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(standardError.String())
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}

type pullRequest struct {
	owner          string
	repository     string
	number         int
	title          string
	url            string
	author         string
	isBot          bool
	isDraft        bool
	headCommit     string
	autoMerge      bool
	inMergeQueue   bool
	lineCount      *lineCount
	reviewDecision string
	ciState        *string
	updatedAt      time.Time
	requestedUsers []string
	requestedTeams []string
}

type pullRequestNode struct {
	Number           int
	Title            string
	URL              string `json:"url"`
	IsDraft          bool
	HeadRefOid       string
	AutoMergeRequest *struct{ EnabledAt string }
	IsInMergeQueue   bool
	UpdatedAt        time.Time
	ReviewDecision   string
	Author           *struct {
		Login    string
		Typename string `json:"__typename"`
	}
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *struct {
				Typename string `json:"__typename"`
				Login    string
				Slug     string
			}
		}
	}
}

func (node pullRequestNode) pullRequest(source repository) pullRequest {
	result := pullRequest{
		owner:          source.owner,
		repository:     source.name,
		number:         node.Number,
		title:          node.Title,
		url:            node.URL,
		author:         "ghost",
		isDraft:        node.IsDraft,
		headCommit:     node.HeadRefOid,
		autoMerge:      node.AutoMergeRequest != nil,
		inMergeQueue:   node.IsInMergeQueue,
		reviewDecision: node.ReviewDecision,
		updatedAt:      node.UpdatedAt,
	}
	if node.Author != nil {
		result.author = node.Author.Login
		result.isBot = node.Author.Typename == "Bot"
	}
	for _, request := range node.ReviewRequests.Nodes {
		reviewer := request.RequestedReviewer
		if reviewer == nil {
			continue
		}
		if reviewer.Typename == "Team" {
			result.requestedTeams = append(result.requestedTeams, reviewer.Slug)
		} else if reviewer.Login != "" {
			result.requestedUsers = append(result.requestedUsers, reviewer.Login)
		}
	}
	return result
}

// fetchPullRequests asks for every repository in one query. The owner is written into each
// alias, because the repositories may belong to different owners.
func fetchPullRequests(repositories []repository) (pullRequests []pullRequest, viewer string, err error) {
	var selections strings.Builder
	for index, source := range repositories {
		fmt.Fprintf(&selections, "repository%d: repository(owner: %q, name: %q) { ...pullRequestFields }\n", index, source.owner, source.name)
	}
	query := "query { viewer { login }\n" + selections.String() + "}\n" + pullRequestFields

	output, err := execute("gh", "api", "graphql", "-f", "query="+query)
	if err != nil {
		return nil, "", err
	}
	var response struct {
		Data map[string]json.RawMessage
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, "", err
	}
	var viewerNode struct{ Login string }
	if err := json.Unmarshal(response.Data["viewer"], &viewerNode); err != nil {
		return nil, "", err
	}
	for index, source := range repositories {
		var repository struct {
			PullRequests struct{ Nodes []pullRequestNode }
		}
		if err := json.Unmarshal(response.Data[fmt.Sprintf("repository%d", index)], &repository); err != nil {
			return nil, "", err
		}
		for _, node := range repository.PullRequests.Nodes {
			pullRequests = append(pullRequests, node.pullRequest(source))
		}
	}
	sort.SliceStable(pullRequests, func(i, j int) bool {
		return pullRequests[i].updatedAt.After(pullRequests[j].updatedAt)
	})
	return pullRequests, viewerNode.Login, nil
}

type lineCount struct {
	Additions int
	Deletions int
}

// fetchLineCounts and fetchCIStates are separate from fetchPullRequests because GitHub
// computes both slowly, and the board should not wait for them.
func fetchLineCounts(source repository) (map[string]lineCount, error) {
	output, err := execute(
		"gh", "api", "graphql", "-f", "query="+lineCountsQuery, "-f", "owner="+source.owner, "-f", "name="+source.name,
	)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			Repository struct {
				PullRequests struct {
					Nodes []struct {
						URL string `json:"url"`
						lineCount
					}
				}
			}
		}
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}
	counts := map[string]lineCount{}
	for _, node := range response.Data.Repository.PullRequests.Nodes {
		counts[node.URL] = node.lineCount
	}
	return counts, nil
}

// fetchCIStates maps each open pull request's URL to its CI state, with "" for no checks.
func fetchCIStates(source repository) (map[string]string, error) {
	output, err := execute(
		"gh", "api", "graphql", "-f", "query="+ciStatesQuery, "-f", "owner="+source.owner, "-f", "name="+source.name,
	)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			Repository struct {
				PullRequests struct {
					Nodes []struct {
						URL     string `json:"url"`
						Commits struct {
							Nodes []struct {
								Commit struct {
									StatusCheckRollup *struct{ State string }
								}
							}
						}
					}
				}
			}
		}
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}
	states := map[string]string{}
	for _, node := range response.Data.Repository.PullRequests.Nodes {
		states[node.URL] = ""
		if len(node.Commits.Nodes) > 0 && node.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
			states[node.URL] = node.Commits.Nodes[0].Commit.StatusCheckRollup.State
		}
	}
	return states, nil
}

func (pr pullRequest) label() string {
	return fmt.Sprintf("%s #%d", pr.repository, pr.number)
}

// fullName is how filters identify the repository, since two owners can have repositories
// with the same name.
func (pr pullRequest) fullName() string {
	return pr.owner + "/" + pr.repository
}

func (pr pullRequest) toggleDraftNotice() string {
	if pr.isDraft {
		return "Marking " + pr.label() + " ready for review…"
	}
	return "Converting " + pr.label() + " back to draft…"
}

func (pr pullRequest) toggleDraftCommand() []string {
	command := []string{"gh", "pr", "ready", pr.url}
	if !pr.isDraft {
		command = append(command, "--undo")
	}
	return command
}

// fetchChangedFiles lists the files the pull request touches that still exist on its branch.
func fetchChangedFiles(pr pullRequest) ([]string, error) {
	output, err := execute("gh", "api", pr.apiEndpoint()+"/files?per_page=100")
	if err != nil {
		return nil, err
	}
	var files []struct {
		Filename string
		Status   string
	}
	if err := json.Unmarshal(output, &files); err != nil {
		return nil, err
	}
	var names []string
	for _, file := range files {
		if file.Status != "removed" {
			names = append(names, file.Filename)
		}
	}
	return names, nil
}

// openInEditorCommand has worktrunk find or create the pull request's worktree in the local
// clone at <cloneRoot>/<repository>, then open that worktree and the given files in the editor.
func (pr pullRequest) openInEditorCommand(editorCommand, cloneRoot string, files []string) []string {
	command := []string{
		"wt", "-C", filepath.Join(cloneRoot, pr.repository), "switch", fmt.Sprintf("pr:%d", pr.number),
		"--no-cd", "-x", editorCommand, "--", ".",
	}
	return append(command, files...)
}

func (pr pullRequest) toggleAutoMergeQuestion() string {
	if pr.autoMerge {
		return "Disable auto-merge on " + pr.label() + "?"
	}
	return "Enable auto-merge (squash) on " + pr.label() + "? It merges as soon as its requirements are met."
}

func (pr pullRequest) toggleAutoMergeCommand() []string {
	if pr.autoMerge {
		return []string{"gh", "pr", "merge", pr.url, "--disable-auto"}
	}
	return []string{"gh", "pr", "merge", pr.url, "--auto", "--squash"}
}

func (pr pullRequest) apiEndpoint() string {
	return fmt.Sprintf("repos/%s/%s/pulls/%d", pr.owner, pr.repository, pr.number)
}

func fetchDescription(pr pullRequest) (string, error) {
	output, err := execute("gh", "api", pr.apiEndpoint())
	if err != nil {
		return "", err
	}
	var response struct{ Body string }
	if err := json.Unmarshal(output, &response); err != nil {
		return "", err
	}
	return strings.ReplaceAll(response.Body, "\r\n", "\n"), nil
}

// sendJSON sends the payload as a JSON file so that gh passes it through untouched.
func sendJSON(method, endpoint string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp("", "prs-request-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(encoded); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = execute("gh", "api", "--method", method, endpoint, "--input", file.Name())
	return err
}

func saveDescription(pr pullRequest, body string) error {
	return sendJSON("PATCH", pr.apiEndpoint(), map[string]string{"body": body})
}

// commentTarget is the diff line a review comment attaches to. Side is LEFT for a line
// that exists only in the old file, and RIGHT otherwise.
type commentTarget struct {
	path        string
	line        int
	side        string
	replyTo     int64
	replyAuthor string
}

func (target commentTarget) label() string {
	version := "new"
	if target.side == "LEFT" {
		version = "old"
	}
	return fmt.Sprintf("%s line %d (%s)", target.path, target.line, version)
}

func postComment(pr pullRequest, target commentTarget, body string) error {
	if target.replyTo != 0 {
		return sendJSON("POST", pr.apiEndpoint()+"/comments", map[string]any{"body": body, "in_reply_to": target.replyTo})
	}
	return sendJSON("POST", pr.apiEndpoint()+"/comments", map[string]any{
		"body":      body,
		"commit_id": pr.headCommit,
		"path":      target.path,
		"line":      target.line,
		"side":      target.side,
	})
}

type reviewComment struct {
	ID        int64
	InReplyTo int64 `json:"in_reply_to_id"`
	Path      string
	Line      int
	Side      string
	User      struct{ Login string }
	Body      string
	CreatedAt time.Time `json:"created_at"`
}

type commentThread struct {
	root    reviewComment
	replies []reviewComment
}

type threadKey struct {
	path string
	side string
	line int
}

func fetchComments(pr pullRequest) ([]reviewComment, error) {
	output, err := execute("gh", "api", pr.apiEndpoint()+"/comments?per_page=100")
	if err != nil {
		return nil, err
	}
	var comments []reviewComment
	return comments, json.Unmarshal(output, &comments)
}

// groupThreads keys each thread by the diff line it sits on. GitHub reports no line for a
// comment whose code has since changed, so those outdated threads are left out.
func groupThreads(comments []reviewComment) map[threadKey][]*commentThread {
	threads := map[threadKey][]*commentThread{}
	byRoot := map[int64]*commentThread{}
	for _, comment := range comments {
		if comment.InReplyTo == 0 && comment.Line > 0 {
			thread := &commentThread{root: comment}
			byRoot[comment.ID] = thread
			key := threadKey{comment.Path, comment.Side, comment.Line}
			threads[key] = append(threads[key], thread)
		}
	}
	for _, comment := range comments {
		if thread := byRoot[comment.InReplyTo]; thread != nil {
			thread.replies = append(thread.replies, comment)
		}
	}
	return threads
}

var commentSanitizer = strings.NewReplacer("\r", "", "\x1b", "", "\t", "    ")

func (thread commentThread) render(width int) []string {
	var lines []string
	for index, comment := range append([]reviewComment{thread.root}, thread.replies...) {
		corner := "┌ "
		if index > 0 {
			corner = "├ "
		}
		heading := boldStyle.Render(comment.User.Login) + dimStyle.Render(" · "+ageText(comment.CreatedAt))
		lines = append(lines, yellowStyle.Render(corner)+heading)
		body := commentSanitizer.Replace(strings.TrimSpace(comment.Body))
		for _, text := range strings.Split(ansi.Wrap(body, max(width-2, 10), ""), "\n") {
			lines = append(lines, yellowStyle.Render("│ ")+text)
		}
	}
	return lines
}

func (thread commentThread) replyTarget() commentTarget {
	root := thread.root
	return commentTarget{
		path: root.Path, line: root.Line, side: root.Side, replyTo: root.ID, replyAuthor: root.User.Login,
	}
}

func (line diffLine) threadKeys(path string) []threadKey {
	switch line.kind {
	case removedLine:
		return []threadKey{{path, "LEFT", line.oldNumber}}
	case addedLine:
		return []threadKey{{path, "RIGHT", line.newNumber}}
	case contextLine:
		return []threadKey{{path, "RIGHT", line.newNumber}, {path, "LEFT", line.oldNumber}}
	}
	return nil
}

// viewRow is one row of the diff pane in the current layout: a diff line, a side-by-side
// pair, or one line of a comment thread anchored to the diff line above it.
type viewRow struct {
	left    int
	right   int
	thread  *commentThread
	comment string
}

func renderMarkdown(body string, width int) []string {
	body = strings.ReplaceAll(body, "\x1b", "")
	if strings.TrimSpace(body) == "" {
		return []string{dimStyle.Render("No description.")}
	}
	theme := "light"
	if darkBackground {
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

// requestTeamReviewCommands marks a draft ready first, because a review request on a draft
// does not ask anyone to look yet.
func (pr pullRequest) requestTeamReviewCommands(reviewTeam string) [][]string {
	if pr.isDraft {
		return [][]string{pr.toggleDraftCommand(), pr.requestTeamReviewCommand(reviewTeam)}
	}
	return [][]string{pr.requestTeamReviewCommand(reviewTeam)}
}

func (pr pullRequest) requestTeamReviewNotice(reviewTeam string) string {
	if pr.isDraft {
		return "Marking " + pr.label() + " ready and requesting review from " + reviewTeam + "…"
	}
	return "Requesting review from " + reviewTeam + " on " + pr.label() + "…"
}

func (pr pullRequest) requestTeamReviewCommand(reviewTeam string) []string {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/requested_reviewers", pr.owner, pr.repository, pr.number)
	return []string{"gh", "api", "--method", "POST", endpoint, "-f", "team_reviewers[]=" + reviewTeam}
}

type segment struct {
	text  string
	style lipgloss.Style
}

// awaitsReviewFrom reports whether someone else's pull request has a review requested from
// the viewer or from the review team. Without a review team, only the viewer counts.
func (pr pullRequest) awaitsReviewFrom(viewer, reviewTeam string) bool {
	if pr.author == viewer {
		return false
	}
	return slices.Contains(pr.requestedUsers, viewer) || (reviewTeam != "" && slices.Contains(pr.requestedTeams, reviewTeam))
}

// stateMarks summarises the pull request as emoji: review state, whether it waits on the
// viewer, merge queue and auto-merge, then CI.
func (pr pullRequest) stateMarks(viewer, reviewTeam string) string {
	var marks []string
	switch {
	case pr.isDraft:
		marks = append(marks, "📝")
	case pr.reviewDecision == "APPROVED":
		marks = append(marks, "✅")
	case pr.reviewDecision == "CHANGES_REQUESTED":
		marks = append(marks, "❌")
	}
	if pr.awaitsReviewFrom(viewer, reviewTeam) {
		marks = append(marks, "✋")
	}
	if pr.inMergeQueue {
		marks = append(marks, "♻️")
	}
	if pr.autoMerge {
		marks = append(marks, "⏩")
	}
	switch {
	case pr.ciState == nil:
		marks = append(marks, "…")
	case *pr.ciState == "SUCCESS":
		marks = append(marks, "🟢")
	case *pr.ciState == "FAILURE", *pr.ciState == "ERROR":
		marks = append(marks, "🔴")
	case *pr.ciState == "PENDING", *pr.ciState == "EXPECTED":
		marks = append(marks, "🟡")
	}
	return strings.Join(marks, " ")
}

func (pr pullRequest) reviewSegments(viewer, reviewTeam string) []segment {
	var reviewers []segment
	if slices.Contains(pr.requestedUsers, viewer) {
		reviewers = append(reviewers, segment{"you", youStyle})
	}
	if reviewTeam != "" && slices.Contains(pr.requestedTeams, reviewTeam) {
		reviewers = append(reviewers, segment{reviewTeam, teamStyle})
	}
	for _, login := range pr.requestedUsers {
		if login != viewer {
			reviewers = append(reviewers, segment{login, dimStyle})
		}
	}
	for _, slug := range pr.requestedTeams {
		if slug != reviewTeam {
			reviewers = append(reviewers, segment{slug, dimStyle})
		}
	}
	var segments []segment
	for index, reviewer := range reviewers {
		if index > 0 {
			segments = append(segments, segment{", ", plainStyle})
		}
		segments = append(segments, reviewer)
	}
	return segments
}

func (pr pullRequest) lineCountSegments() []segment {
	if pr.lineCount == nil {
		return []segment{{"…", dimStyle}}
	}
	return lineCountSegments(pr.lineCount.Additions, pr.lineCount.Deletions)
}

func lineCountSegments(additions, deletions int) []segment {
	return []segment{
		{fmt.Sprintf("+%d", additions), greenStyle},
		{" ", plainStyle},
		{fmt.Sprintf("−%d", deletions), redStyle},
	}
}

func (pr pullRequest) age() string {
	return ageText(pr.updatedAt)
}

func ageText(since time.Time) string {
	minutes := int(time.Since(since).Minutes())
	switch {
	case minutes < 60:
		return fmt.Sprintf("%dm", minutes)
	case minutes < 60*24:
		return fmt.Sprintf("%dh", minutes/60)
	}
	return fmt.Sprintf("%dd", minutes/(60*24))
}

func (pr pullRequest) columns(viewer, reviewTeam string) [][]segment {
	authorStyle := plainStyle
	if pr.isBot {
		authorStyle = dimStyle
	}
	return [][]segment{
		{{pr.repository, plainStyle}},
		{{strconv.Itoa(pr.number), plainStyle}},
		{{pr.title, plainStyle}},
		{{pr.author, authorStyle}},
		pr.lineCountSegments(),
		{{pr.stateMarks(viewer, reviewTeam), plainStyle}},
		pr.reviewSegments(viewer, reviewTeam),
		{{pr.age(), plainStyle}},
	}
}

func plainText(segments []segment) string {
	var text strings.Builder
	for _, part := range segments {
		text.WriteString(part.text)
	}
	return text.String()
}

func styledText(segments []segment) string {
	var text strings.Builder
	for _, part := range segments {
		text.WriteString(part.style.Render(part.text))
	}
	return text.String()
}

func fit(text string, width int) string {
	text = ansi.Truncate(text, width, "…")
	return text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}

// renderTable draws the header and the rows from offset that fit in rowCount lines.
// Pass selected = -1 to draw no cursor.
func renderTable(pullRequests []pullRequest, viewer, reviewTeam string, width, selected, offset, rowCount int) string {
	widths := make([]int, len(columnHeaders))
	for index, header := range columnHeaders {
		widths[index] = lipgloss.Width(header)
	}
	rows := make([][][]segment, len(pullRequests))
	for rowIndex, pr := range pullRequests {
		rows[rowIndex] = pr.columns(viewer, reviewTeam)
		for index, column := range rows[rowIndex] {
			widths[index] = max(widths[index], lipgloss.Width(plainText(column)))
		}
	}
	widthOfOtherColumns := len(columnGap) * (len(widths) - 1)
	for index, columnWidth := range widths {
		if index != titleColumn {
			widthOfOtherColumns += columnWidth
		}
	}
	widths[titleColumn] = max(minimumTitle, min(widths[titleColumn], width-widthOfOtherColumns))

	headerCells := make([]string, len(columnHeaders))
	for index, header := range columnHeaders {
		headerCells[index] = fit(header, widths[index])
	}
	lines := []string{boldStyle.Render(ansi.Truncate(strings.Join(headerCells, columnGap), width, "…"))}

	end := min(len(rows), offset+max(rowCount, 0))
	for rowIndex := offset; rowIndex < end; rowIndex++ {
		cells := make([]string, len(widths))
		for index, column := range rows[rowIndex] {
			if rowIndex == selected {
				cells[index] = fit(plainText(column), widths[index])
			} else {
				cells[index] = fit(styledText(column), widths[index])
			}
		}
		line := ansi.Truncate(strings.Join(cells, columnGap), width, "…")
		if rowIndex == selected {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// filters keys the selected repositories by full name. available is the configured
// repositories in order, which is also the order of the number keys in the filter menu.
type filters struct {
	available    []repository
	repositories map[string]bool
	showBots     bool
}

// maximumFilterKeys is how many repositories get a number key, as the keys are 1 to 9.
const maximumFilterKeys = 9

func newFilters(available []repository, selectedRepositories []string, showBots bool) filters {
	selected := map[string]bool{}
	if len(selectedRepositories) == 0 {
		for _, source := range available {
			selected[source.fullName()] = true
		}
	}
	for _, name := range selectedRepositories {
		selected[name] = true
	}
	return filters{available: available, repositories: selected, showBots: showBots}
}

func (f filters) matches(pr pullRequest) bool {
	return f.repositories[pr.fullName()] && pr.isBot == f.showBots
}

func (f filters) visible(pullRequests []pullRequest) []pullRequest {
	var visible []pullRequest
	for _, pr := range pullRequests {
		if f.matches(pr) {
			visible = append(visible, pr)
		}
	}
	return visible
}

func (f filters) authors() string {
	if f.showBots {
		return "bots"
	}
	return "people"
}

func (f filters) repositoryMark(source repository) string {
	if f.repositories[source.fullName()] {
		return "● " + source.name
	}
	return dimStyle.Render("○ " + source.name)
}

func (f filters) summary() string {
	parts := make([]string, len(f.available))
	for index, source := range f.available {
		parts[index] = f.repositoryMark(source)
	}
	return "Repositories: " + strings.Join(parts, "  ") + "    Authors: " + boldStyle.Render(f.authors())
}

func (f filters) menu() string {
	lines := []string{boldStyle.Render("Filters"), ""}
	for index, source := range f.available {
		key := " "
		if index < maximumFilterKeys {
			key = keyStyle.Render(strconv.Itoa(index + 1))
		}
		lines = append(lines, key+"  "+f.repositoryMark(source))
	}
	lines = append(lines, "", keyStyle.Render("b")+"  authors: "+f.authors())
	lines = append(lines, "", keyStyle.Render("esc")+"  close")
	return menuStyle.Render(strings.Join(lines, "\n"))
}

type lineKind int

const (
	metadataLine lineKind = iota
	hunkLine
	contextLine
	addedLine
	removedLine
)

type diffLine struct {
	text      string
	kind      lineKind
	code      []segment
	oldNumber int
	newNumber int
}

// splitRow is one row of the side-by-side view, as indexes into a file's lines. A side
// with nothing on it is -1, and a line that spans both sides has the same index twice.
type splitRow struct {
	left  int
	right int
}

type diffFile struct {
	path        string
	additions   int
	deletions   int
	lines       []diffLine
	splitRows   []splitRow
	numberWidth int
}

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)`)

var diffLineSanitizer = strings.NewReplacer("\t", "    ", "\r", "", "\x1b", "␛")

// parseDiff splits a unified diff into its files. Lines before a file's first hunk are
// metadata, so a leading + or - only counts as a change once a hunk has started.
func parseDiff(text string) []diffFile {
	var files []diffFile
	inHunk := false
	oldNumber, newNumber := 0, 0
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			path := line
			if index := strings.LastIndex(line, " b/"); index >= 0 {
				path = line[index+3:]
			}
			files = append(files, diffFile{path: path})
			inHunk = false
			continue
		}
		if len(files) == 0 {
			continue
		}
		file := &files[len(files)-1]
		parsed := diffLine{text: diffLineSanitizer.Replace(line)}
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
			parsed.kind = hunkLine
			if numbers := hunkHeaderPattern.FindStringSubmatch(line); numbers != nil {
				oldNumber, _ = strconv.Atoi(numbers[1])
				newNumber, _ = strconv.Atoi(numbers[2])
			}
		case !inHunk || strings.HasPrefix(line, `\`):
		case strings.HasPrefix(line, "+"):
			file.additions++
			parsed.kind, parsed.newNumber = addedLine, newNumber
			newNumber++
		case strings.HasPrefix(line, "-"):
			file.deletions++
			parsed.kind, parsed.oldNumber = removedLine, oldNumber
			oldNumber++
		default:
			parsed.kind, parsed.oldNumber, parsed.newNumber = contextLine, oldNumber, newNumber
			oldNumber++
			newNumber++
		}
		file.lines = append(file.lines, parsed)
	}
	for index := range files {
		files[index].highlight()
		files[index].pairLines()
	}
	return files
}

// highlight tokenises each hunk as one block of source, so a string or comment that spans
// several lines of the hunk keeps its colour.
func (file *diffFile) highlight() {
	lexer := lexers.Match(file.path)
	if lexer == nil {
		return
	}
	lexer = chroma.Coalesce(lexer)
	start := 0
	for start < len(file.lines) {
		if file.lines[start].kind < contextLine {
			start++
			continue
		}
		end := start
		var source strings.Builder
		for end < len(file.lines) && file.lines[end].kind >= contextLine {
			if text := file.lines[end].text; text != "" {
				source.WriteString(text[1:])
			}
			source.WriteString("\n")
			end++
		}
		if iterator, err := lexer.Tokenise(nil, source.String()); err == nil {
			for offset, tokens := range chroma.SplitTokensIntoLines(iterator.Tokens()) {
				if start+offset < end {
					file.lines[start+offset].code = codeSegments(tokens)
				}
			}
		}
		start = end
	}
}

// pairLines lays the file out for the side-by-side view: each run of removed lines sits
// beside the added lines that replace it.
func (file *diffFile) pairLines() {
	lines := file.lines
	highestNumber := 0
	for index := 0; index < len(lines); {
		highestNumber = max(highestNumber, lines[index].oldNumber, lines[index].newNumber)
		if lines[index].kind != removedLine && lines[index].kind != addedLine {
			file.splitRows = append(file.splitRows, splitRow{index, index})
			index++
			continue
		}
		removedStart := index
		for index < len(lines) && lines[index].kind == removedLine {
			index++
		}
		addedStart := index
		for index < len(lines) && lines[index].kind == addedLine {
			highestNumber = max(highestNumber, lines[index].newNumber)
			index++
		}
		removed, added := addedStart-removedStart, index-addedStart
		for offset := range max(removed, added) {
			row := splitRow{-1, -1}
			if offset < removed {
				row.left = removedStart + offset
			}
			if offset < added {
				row.right = addedStart + offset
			}
			file.splitRows = append(file.splitRows, row)
		}
	}
	file.numberWidth = len(strconv.Itoa(highestNumber))
}

func (file diffFile) renderSplitRow(row splitRow, width int, separator string) string {
	if row.left == row.right && file.lines[row.left].kind < contextLine {
		return file.lines[row.left].render(width)
	}
	side := (width - 3) / 2
	return file.renderSide(row.left, side, true) + separator + file.renderSide(row.right, side, false)
}

func (file diffFile) renderSide(index, width int, isOldSide bool) string {
	if index < 0 {
		return strings.Repeat(" ", width)
	}
	line := file.lines[index]
	number := line.newNumber
	if isOldSide {
		number = line.oldNumber
	}
	codeWidth := max(width-file.numberWidth-1, 1)
	return dimStyle.Render(fmt.Sprintf("%*d ", file.numberWidth, number)) + fit(line.render(codeWidth), codeWidth)
}

func codeSegments(tokens []chroma.Token) []segment {
	segments := []segment{}
	for _, token := range tokens {
		text := strings.TrimRight(token.Value, "\n")
		if text == "" {
			continue
		}
		style := plainStyle
		entry := syntaxTheme.Get(token.Type)
		if entry.Colour.IsSet() {
			style = style.Foreground(lipgloss.Color(entry.Colour.String()))
		}
		if entry.Bold == chroma.Yes {
			style = style.Bold(true)
		}
		segments = append(segments, segment{text, style})
	}
	return segments
}

func (line diffLine) render(width int) string {
	switch line.kind {
	case metadataLine:
		return dimStyle.Render(ansi.Truncate(line.text, width, "…"))
	case hunkLine:
		return cyanStyle.Render(ansi.Truncate(line.text, width, "…"))
	}
	marker, code := "", line.code
	if line.text != "" {
		marker = line.text[:1]
		if code == nil {
			code = []segment{{line.text[1:], plainStyle}}
		}
	}
	if line.kind == contextLine {
		return ansi.Truncate(marker+styledText(code), width, "…")
	}
	markerStyle, background := greenStyle, addedBackground
	if line.kind == removedLine {
		markerStyle, background = redStyle, removedBackground
	}
	var rendered strings.Builder
	rendered.WriteString(markerStyle.Background(background).Render(marker))
	for _, part := range code {
		rendered.WriteString(part.style.Background(background).Render(part.text))
	}
	text := ansi.Truncate(rendered.String(), width, "…")
	padding := strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
	return text + lipgloss.NewStyle().Background(background).Render(padding)
}

func (file diffFile) listSegments(width int) []segment {
	counts := lineCountSegments(file.additions, file.deletions)
	path := []rune(file.path)
	available := max(width-lipgloss.Width(plainText(counts))-1, 1)
	if len(path) > available {
		path = append([]rune("…"), path[len(path)-available+1:]...)
	}
	return append(counts, segment{" " + string(path), plainStyle})
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
		pullRequests []pullRequest
		viewer       string
		err          error
	}
	lineCountsLoadedMessage struct{ counts map[string]lineCount }
	ciStatesLoadedMessage   struct{ states map[string]string }
	editorOpenedMessage     struct{ err error }
	commentsLoadedMessage   struct {
		url      string
		comments []reviewComment
		err      error
	}
	commentPostedMessage struct {
		target commentTarget
		err    error
	}
	descriptionLoadedMessage struct {
		pullRequest pullRequest
		body        string
		err         error
	}
	descriptionEditedMessage struct {
		pullRequest pullRequest
		path        string
		err         error
	}
	descriptionSavedMessage struct {
		pullRequest pullRequest
		body        string
		path        string
		err         error
	}
	commandFinishedMessage struct{ err error }
	diffLoadedMessage      struct {
		pullRequest pullRequest
		files       []diffFile
		err         error
	}
	refreshTickMessage struct{}
)

func load(repositories []repository) tea.Cmd {
	return func() tea.Msg {
		pullRequests, viewer, err := fetchPullRequests(repositories)
		return loadedMessage{pullRequests: pullRequests, viewer: viewer, err: err}
	}
}

func detailCommands(repositories []repository) []tea.Cmd {
	var commands []tea.Cmd
	for _, source := range repositories {
		commands = append(commands, func() tea.Msg {
			states, _ := fetchCIStates(source)
			return ciStatesLoadedMessage{states: states}
		})
	}
	for _, source := range repositories {
		commands = append(commands, func() tea.Msg {
			counts, _ := fetchLineCounts(source)
			return lineCountsLoadedMessage{counts: counts}
		})
	}
	return commands
}

func scheduleRefresh(refreshInterval time.Duration) tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return refreshTickMessage{} })
}

// runCommands runs the commands in order and stops at the first failure.
func runCommands(commands ...[]string) tea.Cmd {
	return func() tea.Msg {
		for _, command := range commands {
			if _, err := execute(command...); err != nil {
				return commandFinishedMessage{err: err}
			}
		}
		return commandFinishedMessage{}
	}
}

func loadDiff(pr pullRequest) tea.Cmd {
	return func() tea.Msg {
		output, err := execute("gh", "pr", "diff", pr.url)
		return diffLoadedMessage{pullRequest: pr, files: parseDiff(string(output)), err: err}
	}
}

// cachedDiff is reused only while the pull request's head commit is unchanged.
type cachedDiff struct {
	headCommit string
	files      []diffFile
}

func loadComments(pr pullRequest) tea.Cmd {
	return func() tea.Msg {
		comments, err := fetchComments(pr)
		return commentsLoadedMessage{url: pr.url, comments: comments, err: err}
	}
}

func sendComment(pr pullRequest, target commentTarget, body string) tea.Cmd {
	return func() tea.Msg {
		return commentPostedMessage{target: target, err: postComment(pr, target, body)}
	}
}

func loadDescription(pr pullRequest) tea.Cmd {
	return func() tea.Msg {
		body, err := fetchDescription(pr)
		return descriptionLoadedMessage{pullRequest: pr, body: body, err: err}
	}
}

func updateDescription(pr pullRequest, body, path string) tea.Cmd {
	return func() tea.Msg {
		err := saveDescription(pr, body)
		return descriptionSavedMessage{pullRequest: pr, body: body, path: path, err: err}
	}
}

// openInEditor still opens the worktree when the changed files cannot be listed.
func openInEditor(pr pullRequest, settings configuration) tea.Cmd {
	return func() tea.Msg {
		files, _ := fetchChangedFiles(pr)
		_, err := execute(pr.openInEditorCommand(settings.editor.command, settings.pullRequests.cloneRoot, files)...)
		return editorOpenedMessage{err: err}
	}
}

func openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		execute("open", url)
		return nil
	}
}

func firstLine(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}

type model struct {
	config          configuration
	filters         filters
	viewer          string
	pullRequests    []pullRequest
	lineCounts      map[string]lineCount
	ciStates        map[string]string
	diffCache       map[string]cachedDiff
	visible         []pullRequest
	loadedAt        time.Time
	cursor          int
	offset          int
	width           int
	height          int
	mode            mode
	notice          string
	question        string
	pendingCommand  []string
	diffPullRequest pullRequest
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
	commentTarget    commentTarget
	commentInput     textarea.Model
	commentPosting   bool
	commentError     string

	descriptionPullRequest pullRequest
	descriptionBody        string
	descriptionLines       []string
	descriptionLoading     bool
	descriptionNotice      string
	descriptionOffset      int
	descriptionReturnsTo   mode
}

func newModel(settings configuration, f filters) model {
	return model{config: settings, filters: f, lineCounts: map[string]lineCount{}, ciStates: map[string]string{}, diffCache: map[string]cachedDiff{}, splitDiff: true, width: 120, height: 30}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(load(m.repositories()), scheduleRefresh(m.config.refreshInterval))
}

func (m model) repositories() []repository {
	return m.config.pullRequests.repositories
}

func (m model) selected() (pullRequest, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return pullRequest{}, false
	}
	return m.visible[m.cursor], true
}

// applyFilters recomputes the visible rows and keeps the cursor on the same pull request.
func (m *model) applyFilters() {
	selected, hadSelection := m.selected()
	for index, pr := range m.pullRequests {
		if count, known := m.lineCounts[pr.url]; known {
			m.pullRequests[index].lineCount = &count
		}
		if state, known := m.ciStates[pr.url]; known {
			m.pullRequests[index].ciState = &state
		}
	}
	m.visible = m.filters.visible(m.pullRequests)
	if !hadSelection {
		return
	}
	for index, pr := range m.visible {
		if pr.url == selected.url {
			m.cursor = index
			return
		}
	}
}

func (m model) tableRows() int {
	rows := m.height - 4
	if m.mode == filterMode {
		rows -= lipgloss.Height(m.filters.menu())
	}
	return max(rows, 1)
}

func (m *model) keepCursorVisible() {
	m.cursor = max(0, min(m.cursor, len(m.visible)-1))
	m.offset = min(m.offset, m.cursor)
	m.offset = max(m.offset, m.cursor-m.tableRows()+1)
}

func (m model) diffRows() int {
	if m.mode == commentMode {
		return max(m.height-3-commentRows, 1)
	}
	return max(m.height-3, 1)
}

func (m model) fileListWidth() int {
	widest := 0
	for _, file := range m.diffFiles {
		widest = max(widest, lipgloss.Width(plainText(file.listSegments(m.width))))
	}
	if m.splitDiff {
		return min(widest, max(20, m.width/4))
	}
	return min(widest, max(20, m.width*2/5))
}

func (m model) diffRowCount() int {
	return len(m.rows)
}

// rebuildRows lays out the selected file for the current layout and width, with each
// comment thread directly under the line it is attached to.
func (m *model) rebuildRows() {
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
func (m model) cursorLine() int {
	if m.diffCursor < 0 || m.diffCursor >= len(m.rows) {
		return -1
	}
	row := m.rows[m.diffCursor]
	if row.right < 0 || (m.commentOnOldSide && row.left >= 0) {
		return row.left
	}
	return row.right
}

func (m model) rowForLine(line int) int {
	for index, row := range m.rows {
		if row.thread == nil && (row.left == line || row.right == line) {
			return index
		}
	}
	return 0
}

func (m model) cursorCommentTarget() (commentTarget, bool) {
	index := m.cursorLine()
	if index < 0 {
		return commentTarget{}, false
	}
	if thread := m.rows[m.diffCursor].thread; thread != nil {
		return thread.replyTarget(), true
	}
	file := m.diffFiles[m.fileCursor]
	switch line := file.lines[index]; line.kind {
	case removedLine:
		return commentTarget{path: file.path, line: line.oldNumber, side: "LEFT"}, true
	case addedLine, contextLine:
		return commentTarget{path: file.path, line: line.newNumber, side: "RIGHT"}, true
	}
	return commentTarget{}, false
}

func (m *model) startComment() tea.Cmd {
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

func (m model) handleCommentKey(message tea.KeyMsg) (model, tea.Cmd) {
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
		return m, sendComment(m.diffPullRequest, m.commentTarget, body)
	}
	if m.commentPosting {
		return m, nil
	}
	var command tea.Cmd
	m.commentInput, command = m.commentInput.Update(message)
	return m, command
}

func (m model) cursorIsOnNewSide(index int) bool {
	row := m.rows[index]
	return m.splitDiff && row.thread == nil && row.left != row.right && m.cursorLine() == row.right
}

func (m model) renderDiffRow(index, width int) string {
	file := m.diffFiles[m.fileCursor]
	row := m.rows[index]
	switch {
	case row.thread != nil:
		return ansi.Truncate("  "+row.comment, width, "…")
	case !m.splitDiff:
		return file.lines[row.left].render(width)
	}
	separator := dimStyle.Render(" │ ")
	if index == m.diffCursor && m.cursorIsOnNewSide(index) {
		separator = " " + cursorMarker + " "
	}
	return file.renderSplitRow(splitRow{row.left, row.right}, width, separator)
}

// diffSeparator sits between the file list and the diff. On the cursor row it carries the
// cursor, unless the cursor is on the new side of a side-by-side row.
func (m model) diffSeparator(index int) string {
	if index != m.diffCursor || index >= len(m.rows) || m.cursorIsOnNewSide(index) {
		return dimStyle.Render(" │ ")
	}
	return " " + cursorMarker + " "
}

func (m *model) keepDiffPositionVisible() {
	m.fileCursor = max(0, min(m.fileCursor, len(m.diffFiles)-1))
	m.fileOffset = min(m.fileOffset, m.fileCursor)
	m.fileOffset = max(m.fileOffset, m.fileCursor-m.diffRows()+1)
	m.rebuildRows()
	m.diffCursor = max(0, min(m.diffCursor, m.diffRowCount()-1))
	m.diffOffset = min(m.diffOffset, m.diffCursor)
	m.diffOffset = max(m.diffOffset, m.diffCursor-m.diffRows()+1, 0)
}

func (m model) descriptionRows() int {
	return max(m.height-3, 1)
}

func (m *model) keepDescriptionVisible() {
	m.descriptionOffset = max(0, min(m.descriptionOffset, len(m.descriptionLines)-m.descriptionRows()))
}

func (m *model) openDescription(pr pullRequest) tea.Cmd {
	m.descriptionReturnsTo = m.mode
	m.mode = descriptionMode
	m.descriptionPullRequest = pr
	m.descriptionBody, m.descriptionLines = "", nil
	m.descriptionLoading, m.descriptionNotice, m.descriptionOffset = true, "", 0
	return loadDescription(pr)
}

func (m *model) showDescription(body string) {
	m.descriptionBody = body
	m.descriptionLines = renderMarkdown(body, m.width)
}

// startEditingDescription writes the description to a temporary file for the editor. The
// file is kept if the update fails, so the edit is not lost.
func (m *model) startEditingDescription() tea.Cmd {
	pr := m.descriptionPullRequest
	file, err := os.CreateTemp("", "prs-description-*.md")
	if err == nil {
		_, err = file.WriteString(m.descriptionBody + "\n")
		err = cmp.Or(err, file.Close())
	}
	if err != nil {
		m.descriptionNotice = "Failed: " + firstLine(err)
		return nil
	}
	return editInEditor(file.Name(), func(err error) tea.Msg {
		return descriptionEditedMessage{pullRequest: pr, path: file.Name(), err: err}
	})
}

func (m *model) finishEditingDescription(message descriptionEditedMessage) tea.Cmd {
	if message.err != nil {
		m.descriptionNotice = "Editor failed: " + firstLine(message.err)
		return nil
	}
	edited, err := os.ReadFile(message.path)
	if err != nil {
		m.descriptionNotice = "Failed: " + firstLine(err)
		return nil
	}
	body := strings.TrimSpace(string(edited))
	if body == "" || body == strings.TrimSpace(m.descriptionBody) {
		os.Remove(message.path)
		m.descriptionNotice = "Description unchanged."
		return nil
	}
	m.descriptionNotice = "Updating description…"
	return updateDescription(message.pullRequest, body, message.path)
}

func (m *model) closeDiff() {
	m.mode = boardMode
	m.diffFiles, m.diffLoading, m.diffThreads = nil, false, nil
	m.fileCursor, m.fileOffset, m.diffOffset, m.diffCursor, m.diffNotice = 0, 0, 0, 0, ""
}

func (m *model) ask(question string, command []string) {
	m.mode = confirmMode
	m.question = question
	m.pendingCommand = command
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	next, command := m.update(message)
	next.keepCursorVisible()
	next.keepDiffPositionVisible()
	next.keepDescriptionVisible()
	return next, command
}

func (m model) update(message tea.Msg) (model, tea.Cmd) {
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
		if m.mode != descriptionMode || message.pullRequest.url != m.descriptionPullRequest.url {
			return m, nil
		}
		m.descriptionLoading = false
		if message.err != nil {
			m.descriptionNotice = "Failed: " + firstLine(message.err)
			return m, nil
		}
		m.showDescription(message.body)
	case descriptionEditedMessage:
		return m, m.finishEditingDescription(message)
	case descriptionSavedMessage:
		if message.err != nil {
			m.descriptionNotice = "Failed: " + firstLine(message.err) + " · your edit is kept at " + message.path
			return m, nil
		}
		os.Remove(message.path)
		m.descriptionNotice = "Description updated."
		if message.pullRequest.url == m.descriptionPullRequest.url {
			m.showDescription(message.body)
		}
	case loadedMessage:
		if message.err != nil {
			m.notice = "Refresh failed: " + firstLine(message.err)
			return m, nil
		}
		m.notice = ""
		m.viewer = message.viewer
		m.pullRequests = message.pullRequests
		m.loadedAt = time.Now()
		m.applyFilters()
		return m, tea.Batch(detailCommands(m.repositories())...)
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
			m.notice = "Failed: " + firstLine(message.err)
			return m, nil
		}
		return m, load(m.repositories())
	case diffLoadedMessage:
		if message.err == nil {
			m.diffCache[message.pullRequest.url] = cachedDiff{message.pullRequest.headCommit, message.files}
		}
		if m.mode != diffMode || message.pullRequest.url != m.diffPullRequest.url {
			return m, nil
		}
		if message.err != nil {
			m.closeDiff()
			m.notice = "Diff failed: " + firstLine(message.err)
			return m, nil
		}
		m.diffFiles, m.diffLoading = message.files, false
	case editorOpenedMessage:
		m.notice = ""
		if message.err != nil {
			m.notice = "Failed: " + firstLine(message.err)
		}
	case refreshTickMessage:
		return m, tea.Batch(load(m.repositories()), scheduleRefresh(m.config.refreshInterval))
	case commentPostedMessage:
		m.commentPosting = false
		if message.err != nil {
			m.commentError = firstLine(message.err)
			return m, nil
		}
		if m.mode == commentMode {
			m.mode = diffMode
		}
		m.diffNotice = "Comment posted on " + message.target.label() + "."
		return m, loadComments(m.diffPullRequest)
	case commentsLoadedMessage:
		if message.url != m.diffPullRequest.url || (m.mode != diffMode && m.mode != commentMode) {
			return m, nil
		}
		if message.err != nil {
			m.diffNotice = "Comments failed to load: " + firstLine(message.err)
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

func (m model) handleKey(key string) (model, tea.Cmd) {
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
		return m, load(m.repositories())
	case "o":
		if hasSelection {
			return m, openInBrowser(pr.url)
		}
	case "enter":
		if hasSelection {
			m.mode = diffMode
			m.diffPullRequest = pr
			if cached, isCached := m.diffCache[pr.url]; isCached && cached.headCommit == pr.headCommit {
				m.diffFiles = cached.files
				return m, loadComments(pr)
			}
			m.diffLoading = true
			return m, tea.Batch(loadDiff(pr), loadComments(pr))
		}
	case "p":
		if hasSelection {
			return m, m.openDescription(pr)
		}
	case "v":
		if hasSelection {
			m.notice = "Opening " + pr.label() + " in " + m.config.editor.name + "…"
			return m, openInEditor(pr, m.config)
		}
	case "d":
		if hasSelection {
			m.notice = pr.toggleDraftNotice()
			return m, runCommands(pr.toggleDraftCommand())
		}
	case "m":
		if hasSelection {
			m.ask(pr.toggleAutoMergeQuestion(), pr.toggleAutoMergeCommand())
		}
	case "e":
		if reviewTeam := m.config.pullRequests.reviewTeam; hasSelection && reviewTeam != "" {
			m.notice = pr.requestTeamReviewNotice(reviewTeam)
			return m, runCommands(pr.requestTeamReviewCommands(reviewTeam)...)
		}
	}
	return m, nil
}

func (m model) handleDiffKey(key string) (model, tea.Cmd) {
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
		return m, openInBrowser(m.diffPullRequest.url)
	case "v":
		return m, openInEditor(m.diffPullRequest, m.config)
	case "p":
		return m, m.openDescription(m.diffPullRequest)
	}
	return m, nil
}

func (m model) handleDescriptionKey(key string) (model, tea.Cmd) {
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
		return m, openInBrowser(m.descriptionPullRequest.url)
	case "v":
		return m, openInEditor(m.descriptionPullRequest, m.config)
	}
	return m, nil
}

func (m model) handleFilterKey(key string) model {
	switch key {
	case "f", "esc", "enter":
		m.mode = boardMode
		return m
	case "b":
		m.filters.showBots = !m.filters.showBots
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '0'+maximumFilterKeys {
			if index := int(key[0] - '1'); index < len(m.filters.available) {
				name := m.filters.available[index].fullName()
				m.filters.repositories[name] = !m.filters.repositories[name]
			}
		}
	}
	m.applyFilters()
	return m
}

func (m model) handleConfirmKey(key string) (model, tea.Cmd) {
	switch key {
	case "y", "enter":
		command := m.pendingCommand
		m.mode, m.question, m.pendingCommand = boardMode, "", nil
		m.notice = "Working…"
		return m, runCommands(command)
	case "n", "esc":
		m.mode, m.question, m.pendingCommand = boardMode, "", nil
	}
	return m, nil
}

func (m model) statusLine() string {
	switch {
	case m.mode == confirmMode:
		return keyStyle.Render(m.question) + "  " + helpLine("y", "yes", "n", "no")
	case m.notice != "":
		return m.notice
	case m.loadedAt.IsZero():
		return dimStyle.Render("Loading…")
	}
	return dimStyle.Render(fmt.Sprintf(
		"%d of %d open pull requests · updated %s · refreshes every %s",
		len(m.visible), len(m.pullRequests), m.loadedAt.Format("15:04"), intervalText(m.config.refreshInterval),
	))
}

// intervalText says a refresh interval in whole minutes where it can, as in "3 minutes".
func intervalText(interval time.Duration) string {
	minutes := int(interval / time.Minute)
	switch {
	case interval == time.Minute:
		return "minute"
	case interval%time.Minute == 0:
		return fmt.Sprintf("%d minutes", minutes)
	}
	return fmt.Sprintf("%d seconds", int(interval.Round(time.Second)/time.Second))
}

func helpLine(keysAndLabels ...string) string {
	var parts []string
	for index := 0; index < len(keysAndLabels); index += 2 {
		parts = append(parts, keyStyle.Render(keysAndLabels[index])+" "+dimStyle.Render(keysAndLabels[index+1]))
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

func (m model) help() string {
	switch m.mode {
	case filterMode:
		return helpLine("esc", "close filters", "q", "quit")
	case confirmMode:
		return ""
	}
	keysAndLabels := []string{"enter", "diff", "p", "description", "o", "open", "v", m.config.editor.name, "d", "draft / ready"}
	if reviewTeam := m.config.pullRequests.reviewTeam; reviewTeam != "" {
		keysAndLabels = append(keysAndLabels, "e", "request "+reviewTeam)
	}
	return helpLine(append(keysAndLabels, "m", "auto-merge", "f", "filters", "r", "refresh", "q", "quit")...)
}

func (m model) diffBody() []string {
	if m.diffLoading {
		return []string{dimStyle.Render("Loading diff…")}
	}
	if len(m.diffFiles) == 0 {
		return []string{dimStyle.Render("No changes.")}
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
				left = selectedStyle.Render(fit(plainText(segments), listWidth))
			} else {
				left = fit(styledText(segments), listWidth)
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

func (m model) commentPanel() []string {
	title := boldStyle.Render("Comment on " + m.commentTarget.label())
	if m.commentTarget.replyTo != 0 {
		title = boldStyle.Render("Reply to " + m.commentTarget.replyAuthor + " on " + m.commentTarget.label())
	}
	switch {
	case m.commentPosting:
		title += dimStyle.Render("  Posting…")
	case m.commentError != "":
		title += "  " + redStyle.Render("Failed: "+m.commentError)
	}
	return []string{
		ansi.Truncate(title, m.width, "…"),
		m.commentInput.View(),
		helpLine("ctrl+s", "post", "esc", "cancel"),
	}
}

func (m model) diffHelp() string {
	position := ""
	if len(m.diffFiles) > 0 {
		position = dimStyle.Render(fmt.Sprintf("file %d of %d · ", m.fileCursor+1, len(m.diffFiles)))
	}
	layout := "split"
	if m.splitDiff {
		layout = "unified"
	}
	return position + helpLine(
		"↑/↓", "file", "j/k", "line", "space/b", "page", "c", "comment", "tab", "old/new side", "s", layout,
		"p", "description", "o", "open", "v", m.config.editor.name, "esc", "back",
	)
}

func (m model) diffView() string {
	pr := m.diffPullRequest
	header := boldStyle.Render(pr.label()) + "  " + pr.title + "  " + styledText(pr.lineCountSegments())
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

func (m model) descriptionView() string {
	pr := m.descriptionPullRequest
	header := ansi.Truncate(boldStyle.Render(pr.label())+"  "+pr.title, m.width, "…")
	body := []string{dimStyle.Render("Loading description…")}
	if !m.descriptionLoading {
		body = m.descriptionLines[min(m.descriptionOffset, len(m.descriptionLines)):]
		body = body[:min(len(body), m.descriptionRows())]
	}
	lines := append([]string{header}, body...)
	for len(lines) < m.descriptionRows()+1 {
		lines = append(lines, "")
	}
	help := helpLine("↑/↓", "scroll", "space/b", "page", "e", "edit", "o", "open", "v", m.config.editor.name, "esc", "back")
	lines = append(lines, ansi.Truncate(m.descriptionNotice, m.width, "…"), ansi.Truncate(help, m.width, "…"))
	return strings.Join(lines, "\n")
}

func (m model) View() string {
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
	table := renderTable(m.visible, m.viewer, m.config.pullRequests.reviewTeam, m.width, m.cursor, m.offset, rows)
	blankLines := rows + 1 - lipgloss.Height(table)
	sections = append(sections, table+strings.Repeat("\n", max(blankLines, 0)))
	sections = append(sections, ansi.Truncate(m.statusLine(), m.width, "…"), ansi.Truncate(m.help(), m.width, "…"))
	return strings.Join(sections, "\n")
}

// resolveRepositories turns each --repo value, either a short name or owner/name, into the
// full name filters use. A short name shared by two owners is ambiguous and must be spelled out.
func resolveRepositories(available []repository, values []string) ([]string, error) {
	var names []string
	for _, value := range values {
		var matches []repository
		for _, source := range available {
			if value == source.fullName() || value == source.name {
				matches = append(matches, source)
			}
		}
		switch len(matches) {
		case 0:
			fullNames := make([]string, len(available))
			for index, source := range available {
				fullNames[index] = source.fullName()
			}
			return nil, fmt.Errorf("unknown repository %q; configured repositories: %s", value, strings.Join(fullNames, ", "))
		case 1:
			names = append(names, matches[0].fullName())
		default:
			return nil, fmt.Errorf("repository name %q matches more than one owner; write it as owner/name", value)
		}
	}
	return names, nil
}

func terminalWidth() int {
	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		return width
	}
	return 120
}

func printOnce(settings configuration, f filters) error {
	pullRequests, viewer, err := fetchPullRequests(settings.pullRequests.repositories)
	if err != nil {
		return err
	}
	m := newModel(settings, f)
	m.pullRequests = pullRequests
	commands := detailCommands(m.repositories())
	messages := make(chan tea.Msg, len(commands))
	for _, command := range commands {
		go func() { messages <- command() }()
	}
	for range commands {
		m, _ = m.update(<-messages)
	}
	visible := m.visible
	fmt.Println(f.summary())
	fmt.Println(renderTable(visible, viewer, settings.pullRequests.reviewTeam, terminalWidth(), -1, 0, len(visible)))
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func main() {
	var selectedRepositories []string
	flag.Func("repo", "start with only this repository selected, as a name or owner/name (repeatable)", func(value string) error {
		selectedRepositories = append(selectedRepositories, value)
		return nil
	})
	showBots := flag.Bool("bots", false, "start on bot-authored pull requests")
	once := flag.Bool("once", false, "print the table and exit")
	configFlag := flag.String("config", "", "path to the configuration file")
	initialize := flag.Bool("init", false, "write a starter configuration file and exit")
	flag.Parse()

	path, err := configPath(*configFlag)
	if err != nil {
		fail(err)
	}
	if *initialize {
		if err := writeStarterConfiguration(path); err != nil {
			fail(err)
		}
		fmt.Println("Wrote", path)
		return
	}
	settings, err := loadConfiguration(path)
	if err != nil {
		fail(err)
	}
	selected, err := resolveRepositories(settings.pullRequests.repositories, selectedRepositories)
	if err != nil {
		fail(err)
	}

	f := newFilters(settings.pullRequests.repositories, selected, *showBots)
	if *once {
		err = printOnce(settings, f)
	} else {
		_, err = tea.NewProgram(newModel(settings, f), tea.WithAltScreen()).Run()
	}
	if err != nil {
		fail(err)
	}
}
