package pullrequests

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// filters keys the selected repositories by full name. available is the configured
// repositories in order. The first nine also have number keys in the filter menu; cursor is
// the repository the menu is on.
type filters struct {
	available    []Repository
	repositories map[string]bool
	showBots     bool
	cursor       int
}

// numberKeyCount is how many repositories get a number key, as the keys are 1 to 9.
const numberKeyCount = 9

func newFilters(available []Repository, selectedRepositories []string, showBots bool) filters {
	selected := map[string]bool{}
	if len(selectedRepositories) == 0 {
		for _, source := range available {
			selected[source.FullName()] = true
		}
	}
	for _, name := range selectedRepositories {
		selected[name] = true
	}
	return filters{available: available, repositories: selected, showBots: showBots}
}

func (f filters) matches(pr PullRequest) bool {
	return f.repositories[pr.Repository.FullName()] && pr.IsBot == f.showBots
}

func (f filters) visible(pullRequests []PullRequest) []PullRequest {
	var visible []PullRequest
	for _, pr := range pullRequests {
		if f.matches(pr) {
			visible = append(visible, pr)
		}
	}
	return visible
}

func (f filters) authors() string {
	if f.showBots {
		return "bots"
	}
	return "people"
}

func (f filters) repositoryMark(source Repository) string {
	if f.repositories[source.FullName()] {
		return "● " + source.Name
	}
	return ui.DimStyle.Render("○ " + source.Name)
}

func (f filters) summary() string {
	parts := make([]string, len(f.available))
	for index, source := range f.available {
		parts[index] = f.repositoryMark(source)
	}
	return "Repositories: " + strings.Join(parts, "  ") + "    Authors: " + ui.BoldStyle.Render(f.authors())
}

func (f filters) menu() string {
	lines := []string{ui.BoldStyle.Render("Filters"), ""}
	for index, source := range f.available {
		marker, key := " ", " "
		if index == f.cursor {
			marker = ui.CursorMarker
		}
		if index < numberKeyCount {
			key = ui.KeyStyle.Render(strconv.Itoa(index + 1))
		}
		lines = append(lines, marker+" "+key+"  "+f.repositoryMark(source))
	}
	lines = append(lines, "", "  "+ui.KeyStyle.Render("b")+"  authors: "+f.authors())
	lines = append(lines, "", "  "+ui.HelpLine("↑↓", "move", "space", "toggle", "esc", "close"))
	return ui.MenuStyle.Render(strings.Join(lines, "\n"))
}

// resolveRepositories turns each --repo value, either a short name or owner/name, into the
// full name filters use. A short name shared by two owners is ambiguous and must be spelled out.
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
