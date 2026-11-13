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
