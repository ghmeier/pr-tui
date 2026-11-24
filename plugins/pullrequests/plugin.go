// Package pullrequests is the board of open pull requests. It is a plugin for the host in
// internal/plugin, and it has no forge-specific code: pull requests come from a Provider.
package pullrequests

import (
	"flag"
	"slices"
	"strings"
	"sync"

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
	reviews    *pendingReviews
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
		reviews:    newPendingReviews(),
	}
	host.AddBoard(plugin.Board{
		Name:    BoardName,
		Title:   "Pull requests",
		RowNoun: "open pull requests",
		Load:    board.load,
		Reload:  board.reload,
		Columns: board.columns(),
		Actions: board.actions(),
		Facets:  board.facets(selected, pullRequests.showBots),
	})
	return nil
}

// reload returns nil when the pull request is no longer open, which takes it off the board.
func (board *pullRequestBoard) reload(row plugin.Row) (plugin.Row, error) {
	previous := row.(Row)
	pr, open, err := board.provider.PullRequest(previous.PullRequest)
	if err != nil || !open {
		return nil, err
	}
	return Row{PullRequest: pr, Viewer: previous.Viewer, TeamMembers: previous.TeamMembers}, nil
}

func (board *pullRequestBoard) load() ([]plugin.Row, error) {
	waitForMembers := board.loadTeamMembers()
	pullRequests, viewer, err := board.provider.PullRequests()
	membersByOwner := waitForMembers()
	if err != nil {
		return nil, err
	}
	rows := make([]plugin.Row, len(pullRequests))
	for index, pr := range pullRequests {
		rows[index] = Row{PullRequest: pr, Viewer: viewer, TeamMembers: membersByOwner[strings.ToLower(pr.Repository.Owner)]}
	}
	return rows, nil
}

// loadTeamMembers starts listing the review team's members once per distinct owner of the
// configured repositories, while the pull requests load. The function it returns waits for the
// listing and gives the members by lowercase owner. Members are a convenience, so an owner whose
// members cannot be listed simply has none.
func (board *pullRequestBoard) loadTeamMembers() func() map[string][]string {
	var owners []string
	var repositories []Repository
	if board.reviewTeam != "" {
		for _, repository := range board.provider.Repositories() {
			owner := strings.ToLower(repository.Owner)
			if !slices.Contains(owners, owner) {
				owners = append(owners, owner)
				repositories = append(repositories, repository)
			}
		}
	}
	// Each goroutine writes only its own element, so no lock is needed before the wait.
	listed := make([][]string, len(owners))
	var group sync.WaitGroup
	for index, repository := range repositories {
		group.Go(func() {
			if logins, err := board.provider.TeamMembers(repository); err == nil {
				listed[index] = logins
			}
		})
	}
	return func() map[string][]string {
		group.Wait()
		membersByOwner := make(map[string][]string, len(owners))
		for index, owner := range owners {
			membersByOwner[owner] = listed[index]
		}
		return membersByOwner
	}
}
