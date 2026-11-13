package pullrequests

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/config"
	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// testReviewTeam is the team slug that the fixture's third pull request requests a review from.
const testReviewTeam = "engineering"

const testViewer = "viewer"

const fixtureDiff = `diff --git a/first.py b/first.py
index 111..222 100644
--- a/first.py
+++ b/first.py
@@ -1,3 +1,4 @@
 unchanged
-removed
+added
+added again
diff --git a/old_name.py b/second/new_name.py
similarity index 90%
rename from old_name.py
rename to second/new_name.py
--- a/old_name.py
+++ b/second/new_name.py
@@ -1 +1 @@
--- looks like a header
+++ looks like a header
`

var (
	gazeboRepository         = Repository{Owner: "CoProcure", Name: "gazebo"}
	secondRepository         = Repository{Owner: "CoProcure", Name: "u2"}
	analyticsRepository      = Repository{Owner: "CoProcure", Name: "analytics"}
	infrastructureRepository = Repository{Owner: "CoProcure", Name: "infrastructure"}
)

// recordedCall is one change the board asked the fake provider to make.
type recordedCall struct {
	method string
	url    string
	flag   bool
	body   string
	target CommentTarget
}

func draftCall(pr PullRequest, draft bool) recordedCall {
	return recordedCall{method: "SetDraft", url: pr.URL, flag: draft}
}

func autoMergeCall(pr PullRequest, enabled bool) recordedCall {
	return recordedCall{method: "SetAutoMerge", url: pr.URL, flag: enabled}
}

func teamReviewCall(pr PullRequest) recordedCall {
	return recordedCall{method: "RequestTeamReview", url: pr.URL}
}

func saveDescriptionCall(pr PullRequest, body string) recordedCall {
	return recordedCall{method: "SaveDescription", url: pr.URL, body: body}
}

func postCommentCall(pr PullRequest, target CommentTarget, body string) recordedCall {
	return recordedCall{method: "PostComment", url: pr.URL, target: target, body: body}
}

// fakeProvider serves fixture data and records the changes it is asked to make. The board
// calls it from several goroutines at once, so every method takes the mutex.
type fakeProvider struct {
	mutex        sync.Mutex
	repositories []Repository
	reviewTeam   string
	pullRequests []PullRequest
	lineCounts   map[string]LineCount
	ciStates     map[string]CIState
	comments     []ReviewComment
	changedFiles []string
	description  string
	diff         string
	// failures maps a method name to the error it returns.
	failures    map[string]error
	calls       []recordedCall
	diffFetches int
}

// newFakeProvider has three pull requests, newest first: a draft from a person and a bot's
// pull request in the first repository, and a ready pull request with reviewers and
// auto-merge in the second.
func newFakeProvider() *fakeProvider {
	updated := func(hour int) time.Time { return time.Date(2026, 11, 10, hour, 0, 0, 0, time.UTC) }
	pullRequests := []PullRequest{
		{
			Repository: gazeboRepository, Number: 1, Title: "Draft from a person", URL: "https://example.test/gazebo/1",
			Author: "person", IsDraft: true, HeadCommit: "first-commit", UpdatedAt: updated(12),
		},
		{
			Repository: gazeboRepository, Number: 2, Title: "Dependency update", URL: "https://example.test/gazebo/2",
			Author: "renovate", IsBot: true, UpdatedAt: updated(11),
		},
		{
			Repository: secondRepository, Number: 3, Title: "Ready with reviewers", URL: "https://example.test/u2/3",
			Author: "person", AutoMerge: true, UpdatedAt: updated(10),
			RequestedUsers: []string{testViewer, "someone-else"}, RequestedTeams: []string{testReviewTeam},
		},
	}
	return &fakeProvider{
		repositories: []Repository{gazeboRepository, secondRepository, analyticsRepository, infrastructureRepository},
		reviewTeam:   testReviewTeam,
		pullRequests: pullRequests,
		lineCounts:   map[string]LineCount{pullRequests[0].URL: {Additions: 12, Deletions: 3}},
		ciStates:     map[string]CIState{pullRequests[0].URL: CIStateFailing, pullRequests[2].URL: CIStateNone},
		comments: []ReviewComment{
			{ID: 10, Path: "first.py", Line: 2, Side: CommentSideNew, Author: "reviewer", Body: "Why this change?", CreatedAt: updated(9)},
			{ID: 11, InReplyTo: 10, Path: "first.py", Line: 2, Side: CommentSideNew, Author: "person", Body: "Because it was wrong.", CreatedAt: updated(9).Add(30 * time.Minute)},
			{ID: 12, Path: "first.py", Side: CommentSideNew, Author: "reviewer", Body: "Outdated remark", CreatedAt: updated(9).Add(-24 * time.Hour)},
		},
		changedFiles: []string{"kept.py", "tests/added.py"},
		description:  "Original description\n\nSecond paragraph",
		diff:         fixtureDiff,
		failures:     map[string]error{},
	}
}

func (fake *fakeProvider) pullRequest(number int) PullRequest {
	for _, pr := range fake.pullRequests {
		if pr.Number == number {
			return pr
		}
	}
	panic(fmt.Sprintf("the fixture has no pull request %d", number))
}

func (fake *fakeProvider) fail(method string, err error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.failures[method] = err
}

func (fake *fakeProvider) recorded() []recordedCall {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return slices.Clone(fake.calls)
}

func (fake *fakeProvider) fetchesOfTheDiff() int {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.diffFetches
}

// change records a call, and returns the failure set for the method, if any.
func (fake *fakeProvider) change(method string, call recordedCall) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	call.method = method
	fake.calls = append(fake.calls, call)
	return fake.failures[method]
}

func (fake *fakeProvider) Repositories() []Repository { return fake.repositories }

func (fake *fakeProvider) ReviewTeam() string { return fake.reviewTeam }

func (fake *fakeProvider) PullRequests() ([]PullRequest, string, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if err := fake.failures["PullRequests"]; err != nil {
		return nil, "", err
	}
	return slices.Clone(fake.pullRequests), testViewer, nil
}

func (fake *fakeProvider) LineCounts(Repository) (map[string]LineCount, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if err := fake.failures["LineCounts"]; err != nil {
		return nil, err
	}
	return fake.lineCounts, nil
}

func (fake *fakeProvider) CIStates(Repository) (map[string]CIState, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if err := fake.failures["CIStates"]; err != nil {
		return nil, err
	}
	return fake.ciStates, nil
}

func (fake *fakeProvider) Diff(PullRequest) (string, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.diffFetches++
	return fake.diff, fake.failures["Diff"]
}

func (fake *fakeProvider) ChangedFiles(PullRequest) ([]string, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if err := fake.failures["ChangedFiles"]; err != nil {
		return nil, err
	}
	return fake.changedFiles, nil
}

func (fake *fakeProvider) Description(PullRequest) (string, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.description, fake.failures["Description"]
}

func (fake *fakeProvider) SaveDescription(pr PullRequest, body string) error {
	return fake.change("SaveDescription", recordedCall{url: pr.URL, body: body})
}

func (fake *fakeProvider) Comments(PullRequest) ([]ReviewComment, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if err := fake.failures["Comments"]; err != nil {
		return nil, err
	}
	return fake.comments, nil
}

func (fake *fakeProvider) PostComment(pr PullRequest, target CommentTarget, body string) error {
	return fake.change("PostComment", recordedCall{url: pr.URL, target: target, body: body})
}

func (fake *fakeProvider) SetDraft(pr PullRequest, draft bool) error {
	return fake.change("SetDraft", recordedCall{url: pr.URL, flag: draft})
}

func (fake *fakeProvider) SetAutoMerge(pr PullRequest, enabled bool) error {
	return fake.change("SetAutoMerge", recordedCall{url: pr.URL, flag: enabled})
}

func (fake *fakeProvider) RequestTeamReview(pr PullRequest) error {
	return fake.change("RequestTeamReview", recordedCall{url: pr.URL})
}

func testSettings() Settings {
	return Settings{
		ProviderName:    "fake",
		CloneRoot:       "/clones",
		RefreshInterval: 5 * time.Minute,
		Editor:          config.Editor{Name: "Test Editor", Command: []string{"test-editor", "--reuse-window"}},
	}
}

func boardFor(provider Provider, settings Settings) Model {
	return newModel(settings, provider, newFilters(provider.Repositories(), nil, false))
}

func loadedBoardFor(t *testing.T, provider Provider, settings Settings) Model {
	t.Helper()
	board := boardFor(provider, settings)
	return settle(t, board, board.load())
}

func loadedBoardWith(t *testing.T, provider Provider) Model {
	t.Helper()
	return loadedBoardFor(t, provider, testSettings())
}

func loadedBoard(t *testing.T) Model {
	t.Helper()
	return loadedBoardWith(t, newFakeProvider())
}

// recordShell replaces shell.Run so that no test opens the browser or an editor, and
// returns the commands it was asked to run.
func recordShell(t *testing.T) *[][]string {
	t.Helper()
	recorded := &[][]string{}
	original := shell.Run
	shell.Run = func(arguments ...string) ([]byte, error) {
		*recorded = append(*recorded, arguments)
		return nil, nil
	}
	t.Cleanup(func() { shell.Run = original })
	return recorded
}

func settle(t *testing.T, m Model, command tea.Cmd) Model {
	t.Helper()
	if command == nil {
		return m
	}
	message := command()
	if batch, isBatch := message.(tea.BatchMsg); isBatch {
		for _, batched := range batch {
			m = settle(t, m, batched)
		}
		return m
	}
	if message == nil {
		return m
	}
	next, nextCommand := m.Update(message)
	return settle(t, next.(Model), nextCommand)
}

func keyMessage(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, key := range keys {
		next, command := m.Update(keyMessage(key))
		m = settle(t, next.(Model), command)
	}
	return m
}

func visibleLabels(m Model) []string {
	labels := []string{}
	for _, pr := range m.visible {
		labels = append(labels, pr.label())
	}
	return labels
}

// assertShown checks which pull requests the board lists, in order.
func assertShown(t *testing.T, m Model, expected ...PullRequest) {
	t.Helper()
	expectedLabels := []string{}
	for _, pr := range expected {
		expectedLabels = append(expectedLabels, pr.label())
	}
	if labels := visibleLabels(m); !reflect.DeepEqual(labels, expectedLabels) {
		t.Fatalf("visible pull requests = %v, expected %v", labels, expectedLabels)
	}
}

func assertCalls(t *testing.T, provider *fakeProvider, expected ...recordedCall) {
	t.Helper()
	calls := provider.recorded()
	if len(calls) == 0 && len(expected) == 0 {
		return
	}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("provider calls = %+v, expected %+v", calls, expected)
	}
}

func assertCommands(t *testing.T, recorded *[][]string, expected ...[]string) {
	t.Helper()
	if len(*recorded) == 0 && len(expected) == 0 {
		return
	}
	if !reflect.DeepEqual(*recorded, expected) {
		t.Fatalf("commands = %v, expected %v", *recorded, expected)
	}
}

// editorWrites replaces the editor with one that saves the given text.
func editorWrites(t *testing.T, text string) {
	t.Helper()
	original := editInEditor
	editInEditor = func(path string, finished func(error) tea.Msg) tea.Cmd {
		return func() tea.Msg {
			if text != "" {
				os.WriteFile(path, []byte(text), 0o600)
			}
			return finished(nil)
		}
	}
	t.Cleanup(func() { editInEditor = original })
}

// repositoriesNamed names repositories repository0, repository1, and so on.
func repositoriesNamed(count int) []Repository {
	var repositories []Repository
	for index := range count {
		repositories = append(repositories, Repository{Owner: "CoProcure", Name: fmt.Sprintf("repository%d", index)})
	}
	return repositories
}

func providerWithRepositories(repositories ...Repository) *fakeProvider {
	provider := newFakeProvider()
	provider.repositories = repositories
	return provider
}

var errorForTests = errors.New("cannot do that")

// failShell replaces shell.Run with one that always fails.
func failShell(t *testing.T, err error) {
	t.Helper()
	original := shell.Run
	shell.Run = func(...string) ([]byte, error) { return nil, err }
	t.Cleanup(func() { shell.Run = original })
}

// linesColumn is the plain text of the Lines cell of the board's row at the given index.
func linesColumn(m Model, row int) string {
	return ui.PlainText(m.visible[row].columns(m.viewer, m.reviewTeam())[4])
}

func filePlainText(file diffFile) string {
	return ui.PlainText(file.listSegments(80))
}
