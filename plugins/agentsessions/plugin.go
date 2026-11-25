// Package agentsessions shows which live agent session is working on each row of another
// plugin's board, and sends a message to that session. It has no session-specific code:
// sessions come from a Provider.
package agentsessions

import (
	"flag"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

const pluginName = "agent_sessions"

func init() {
	plugin.Register(&agentSessionsPlugin{})
}

type agentSessionsPlugin struct{}

func (*agentSessionsPlugin) Name() string { return pluginName }

// Flags has nothing to register, because this plugin takes no command-line flags.
func (*agentSessionsPlugin) Flags(*flag.FlagSet) {}

func (*agentSessionsPlugin) Setup(host *plugin.Host) error {
	settings, err := loadSettings(host.Section)
	if err != nil {
		return err
	}
	provider, err := newProvider(settings.providerName, ProviderSettings{RelayModel: settings.relayModel})
	if err != nil {
		return err
	}
	if host.Enabled(pullrequests.BoardName) {
		host.AddColumn(pullrequests.BoardName, pullRequestColumn(provider))
		host.AddAction(pullrequests.BoardName, pullRequestMessageAction(provider))
	}
	return nil
}
