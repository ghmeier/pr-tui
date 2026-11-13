# Writing a plugin

This guide is for someone who knows Go but has not seen this code. It builds a small plugin
called `tickets` from nothing, then covers screens, providers, and tests. The finished plugin
is in `plugins/tickets`, and every code block below is that code, so you can read it
in the repository, compile it, and run its tests. Each block says which file it comes from.

## The model

A plugin is a Go package that is compiled into the program. It registers itself from `init`,
and it runs only when its name is in the `plugins` list of the configuration file. When the
program starts, the host calls each enabled plugin's `Setup` once. In `Setup` the plugin reads
its own table of the configuration file and registers contributions: boards (tabs), and
columns and actions that belong to a board. A contribution may target a board that another
plugin owns, which is how the example below adds a Ticket column to the pull request board
without the pull request plugin knowing it exists. The host owns everything generic about a
board: the table, the cursor, the filter menu, refreshing, the status line, confirmation
questions, and tabs.

`CLAUDE.md` lists the contribution types and the boundaries between packages. Read it first if
you have not.

## A worked example

The `tickets` plugin works on made-up data held in memory, so it needs no service and no
network. It does four things:

- reads its own `[tickets]` table of the configuration file,
- registers a board of tickets with a facet and an action,
- adds a Ticket column and a key to the `pull_requests` board, matching each pull request to a
  ticket by the ticket id in its head branch name,
- is switched on in `main.go` and in the configuration file.

### The plugin and its setup

`plugins/tickets/plugin.go`:

```go
// Package tickets is the example plugin from docs/writing-a-plugin.md. It has a made-up,
// in-memory list of tickets, a board for them, and a Ticket column and key on the pull request
// board. It is registered when imported, and only runs when "tickets" is in the plugins list
// of the configuration file.
package tickets

import (
	"flag"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

func init() {
	plugin.Register(&ticketsPlugin{})
}

type ticketsPlugin struct{}

func (*ticketsPlugin) Name() string { return "tickets" }

// Flags has nothing to register, because this plugin takes no command-line flags.
func (*ticketsPlugin) Flags(*flag.FlagSet) {}

func (*ticketsPlugin) Setup(host *plugin.Host) error {
	settings, err := loadSettings(host.Section)
	if err != nil {
		return err
	}
	tracker := newTracker(settings)

	host.AddBoard(tracker.board())
	if host.Enabled(pullrequests.BoardName) {
		host.AddColumn(pullrequests.BoardName, tracker.pullRequestColumn())
		host.AddAction(pullrequests.BoardName, tracker.openTicketOfPullRequest())
	}
	return nil
}
```

`Name` is also the name of the plugin's configuration table, so this plugin owns `[tickets]`.
`Flags` is called for every registered plugin, enabled or not, because command-line flags are
parsed before the configuration file is read; this plugin has none.

The guard `host.Enabled(pullrequests.BoardName)` is what lets the plugin start when
`pull_requests` is not in the `plugins` list. Without it, the host would report that the plugin
adds a column to a board that no enabled plugin registers. `Enabled` does not depend on the
order of the `plugins` list, and the board being targeted may be set up before or after this
plugin. The host attaches columns and actions to boards after every plugin has run `Setup`.

Importing `plugins/pullrequests` here is allowed because the plugin needs the pull request
plugin's exported types: `pullrequests.BoardName` and `pullrequests.Row`.

### Reading configuration

`plugins/tickets/settings.go`:

```go
package tickets

import (
	"cmp"
	"errors"
	"strings"
)

const defaultURLPrefix = "https://tickets.example.com/browse/"

// settings is the [tickets] table of the configuration file.
type settings struct {
	// URLPrefix is put in front of a ticket's id to make the address that opens it.
	URLPrefix string `toml:"url_prefix"`
}

// loadSettings reads the [tickets] table. The decoder is Host.Section, which rejects keys that
// settings does not declare.
func loadSettings(decodeSection func(name string, target any) error) (settings, error) {
	var section settings
	if err := decodeSection("tickets", &section); err != nil {
		return settings{}, err
	}
	section.URLPrefix = cmp.Or(section.URLPrefix, defaultURLPrefix)
	if !strings.HasPrefix(section.URLPrefix, "https://") && !strings.HasPrefix(section.URLPrefix, "http://") {
		return settings{}, errors.New(`[tickets] url_prefix must start with "https://" or "http://"`)
	}
	return section, nil
}
```

`host.Section` decodes one table into a struct and rejects any key the struct does not
declare, so a misspelled key is an error that names the file. Asking for a table also makes it
a known table, so the host does not report it as unknown. A missing table is not an error: it
decodes to the zero value, and the plugin applies its defaults. Give every setting a neutral
default, validate it, and name the table and key in the error message. Never compile in a
personal preference, and never put a credential in the file.

The configuration that turns the plugin on looks like this:

```toml
plugins = ["pull_requests", "tickets"]

[tickets]
url_prefix = "https://tickets.example.com/browse/"   # optional
```

### The data

`plugins/tickets/tracker.go`:

```go
package tickets

import "strings"

// ticket is one made-up ticket. A real plugin would fetch these from a service.
type ticket struct {
	ID     string
	Title  string
	Status string
}

const (
	statusOpen       = "Open"
	statusInProgress = "In progress"
	statusDone       = "Done"
)

func exampleTickets() []ticket {
	return []ticket{
		{ID: "TCK-101", Title: "Add the sign-in page", Status: statusInProgress},
		{ID: "TCK-102", Title: "Fix the invoice total", Status: statusOpen},
		{ID: "TCK-103", Title: "Retire the old export", Status: statusDone},
	}
}

// tracker is what the plugin's board, column, and actions share. It is never changed after
// setup, because they run on different goroutines.
type tracker struct {
	urlPrefix string
	tickets   []ticket
}

func newTracker(settings settings) *tracker {
	return &tracker{urlPrefix: settings.URLPrefix, tickets: exampleTickets()}
}

func (tracker *tracker) url(target ticket) string {
	return tracker.urlPrefix + target.ID
}

// ticketForBranch finds the first ticket whose id appears in the branch name, ignoring case,
// such as TCK-101 in "feature/tck-101-sign-in".
func (tracker *tracker) ticketForBranch(branch string) (ticket, bool) {
	upperBranch := strings.ToUpper(branch)
	for _, candidate := range tracker.tickets {
		if strings.Contains(upperBranch, candidate.ID) {
			return candidate, true
		}
	}
	return ticket{}, false
}
```

`tracker` holds what the board, the column, and the actions share. It is built once in `Setup`
and never changed afterwards. That matters because the board's functions run on different
goroutines, as the next sections explain. A plugin that needs state that changes must protect
it with a lock, or better, put what a cell needs on the row.

### A board of its own

`plugins/tickets/board.go`:

```go
package tickets

import (
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/internal/ui"
)

const boardName = "tickets"

// ticketRow is a row of the plugin's own board. Its key is the ticket's id, which stays the
// same across refreshes.
type ticketRow struct{ ticket ticket }

func (row ticketRow) Key() string { return row.ticket.ID }

func (tracker *tracker) board() plugin.Board {
	return plugin.Board{
		Name:    boardName,
		Title:   "Tickets",
		RowNoun: "tickets",
		Load:    tracker.load,
		Columns: []plugin.Column{
			{Header: "Ticket", Cell: func(row plugin.Row, _ any) []ui.Segment {
				return ui.PlainSegments(row.(ticketRow).ticket.ID)
			}},
			{Header: "Title", Flexible: true, Cell: func(row plugin.Row, _ any) []ui.Segment {
				return ui.PlainSegments(row.(ticketRow).ticket.Title)
			}},
			{Header: "Status", Cell: func(row plugin.Row, _ any) []ui.Segment {
				return ui.PlainSegments(row.(ticketRow).ticket.Status)
			}},
		},
		Actions: []plugin.Action{
			{Key: "o", Label: "open", Run: func(selection plugin.Selection) plugin.Effect {
				return tracker.openInBrowser(selection.Row.(ticketRow).ticket)
			}},
		},
		Facets: []plugin.Facet{{
			Title: "Status",
			Options: []plugin.FacetOption{
				{Value: statusOpen, Label: statusOpen},
				{Value: statusInProgress, Label: statusInProgress},
				{Value: statusDone, Label: statusDone},
			},
			Value: func(row plugin.Row) string { return row.(ticketRow).ticket.Status },
		}},
	}
}

// load runs on a background goroutine. A plugin that asks a service for its rows does it here.
func (tracker *tracker) load() ([]plugin.Row, error) {
	rows := make([]plugin.Row, len(tracker.tickets))
	for index, known := range tracker.tickets {
		rows[index] = ticketRow{ticket: known}
	}
	return rows, nil
}

// openInBrowser returns an Effect rather than running anything, so the host shows the notice
// and runs the command on a background goroutine.
func (tracker *tracker) openInBrowser(target ticket) plugin.Effect {
	return plugin.Effect{
		Notice: "Opening " + target.ID + "…",
		Command: func() error {
			_, err := shell.Run("open", tracker.url(target))
			return err
		},
	}
}
```

A board is a `plugin.Board`. Its rows implement `plugin.Row`, whose one method `Key` must
return a stable identity, such as an id or a URL. The cursor and any loaded column data follow
the key across refreshes, so a key that changes on every load makes the cursor jump.

Each column has a `Header` and a `Cell` function that draws one row as styled segments. At
most one column may be `Flexible`; it shrinks when the terminal is narrow. The facet adds a
choice to the filter menu (`f`): its `Value` function says which option a row has, and with no
`Selected` set, every option starts selected.

An action is a key and a `Run` function. `Run` does not do the work. It returns a
`plugin.Effect` that describes it, and the host carries it out: `Notice` goes in the status
line, `Command` runs on a background goroutine and a returned error shows as `Failed: ...`,
`Confirm` asks a question first, `Refresh` reloads the board after the command succeeds, and
`Screen` opens a full-screen view. The host refuses keys it uses itself (for example `f`, `r`,
`q`, and `tab`) and two actions on one key, and says so when the program starts.

All external commands go through `shell.Run`, which is what tests replace.

### A column on another plugin's board

`plugins/tickets/pullrequests.go`:

```go
package tickets

import (
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

const ticketColumnHeader = "Ticket"

// pullRequestColumn is the Ticket column of the pull request board. Its cells are empty until
// the fetch has delivered a ticket, and stay empty for a pull request whose branch names none.
func (tracker *tracker) pullRequestColumn() plugin.Column {
	return plugin.Column{
		Header: ticketColumnHeader,
		Cell: func(_ plugin.Row, data any) []ui.Segment {
			found, loaded := data.(ticket)
			if !loaded {
				return nil
			}
			return ui.PlainSegments(found.ID)
		},
		// Load runs on the update loop, so it only reads the branches off the rows. The fetch it
		// returns does the matching on a background goroutine.
		Load: func(rows []plugin.Row) []plugin.Fetch {
			branches := make(map[string]string, len(rows))
			for _, row := range rows {
				if pullRequestRow, isPullRequest := row.(pullrequests.Row); isPullRequest {
					branches[row.Key()] = pullRequestRow.PullRequest.HeadBranch
				}
			}
			return []plugin.Fetch{func() (map[string]any, error) {
				matched := map[string]any{}
				for key, branch := range branches {
					if found, hasTicket := tracker.ticketForBranch(branch); hasTicket {
						matched[key] = found
					}
				}
				return matched, nil
			}}
		},
	}
}

// openTicketOfPullRequest is the t key of the pull request board. It reads the ticket that the
// Ticket column loaded for the selected row.
func (tracker *tracker) openTicketOfPullRequest() plugin.Action {
	return plugin.Action{
		Key:   "t",
		Label: "ticket",
		Run: func(selection plugin.Selection) plugin.Effect {
			found, matched := selection.Data(ticketColumnHeader).(ticket)
			if !matched {
				return plugin.Effect{Notice: "No ticket matches this branch"}
			}
			return tracker.openInBrowser(found)
		},
	}
}
```

`host.AddColumn("pull_requests", column)` appends the column after the board's own. Rows of
that board are `pullrequests.Row` values, so the column's `Load` type-asserts each row and
reads `PullRequest.HeadBranch`. The type assertion uses the two-value form because a plugin
should not panic on a row it does not recognise.

A column's `Load` returns a list of independent `Fetch` functions. The host runs them in the
background after every refresh and delivers each result, a map from `Row.Key()` to a value.
`Cell` receives that value as `data`, and it is nil until the fetch has delivered one. This
is why the cell above draws nothing for both a pull request that is still loading and one that
has no ticket. Delivered values survive later refreshes. If a fetch returns an error, its
cells stay unloaded.

The action reads the loaded value with `selection.Data("Ticket")`, which looks a column up by
its header. If no ticket was delivered, it sets a notice and runs nothing. Choose a key that
the target board does not already use; the pull request board uses `enter`, `p`, `o`, `v`,
`d`, `e`, and `m`.

### Switching it on

A blank import in `main.go` registers the plugin:

`main.go`:

```go
_ "github.com/ghmeier/pr-tui/plugins/tickets"
```

Then name it in the configuration file, as shown above. Registered plugins are all built, and
only the listed ones run. The example plugin is imported by `main.go` so that it is always
compiled and tested, but it stays off until someone lists it. A plugin that is not in the list
can still have a `[tickets]` table in the file without an error.

## Where code runs

The host runs your functions in two places, and the rule for shared state follows from it.

On a background goroutine, possibly several at once:

- `Board.Load`
- each `Fetch` a column's `Load` returns
- `Effect.Command`

On the update loop, which draws the screen and must not be blocked:

- `Column.Load` (it only returns the fetches; the work belongs in them)
- `Column.Cell`
- `Facet.Value`
- `Action.Run` (it only describes an effect; slow work belongs in its `Command`)

State that one kind of function shares with the other kind needs a lock. It is simpler to
avoid shared state: put whatever a cell needs on the row, and treat rows as values that are
never changed after they are created. `Selection.Data` is a copy made when the selection was
built, so it is safe to call from anywhere, including inside a `Command`. That is why
`openTicketOfPullRequest` can read it in `Run` and why a `Command` can capture the result.

Notice how the Ticket column follows this. `Load` runs on the update loop, so it only copies
the branch names off the rows. The returned fetch does the matching on a goroutine, using the
`branches` map it owns and the `tracker`, which is read-only.

## Screens

An action can open a full-screen view by returning `Effect{Screen: screen}`. A `Screen` is an
interface of three methods:

```go
type Screen interface {
	Init() tea.Cmd
	Update(message tea.Msg) (Screen, tea.Cmd)
	View(width, height int) string
}
```

It is a Bubble Tea model in miniature. `Init` returns the first command, such as a load.
`Update` returns the screen to keep, and returns `nil` to close itself. `View` draws at the
size it is given.

While any screen is open it receives every key except `ctrl+c`, and also every message the host
does not recognise, because screens run their own background commands and their results come
back as messages. The host does not route them: a screen sees messages meant for other
screens, and for other rows of the same screen type. A screen must ignore a message that is
not its own, for example by comparing the pull request in the message with its own and
returning unchanged on a mismatch.

A screen keeps the `Selection` it was opened with. To have the host do something for that
board, it calls `selection.Run(effect)`, which returns a command. That lets a screen set a
notice, ask a confirmation, run a command, refresh the board, or open another screen, with the
same `Effect` an action uses. The effect applies to the board the selection came from, even if
another tab is in view when it arrives.

`plugins/pullrequests/descriptionscreen.go` is the smaller of the two real screens (the other
is the diff view). It loads the description in `Init`, ignores `descriptionLoadedMessage`
values for other pull requests in `Update`, and uses `selection.Run` to show a failed save as a
notice on the board after the screen may already have closed.

## Writing a provider for the pull request plugin

A provider is a data source behind a plugin. The pull request plugin never talks to a forge
directly. It talks to the `pullrequests.Provider` interface in
`plugins/pullrequests/provider.go`, and the GitHub provider in `plugins/pullrequests/github`
is the only code that runs `gh`. To support another forge:

1. Create a package under `plugins/pullrequests`, such as `plugins/pullrequests/gitlab`, that
   implements `pullrequests.Provider`.
2. Register it from `init`:

   ```go
   func init() {
   	pullrequests.RegisterProvider("gitlab", newProvider)
   }
   ```

3. Have the factory read its own table, `[gitlab]`, through the `SectionDecoder` it is given.
   That decodes strictly, like `host.Section`. See `newProvider` in
   `plugins/pullrequests/github/github.go` for validation that names the table and key.
4. Import the package for its side effect in `main.go`, and select it in the configuration
   file with `provider = "gitlab"` under `[pull_requests]`.

Only the provider package may run the forge's command-line tool or know its vocabulary. It
returns the neutral types in `plugins/pullrequests/types.go`, such as `CIState`. Use
`shell.Run` for external commands so that tests can replace it. Credentials are never
settings: use the login of the tool the provider calls.

Some of the pull request plugin is still shaped like GitHub, and a second forge would need work
there:

- The auto-merge question in `toggleAutoMergeQuestion` says "squash", which is GitHub's
  merge method.
- The `v` key builds a worktrunk command with its `pr:<number>` shorthand, which resolves a
  pull request number to its branch on GitHub.

## Testing a plugin

Test at the layer where the behavior lives.

**Host behavior** belongs in `internal/plugin`, tested with fake plugins and fake rows. A plugin's
author rarely needs this.

**Plugin behavior** is tested by driving the real host. Build a configuration with
`configtest.Load`, call `plugin.NewHost`, and then use `plugintest`: `Start` loads every board
and waits for the column data, `Press` sends keys and settles what follows, and `Settle`
runs a command and delivers its messages until nothing is left to run. Assert on what the user
would see (`ansi.Strip(model.View())`) and on the commands that would run. For a plugin that adds
to the pull request board, use a fake provider. `pullrequeststest.Provider` is a small one:

```go
provider := &pullrequeststest.Provider{Source: repository, Open: pullRequests}
name := pullrequeststest.Register(t, provider) // the value for provider = "<name>"
```

The pull request plugin's own tests use a larger fake that records the changes it is asked to
make. `plugins/pullrequests/compose_test.go` shows another plugin's column, action, and board
being added next to the pull request board, and is the best example of this layer.

**Provider behavior** is tested by replacing `shell.Run`, asserting the exact command line, and
parsing a canned response. Tests never run the real `gh` or touch the network.

The tests for `tickets` are in `plugins/tickets/tickets_test.go`. This helper replaces
`shell.Run`, so that no test opens a browser:

`plugins/tickets/tickets_test.go`:

```go
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
```

This test uses it to check what the `t` key does on a pull request:

`plugins/tickets/tickets_test.go`:

```go
func TestTicketKeyOnAPullRequestOpensTheMatchedTicketsAddress(t *testing.T) {
	f := newFixture()
	commands := recordShell(t)
	model := startedWithBoth(t, f)

	plugintest.Press(t, model, "t")

	expected := [][]string{{"open", defaultURLPrefix + f.ticket.ID}}
	if !reflect.DeepEqual(*commands, expected) {
		t.Fatalf("commands = %v, expected %v", *commands, expected)
	}
}
```

`plugins/tickets/guide_test.go` fails when a code block in this guide that is introduced by the
path of a source file is not found in that file. After you change a plugin file, copy the
changed code into the guide.

Each test states one invariant and is named by subject and expected behavior. Arrange, act, and
assert are separated by blank lines, a positive case is paired with a negative one, and
assertions use the fixture's own values rather than retyped literals. The same file covers the
column for a matched and an unmatched pull request, the second tab, the filter, and the plugin
starting with `pull_requests` disabled.

## Checklist before opening a pull request

- No personal preference is compiled in: no repository, organization, team, username, editor,
  or path. Each is a setting with a neutral default, documented in the README and the starter
  configuration.
- No credential is in the code or the configuration file.
- Configuration is read with `host.Section`, so unknown keys are errors. Every setting has a
  default, a validation error that names the table and key, and tests for the accepted and
  rejected forms.
- A column or action added to another plugin's board is guarded by `host.Enabled`, and the
  plugin starts with that other plugin switched off.
- Slow work is in a `Fetch`, a `Board.Load`, or an `Effect.Command`, never in `Cell`,
  `Action.Run`, or `Column.Load`. State shared between goroutines is locked or avoided.
- Screens ignore messages that are not their own.
- External commands go through `shell.Run`, and only a provider package knows its service.
- Tests are at the right layer, never use the network or the real `gh`, and the plugin's
  names are spelled out and descriptive.
- `go vet ./... && gofmt -l .` is clean and `go test ./...` passes. Do not run a bare
  `go build`; it leaves a binary in the working tree.
