package pullrequests

import (
	"flag"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// ticketPlugin stands in for a plugin of another team, such as one for a story tracker. It
// adds a Ticket column and a key to the pull request board, and has a board of its own.
type ticketPlugin struct {
	// ticketsByURL is what the column's fetch delivers, keyed like the board's rows.
	ticketsByURL map[string]string
	// linked is the pull requests that the action was run on.
	linked []PullRequest
	// seenOnAction is what the action read from the Ticket column of its selection.
	seenOnAction any
}

const (
	ticketColumnHeader = "Ticket"
	ticketBoardName    = "tickets"
	ticketBoardTitle   = "Tickets"
)

func (*ticketPlugin) Name() string { return "tickets" }

func (*ticketPlugin) Flags(*flag.FlagSet) {}

// ticketRow is a row of the plugin's own board.
type ticketRow struct{ id string }

func (row ticketRow) Key() string { return row.id }

func (tickets *ticketPlugin) Setup(host *plugin.Host) error {
	host.AddColumn(BoardName, plugin.Column{
		Header: ticketColumnHeader,
		Cell: func(_ plugin.Row, data any) []ui.Segment {
			if data == nil {
				return textCell("no ticket")
			}
			return textCell(data.(string))
		},
		Load: func([]plugin.Row) []plugin.Fetch {
			return []plugin.Fetch{func() (map[string]any, error) {
				found := map[string]any{}
				for url, ticket := range tickets.ticketsByURL {
					found[url] = ticket
				}
				return found, nil
			}}
		},
	})
	host.AddAction(BoardName, plugin.Action{Key: "t", Label: "link ticket", Run: func(selection plugin.Selection) plugin.Effect {
		tickets.linked = append(tickets.linked, selection.Row.(Row).PullRequest)
		tickets.seenOnAction = selection.Data(ticketColumnHeader)
		return plugin.Effect{}
	}})
	host.AddBoard(plugin.Board{
		Name:    ticketBoardName,
		Title:   ticketBoardTitle,
		RowNoun: "tickets",
		Load:    func() ([]plugin.Row, error) { return []plugin.Row{ticketRow{id: "SC-101"}}, nil },
		Columns: []plugin.Column{{Header: "Id", Cell: func(row plugin.Row, _ any) []ui.Segment {
			return textCell(row.(ticketRow).id)
		}}},
	})
	return nil
}

func ticketsFor(provider *fakeProvider) *ticketPlugin {
	return &ticketPlugin{ticketsByURL: map[string]string{
		provider.pullRequest(1).URL: "SC-101",
		provider.pullRequest(3).URL: "SC-303",
	}}
}

func composedBoard(t *testing.T, provider *fakeProvider, tickets *ticketPlugin) *plugin.Model {
	t.Helper()
	model := newSetup(provider).alongside(tickets).started(t)
	plugintest.Resize(t, model, 200, 30)
	return model
}

func TestAnotherPluginsColumnAppearsInThePullRequestTableWithItsFetchedValues(t *testing.T) {
	provider := newFakeProvider()
	tickets := ticketsFor(provider)

	model := composedBoard(t, provider, tickets)

	header := lineWith(t, model, "Review requested")
	if !strings.Contains(header, ticketColumnHeader) || strings.Index(header, ticketColumnHeader) < strings.Index(header, "Updated") {
		t.Fatalf("header = %q, expected the ticket column after the board's own", header)
	}
	for _, number := range []int{1, 3} {
		pr := provider.pullRequest(number)
		if line := lineWith(t, model, pr.Title); !strings.Contains(line, tickets.ticketsByURL[pr.URL]) {
			t.Fatalf("line %q should show the ticket %s", line, tickets.ticketsByURL[pr.URL])
		}
	}
}

func TestAnotherPluginsColumnShowsItsPlaceholderForAPullRequestItsFetchDidNotCover(t *testing.T) {
	provider := newFakeProvider()
	tickets := ticketsFor(provider)
	delete(tickets.ticketsByURL, provider.pullRequest(3).URL)

	model := composedBoard(t, provider, tickets)

	if line := lineWith(t, model, provider.pullRequest(3).Title); !strings.Contains(line, "no ticket") {
		t.Fatalf("line = %q", line)
	}
}

func TestAnotherPluginsActionRunsOnTheSelectedPullRequest(t *testing.T) {
	provider := newFakeProvider()
	tickets := ticketsFor(provider)
	model := composedBoard(t, provider, tickets)

	press(t, model, "down", "t")

	if len(tickets.linked) != 1 || tickets.linked[0].URL != provider.pullRequest(3).URL {
		t.Fatalf("the action ran on %v", tickets.linked)
	}
	if tickets.seenOnAction != tickets.ticketsByURL[provider.pullRequest(3).URL] {
		t.Fatalf("the action read %v from its own column", tickets.seenOnAction)
	}
	if !strings.Contains(helpLine(model), "t link ticket") {
		t.Fatalf("help = %q", helpLine(model))
	}
}

func TestAnotherPluginsActionDoesNotDisturbTheBoardsOwnKeys(t *testing.T) {
	provider := newFakeProvider()
	model := composedBoard(t, provider, ticketsFor(provider))

	press(t, model, "d")

	assertCalls(t, provider, draftCall(provider.pullRequest(1), false))
}

func TestTabBarListsThePullRequestBoardAndTheOtherPluginsBoard(t *testing.T) {
	provider := newFakeProvider()
	model := composedBoard(t, provider, ticketsFor(provider))

	tabs := strings.Split(view(model), "\n")[0]

	if !strings.Contains(tabs, "Pull requests") || !strings.Contains(tabs, ticketBoardTitle) {
		t.Fatalf("first line = %q", tabs)
	}
}

func TestTabSwitchesToTheOtherPluginsBoardAndBack(t *testing.T) {
	provider := newFakeProvider()
	model := composedBoard(t, provider, ticketsFor(provider))

	press(t, model, "tab")
	onTickets := view(model)
	press(t, model, "tab")

	if !strings.Contains(onTickets, "SC-101") || strings.Contains(onTickets, provider.pullRequest(1).Title) {
		t.Fatalf("the tickets board:\n%s", onTickets)
	}
	if !strings.Contains(view(model), provider.pullRequest(1).Title) {
		t.Fatalf("back on the pull requests:\n%s", view(model))
	}
}

func TestPullRequestBoardAloneHasNoTabBar(t *testing.T) {
	model := loadedBoard(t)

	if first := strings.Split(view(model), "\n")[0]; strings.Contains(first, "Pull requests") {
		t.Fatalf("first line = %q", first)
	}
}

func TestOnlyPluginsTheFileEnablesAreSetUp(t *testing.T) {
	provider := newFakeProvider()
	tickets := ticketsFor(provider)
	model := newSetup(provider).started(t)

	press(t, model, "t")

	if len(tickets.linked) != 0 || strings.Contains(view(model), ticketColumnHeader) {
		t.Fatalf("a plugin that is not enabled took part:\n%s", view(model))
	}
}
