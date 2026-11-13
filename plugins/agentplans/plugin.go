// Package agentplans lists the apl documents (plans, research notes, and handoffs) of local
// checkouts on a board, with the pull requests each one mentions. It runs the apl program in
// each checkout and reads the documents from the checkout's .apl directory.
package agentplans

import (
	"flag"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

const pluginName = "agent_plans"

// BoardName is the name of the board, for plugins that add to it.
const BoardName = pluginName

func init() {
	plugin.Register(&agentPlansPlugin{})
}

type agentPlansPlugin struct{}

func (*agentPlansPlugin) Name() string { return pluginName }

// Flags has nothing to register, because this plugin takes no command-line flags.
func (*agentPlansPlugin) Flags(*flag.FlagSet) {}

func (*agentPlansPlugin) Setup(host *plugin.Host) error {
	settings, err := loadSettings(host.Section)
	if err != nil {
		return err
	}
	host.AddBoard(board(settings))
	return nil
}
