// Package pullrequests is the board of open pull requests. It is a plugin for the host in
// internal/plugin, and it has no forge-specific code: pull requests come from a Provider.
package pullrequests

import (
	"flag"

	"github.com/ghmeier/pr-tui/internal/config"
	"github.com/ghmeier/pr-tui/internal/plugin"
)

const pluginName = "pull_requests"

// BoardName is the name other plugins pass to AddColumn and AddAction to add to this board.
// The rows of the board are of the type Row.
const BoardName = pluginName

func init() {
	plugin.Register(&pullRequestsPlugin{})
}

type pullRequestsPlugin struct {
	// repositories and showBots are the command-line choices of how the board starts.
	repositories []string
	showBots     bool
}

func (*pullRequestsPlugin) Name() string { return pluginName }

func (pullRequests *pullRequestsPlugin) Flags(flags *flag.FlagSet) {
	flags.Func("repo", "start with only this repository selected, as a name or owner/name (repeatable)", func(value string) error {
		pullRequests.repositories = append(pullRequests.repositories, value)
		return nil
	})
	flags.BoolVar(&pullRequests.showBots, "bots", false, "start on bot-authored pull requests")
}

// pullRequestBoard is what the board's columns, actions, and screens share. Everything in it
// is fixed once the board is set up, because they run on different goroutines.
type pullRequestBoard struct {
	provider   Provider
	settings   boardSettings
	editor     config.Editor
	reviewTeam string
	diffs      *diffCache
}

func (pullRequests *pullRequestsPlugin) Setup(host *plugin.Host) error {
	settings, err := loadSettings(host.Section)
	if err != nil {
		return err
	}
	provider, err := NewProvider(settings.ProviderName, host.Section)
	if err != nil {
		return err
	}
	selected, err := resolveRepositories(provider.Repositories(), pullRequests.repositories)
	if err != nil {
		return plugin.UsageError(err)
	}
	board := &pullRequestBoard{
		provider:   provider,
		settings:   settings,
		editor:     host.Editor(),
		reviewTeam: provider.ReviewTeam(),
		diffs:      newDiffCache(),
	}
	host.AddBoard(plugin.Board{
		Name:    BoardName,
		Title:   "Pull requests",
		RowNoun: "open pull requests",
		Load:    board.load,
		Columns: board.columns(),
		Actions: board.actions(),
		Facets:  board.facets(selected, pullRequests.showBots),
	})
	return nil
}

func (board *pullRequestBoard) load() ([]plugin.Row, error) {
	pullRequests, viewer, err := board.provider.PullRequests()
	if err != nil {
		return nil, err
	}
	rows := make([]plugin.Row, len(pullRequests))
	for index, pr := range pullRequests {
		rows[index] = Row{PullRequest: pr, Viewer: viewer}
	}
	return rows, nil
}
