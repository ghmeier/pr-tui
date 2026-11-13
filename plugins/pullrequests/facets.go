package pullrequests

import (
	"fmt"
	"strings"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

const (
	people = "people"
	bots   = "bots"
)

// facets are the repositories, selected by full name, and whether to show people's pull
// requests or bots'. The start values come from the command line.
func (board *pullRequestBoard) facets(selectedRepositories []string, showBots bool) []plugin.Facet {
	var repositoryOptions []plugin.FacetOption
	for _, source := range board.provider.Repositories() {
		repositoryOptions = append(repositoryOptions, plugin.FacetOption{Value: source.FullName(), Label: source.Name})
	}
	selectedAuthors := people
	if showBots {
		selectedAuthors = bots
	}
	return []plugin.Facet{
		{
			Title:    "Repositories",
			Options:  repositoryOptions,
			Selected: selectedRepositories,
			Value:    func(row plugin.Row) string { return row.(Row).PullRequest.Repository.FullName() },
		},
		{
			Title:   "Authors",
			Options: []plugin.FacetOption{{Value: people, Label: people}, {Value: bots, Label: bots}},
			Value: func(row plugin.Row) string {
				if row.(Row).PullRequest.IsBot {
					return bots
				}
				return people
			},
			Single:   true,
			Selected: []string{selectedAuthors},
			CycleKey: "b",
		},
	}
}

// resolveRepositories turns each --repo value, either a short name or owner/name, into the
// full name the repositories facet uses. A short name shared by two owners is ambiguous and
// must be spelled out.
func resolveRepositories(available []Repository, values []string) ([]string, error) {
	var names []string
	for _, value := range values {
		var matches []Repository
		for _, source := range available {
			if value == source.FullName() || value == source.Name {
				matches = append(matches, source)
			}
		}
		switch len(matches) {
		case 0:
			fullNames := make([]string, len(available))
			for index, source := range available {
				fullNames[index] = source.FullName()
			}
			return nil, fmt.Errorf("unknown repository %q; configured repositories: %s", value, strings.Join(fullNames, ", "))
		case 1:
			names = append(names, matches[0].FullName())
		default:
			return nil, fmt.Errorf("repository name %q matches more than one owner; write it as owner/name", value)
		}
	}
	return names, nil
}
