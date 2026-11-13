package pullrequests

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/shell"
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
	// lineCountRequests and ciStateRequests count the calls made for each repository.
	lineCountRequests map[string]int
	ciStateRequests   map[string]int
	comments          []ReviewComment
	changedFiles      []string
	description       string
	diff              string
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

		lineCountRequests: map[string]int{},
		ciStateRequests:   map[string]int{},
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

func (fake *fakeProvider) LineCounts(source Repository) (map[string]LineCount, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.lineCountRequests[source.FullName()]++
	if err := fake.failures["LineCounts"]; err != nil {
		return nil, err
	}
	return fake.lineCounts, nil
}

func (fake *fakeProvider) CIStates(source Repository) (map[string]CIState, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.ciStateRequests[source.FullName()]++
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

const (
	testCloneRoot  = "/clones"
	testEditorName = "Test Editor"
)

var testEditorCommand = []string{"test-editor", "--reuse-window"}

// setup describes how a test builds the board: with which provider, editor, and command-line
// arguments, and next to which other plugins.
type setup struct {
	provider      *fakeProvider
	editorCommand []string
	arguments     []string
	others        []plugin.Plugin
}

func newSetup(provider *fakeProvider) setup {
	return setup{provider: provider, editorCommand: testEditorCommand}
}

func (s setup) withArguments(arguments ...string) setup {
	s.arguments = arguments
	return s
}

func (s setup) withEditorCommand(command ...string) setup {
	s.editorCommand = command
	return s
}

func (s setup) alongside(others ...plugin.Plugin) setup {
	s.others = others
	return s
}

func (s setup) configurationText() string {
	enabled := []string{fmt.Sprintf("%q", pluginName)}
	for _, other := range s.others {
		enabled = append(enabled, fmt.Sprintf("%q", other.Name()))
	}
	var command []string
	for _, word := range s.editorCommand {
		command = append(command, fmt.Sprintf("%q", word))
	}
	return fmt.Sprintf(`
plugins = [%s]
refresh_interval = "5m"

[editor]
name = %q
command = [%s]

[pull_requests]
provider = %q
clone_root = %q
`, strings.Join(enabled, ", "), testEditorName, strings.Join(command, ", "), s.providerName(), testCloneRoot)
}

// providerName is unique to the fake provider, so that one test can build two boards.
func (s setup) providerName() string {
	return fmt.Sprintf("fake-%p", s.provider)
}

// host sets up the pull request plugin, with the command-line arguments parsed the way main
// parses them, and the other plugins after it.
func (s setup) host(t *testing.T) (*plugin.Host, error) {
	t.Helper()
	registerTestProvider(t, s.providerName(), func(SectionDecoder) (Provider, error) { return s.provider, nil })
	pullRequests := &pullRequestsPlugin{}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	pullRequests.Flags(flags)
	if err := flags.Parse(s.arguments); err != nil {
		t.Fatal(err)
	}
	file := configtest.Load(t, s.configurationText())
	return plugin.NewHost("config.toml", file, append([]plugin.Plugin{pullRequests}, s.others...))
}

// newHostFromText sets up the plugins with a configuration file of the given text.
func newHostFromText(t *testing.T, text string, plugins ...plugin.Plugin) (*plugin.Host, error) {
	t.Helper()
	return plugin.NewHost("config.toml", configtest.Load(t, text), plugins)
}

// started builds the board and loads it, including the slow columns.
func (s setup) started(t *testing.T) *plugin.Model {
	t.Helper()
	host, err := s.host(t)
	if err != nil {
		t.Fatal(err)
	}
	model := host.Model()
	plugintest.Start(t, model)
	return model
}

func loadedBoardWith(t *testing.T, provider *fakeProvider) *plugin.Model {
	t.Helper()
	return newSetup(provider).started(t)
}

func loadedBoard(t *testing.T) *plugin.Model {
	t.Helper()
	return loadedBoardWith(t, newFakeProvider())
}

func keyMessage(key string) tea.KeyMsg {
	return plugintest.KeyMessage(key)
}

func press(t *testing.T, model *plugin.Model, keys ...string) {
	t.Helper()
	plugintest.Press(t, model, keys...)
}

// view is what the model shows, without styling.
func view(model *plugin.Model) string {
	return ansi.Strip(model.View())
}

// helpLine is the last line of the view, which is where the board lists its keys.
func helpLine(model *plugin.Model) string {
	lines := strings.Split(view(model), "\n")
	return lines[len(lines)-1]
}

// lineWith is the line of the view that contains the text.
func lineWith(t *testing.T, model *plugin.Model, text string) string {
	t.Helper()
	for _, line := range strings.Split(view(model), "\n") {
		if strings.Contains(line, text) {
			return line
		}
	}
	t.Fatalf("no line contains %q:\n%s", text, view(model))
	return ""
}

// assertShown checks which of the provider's pull requests the board lists, by their titles.
func assertShown(t *testing.T, model *plugin.Model, provider *fakeProvider, shown ...PullRequest) {
	t.Helper()
	for _, pr := range provider.pullRequests {
		listed := strings.Contains(view(model), pr.Title)
		if expected := slices.ContainsFunc(shown, func(other PullRequest) bool { return other.URL == pr.URL }); listed != expected {
			t.Fatalf("%q listed = %v, expected %v:\n%s", pr.Title, listed, expected, view(model))
		}
	}
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

// editorWrites replaces the editor with one that saves the given text. Temporary files go to a
// directory that the test removes, and that directory is returned.
func editorWrites(t *testing.T, text string) string {
	t.Helper()
	temporaryDirectory := t.TempDir()
	t.Setenv("TMPDIR", temporaryDirectory)
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
	return temporaryDirectory
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
