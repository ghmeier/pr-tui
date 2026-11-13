package tickets

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
	"github.com/ghmeier/pr-tui/plugins/pullrequests/pullrequeststest"
)

// fixture is two pull requests, newest first: one whose head branch names the first example
// ticket, and one whose branch names no ticket.
type fixture struct {
	named     pullrequests.PullRequest
	unrelated pullrequests.PullRequest
	ticket    ticket
}

func newFixture() fixture {
	source := pullrequests.Repository{Owner: "acme", Name: "storefront"}
	ticket := exampleTickets()[0]
	updated := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return fixture{
		ticket: ticket,
		named: pullrequests.PullRequest{
			Repository: source, Number: 1, Title: "Add the sign-in form", URL: "https://example.test/storefront/1",
			Author: "alice", HeadBranch: "feature/" + strings.ToLower(ticket.ID) + "-sign-in", UpdatedAt: updated,
		},
		unrelated: pullrequests.PullRequest{
			Repository: source, Number: 2, Title: "Tidy the build scripts", URL: "https://example.test/storefront/2",
			Author: "bob", HeadBranch: "chore/tidy", UpdatedAt: updated.Add(-time.Hour),
		},
	}
}

func (example fixture) provider() *pullrequeststest.Provider {
	return &pullrequeststest.Provider{
		Source: example.named.Repository,
		Open:   []pullrequests.PullRequest{example.named, example.unrelated},
	}
}

// startedWith builds the host from a configuration file that enables the named plugins, and
// loads every board. The configuration is the plugins list, and the pull request provider when
// that plugin is enabled.
func startedWith(t *testing.T, example fixture, enabled []string) *plugin.Model {
	t.Helper()
	quoted := make([]string, len(enabled))
	for index, name := range enabled {
		quoted[index] = fmt.Sprintf("%q", name)
	}
	text := fmt.Sprintf("plugins = [%s]\n", strings.Join(quoted, ", "))
	for _, name := range enabled {
		if name == pullrequests.BoardName {
			text += fmt.Sprintf("[pull_requests]\nprovider = %q\n", pullrequeststest.Register(t, example.provider()))
		}
	}
	host, err := plugin.NewHost("config.toml", configtest.Load(t, text), plugin.Registered())
	if err != nil {
		t.Fatal(err)
	}
	model := host.Model()
	plugintest.Start(t, model)
	plugintest.Resize(t, model, 200, 30)
	return model
}

func startedWithBoth(t *testing.T, example fixture) *plugin.Model {
	t.Helper()
	return startedWith(t, example, []string{pullrequests.BoardName, "tickets"})
}

func view(model *plugin.Model) string { return ansi.Strip(model.View()) }

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

// recordShell replaces shell.Run so that no test opens a browser, and returns the commands it
// was asked to run.
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

func TestTicketColumnShowsTheTicketNamedByThePullRequestsHeadBranch(t *testing.T) {
	example := newFixture()

	model := startedWithBoth(t, example)

	if header := lineWith(t, model, "Review requested"); !strings.Contains(header, ticketColumnHeader) {
		t.Fatalf("header = %q", header)
	}
	if line := lineWith(t, model, example.named.Title); !strings.Contains(line, example.ticket.ID) {
		t.Fatalf("line = %q, expected the ticket %s", line, example.ticket.ID)
	}
}

func TestTicketColumnIsEmptyForAPullRequestWhoseBranchNamesNoTicket(t *testing.T) {
	example := newFixture()

	model := startedWithBoth(t, example)

	line := lineWith(t, model, example.unrelated.Title)
	for _, known := range exampleTickets() {
		if strings.Contains(line, known.ID) {
			t.Fatalf("line = %q, expected no ticket", line)
		}
	}
}

func TestTicketsBoardIsASecondTabWithItsOwnRows(t *testing.T) {
	example := newFixture()
	model := startedWithBoth(t, example)

	plugintest.Press(t, model, "tab")

	for _, known := range exampleTickets() {
		if !strings.Contains(view(model), known.Title) {
			t.Fatalf("the tickets board does not list %q:\n%s", known.Title, view(model))
		}
	}
	if strings.Contains(view(model), example.named.Title) {
		t.Fatalf("the tickets board lists a pull request:\n%s", view(model))
	}
}

func TestStatusFilterHidesTheTicketsOfADeselectedStatus(t *testing.T) {
	example := newFixture()
	model := startedWithBoth(t, example)
	plugintest.Press(t, model, "tab")
	done := exampleTickets()[2]

	plugintest.Press(t, model, "f", "3", "esc")

	if strings.Contains(view(model), done.Title) || !strings.Contains(view(model), example.ticket.Title) {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestTicketKeyOnAPullRequestOpensTheMatchedTicketsAddress(t *testing.T) {
	example := newFixture()
	commands := recordShell(t)
	model := startedWithBoth(t, example)

	plugintest.Press(t, model, "t")

	expected := [][]string{{"open", defaultURLPrefix + example.ticket.ID}}
	if !reflect.DeepEqual(*commands, expected) {
		t.Fatalf("commands = %v, expected %v", *commands, expected)
	}
}

func TestTicketKeyOnAPullRequestWithoutATicketRunsNothingAndSaysSo(t *testing.T) {
	example := newFixture()
	commands := recordShell(t)
	model := startedWithBoth(t, example)

	plugintest.Press(t, model, "down", "t")

	if len(*commands) != 0 {
		t.Fatalf("commands = %v", *commands)
	}
	if !strings.Contains(view(model), "No ticket matches this branch") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestOpenKeyOnTheTicketsBoardOpensTheSelectedTicket(t *testing.T) {
	example := newFixture()
	commands := recordShell(t)
	model := startedWithBoth(t, example)

	plugintest.Press(t, model, "tab", "down", "o")

	second := exampleTickets()[1]
	expected := [][]string{{"open", defaultURLPrefix + second.ID}}
	if !reflect.DeepEqual(*commands, expected) {
		t.Fatalf("commands = %v, expected %v", *commands, expected)
	}
}

func TestPluginStillStartsWithOnlyItsOwnBoardWhenPullRequestsIsDisabled(t *testing.T) {
	example := newFixture()

	model := startedWith(t, example, []string{"tickets"})

	if !strings.Contains(view(model), example.ticket.Title) {
		t.Fatalf("view:\n%s", view(model))
	}
	if strings.Contains(view(model), "Pull requests") || strings.Contains(view(model), example.named.Title) {
		t.Fatalf("the pull request board is present:\n%s", view(model))
	}
}
