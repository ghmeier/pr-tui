package agentsessions

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
	"github.com/ghmeier/pr-tui/plugins/pullrequests/pullrequeststest"
)

// fakeProvider serves a fixed list of sessions and records what it is asked to send.
type fakeProvider struct {
	mutex    sync.Mutex
	sessions []Session
	sent     []sentMessage
	sendErr  error
	settings ProviderSettings
}

type sentMessage struct {
	session Session
	text    string
}

func (provider *fakeProvider) Sessions() ([]Session, error) {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()
	return append([]Session(nil), provider.sessions...), nil
}

func (provider *fakeProvider) Send(session Session, message string) error {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()
	provider.sent = append(provider.sent, sentMessage{session: session, text: message})
	return provider.sendErr
}

// registerTestProvider makes a provider selectable under the name for the length of the test.
func registerTestProvider(t *testing.T, name string, factory ProviderFactory) {
	t.Helper()
	RegisterProvider(name, factory)
	t.Cleanup(func() { delete(providerFactories, name) })
}

// registerFake makes the provider selectable as "fake" and keeps the settings it was built with.
func registerFake(t *testing.T, provider *fakeProvider) {
	t.Helper()
	registerTestProvider(t, "fake", func(settings ProviderSettings) (Provider, error) {
		provider.settings = settings
		return provider, nil
	})
}

// boardFixture is a repository named storefront with two open pull requests, and the sessions
// that tests say work on them.
type boardFixture struct {
	source pullrequests.Repository
	// signIn is on the branch feature/sign-in.
	signIn pullrequests.PullRequest
	// tidy is on a branch that no session is on.
	tidy pullrequests.PullRequest
}

func newBoardFixture() boardFixture {
	source := pullrequests.Repository{Owner: "acme", Name: "storefront"}
	updated := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return boardFixture{
		source: source,
		signIn: pullrequests.PullRequest{
			Repository: source, Number: 1, Title: "Add the sign-in form", URL: "https://example.test/pull/1", Author: "alice", HeadBranch: "feature/sign-in", UpdatedAt: updated,
		},
		tidy: pullrequests.PullRequest{
			Repository: source, Number: 2, Title: "Tidy the build scripts", URL: "https://example.test/pull/2", Author: "bob", HeadBranch: "chore/tidy", UpdatedAt: updated.Add(-time.Hour),
		},
	}
}

// mentioning is a session on the main branch that has mentioned the pull request's address.
func (fixture boardFixture) mentioning(name string, pullRequest pullrequests.PullRequest) Session {
	return Session{
		ID: "id-" + name, Name: name, Repository: fixture.source.Name, Branch: "main", Status: StatusIdle,
		LastActive:   time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC),
		PullRequests: []PullRequestReference{{Repository: fixture.source.Owner + "/" + fixture.source.Name, Number: pullRequest.Number}},
	}
}

// onBranch is a session that has mentioned nothing and works on the branch in the repository.
func onBranch(name, repository, branch string) Session {
	return Session{ID: "id-" + name, Name: name, Repository: repository, Branch: branch, Status: StatusIdle, LastActive: time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)}
}

// start loads the pull request board with the column and the action, over the sessions.
func (fixture boardFixture) start(t *testing.T, provider *fakeProvider) *plugin.Model {
	t.Helper()
	return fixture.startWith(t, provider)
}

// startWith is start with more plugins enabled after the others.
func (fixture boardFixture) startWith(t *testing.T, provider *fakeProvider, others ...plugin.Plugin) *plugin.Model {
	t.Helper()
	registerFake(t, provider)
	pullRequestProvider := &pullrequeststest.Provider{Source: fixture.source, Open: []pullrequests.PullRequest{fixture.signIn, fixture.tidy}}
	enabled := []string{fmt.Sprintf("%q", pullrequests.BoardName), fmt.Sprintf("%q", pluginName)}
	for _, other := range others {
		enabled = append(enabled, fmt.Sprintf("%q", other.Name()))
	}
	text := fmt.Sprintf("plugins = [%s]\n[pull_requests]\nprovider = %q\n[agent_sessions]\nprovider = \"fake\"\n",
		strings.Join(enabled, ", "), pullrequeststest.Register(t, pullRequestProvider))
	host, err := plugin.NewHost("config.toml", configtest.Load(t, text), append(plugin.Registered(), others...))
	if err != nil {
		t.Fatal(err)
	}
	model := host.Model()
	plugintest.Start(t, model)
	plugintest.Resize(t, model, 200, 30)
	return model
}

func screenLines(model *plugin.Model) []string {
	return strings.Split(ansi.Strip(model.View()), "\n")
}

// lineWith returns the screen line that contains the text, or "".
func lineWith(model *plugin.Model, text string) string {
	for _, line := range screenLines(model) {
		if strings.Contains(line, text) {
			return line
		}
	}
	return ""
}

func statusLine(model *plugin.Model) string {
	lines := screenLines(model)
	return lines[len(lines)-2]
}

// editorSession records what the replaced editor was opened with.
type editorSession struct {
	opened []string
}

// editorWrites replaces the editor with one that saves the text, or leaves the file as it was
// when the text is empty.
func editorWrites(t *testing.T, text string) *editorSession {
	t.Helper()
	session := &editorSession{}
	original := plugin.RunEditor
	t.Cleanup(func() { plugin.RunEditor = original })
	plugin.RunEditor = func(path string, finished func(error) tea.Msg) tea.Cmd {
		return func() tea.Msg {
			opened, _ := os.ReadFile(path)
			session.opened = append(session.opened, string(opened))
			if text != "" {
				os.WriteFile(path, []byte(text), 0o600)
			}
			return finished(nil)
		}
	}
	return session
}
