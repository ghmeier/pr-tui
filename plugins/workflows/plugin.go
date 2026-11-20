// Package workflows shows CI/CD workflow runs across a configured set of repositories, one
// board for each configured section. It is a plugin for the host in internal/plugin, and it has
// no service-specific code: runs come from a Provider.
package workflows

import (
	"flag"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

const pluginName = "workflows"

// tabTitle is the one tab that every section is a pane of.
const tabTitle = "Workflows"

func init() {
	plugin.Register(&workflowsPlugin{})
}

type workflowsPlugin struct{}

func (*workflowsPlugin) Name() string { return pluginName }

// Flags has nothing to register, because this plugin takes no command-line flags.
func (*workflowsPlugin) Flags(*flag.FlagSet) {}

func (*workflowsPlugin) Setup(host *plugin.Host) error {
	settings, err := loadSettings(host.Section)
	if err != nil {
		return err
	}
	provider, err := newProvider(settings.providerName)
	if err != nil {
		return err
	}
	for _, configured := range settings.sections {
		var board plugin.Board
		switch configured.kind {
		case sectionKindRuns:
			board = runsBoard(provider, configured)
		case sectionKindWorkflows:
			board = workflowsBoard(provider, configured)
		}
		board.RefreshInterval = settings.refreshInterval
		host.AddBoard(board)
	}
	return nil
}

// boardName is the name another plugin passes to AddColumn and AddAction to add to a section's
// board. The rows of a runs section are RunRow, and those of a workflows section WorkflowRow.
func boardName(title string) string {
	return pluginName + ":" + title
}
