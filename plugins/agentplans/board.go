package agentplans

import (
	"fmt"
	"strings"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// shownPullRequests is how many numbers the pull request cell spells out before it counts the
// rest.
const shownPullRequests = 3

// noStatus is the facet value of a document that has no status.
const noStatus = "none"

func board(settings settings) plugin.Board {
	repositories := make([]plugin.FacetOption, len(settings.checkouts))
	for index, checkout := range settings.checkouts {
		repositories[index] = plugin.FacetOption{Value: checkout.name, Label: checkout.name}
	}
	return plugin.Board{
		Name:    BoardName,
		Title:   "Agent plans",
		RowNoun: "docs",
		Load:    settings.load,
		Actions: []plugin.Action{
			{Key: "enter", Label: "view", Run: func(selection plugin.Selection) plugin.Effect {
				return plugin.Effect{Screen: newDocumentScreen(settings, selection)}
			}},
			{Key: "v", Label: settings.editor.Name, Run: func(selection plugin.Selection) plugin.Effect {
				return settings.openInEditor(selection.Row.(Row))
			}},
		},
		Columns: []plugin.Column{
			textColumn("Repository", false, func(row Row) string { return row.Repository }),
			textColumn("Type", false, func(row Row) string { return row.Doc.Type }),
			textColumn("Title", true, func(row Row) string { return row.Doc.Title }),
			textColumn("Status", false, func(row Row) string { return row.Doc.Status }),
			textColumn("Updated", false, func(row Row) string { return row.Doc.Updated }),
			{Header: "Pull requests", Cell: func(row plugin.Row, _ any) []ui.Segment {
				return pullRequestSegments(row.(Row).Doc.PullRequestNumbers)
			}},
		},
		Facets: []plugin.Facet{
			{
				Title:   "Repositories",
				Options: repositories,
				Value:   func(row plugin.Row) string { return row.(Row).Repository },
			},
			{
				Title:           "Type",
				OptionsFromRows: true,
				Selected:        []string{"plan"},
				Value:           func(row plugin.Row) string { return row.(Row).Doc.Type },
			},
			{
				Title:           "Status",
				OptionsFromRows: true,
				Value: func(row plugin.Row) string {
					if status := row.(Row).Doc.Status; status != "" {
						return status
					}
					return noStatus
				},
			},
		},
	}
}

func textColumn(header string, flexible bool, text func(row Row) string) plugin.Column {
	return plugin.Column{Header: header, Flexible: flexible, Cell: func(row plugin.Row, _ any) []ui.Segment {
		return ui.PlainSegments(text(row.(Row)))
	}}
}

// pullRequestSegments spells out the largest numbers and counts the others.
func pullRequestSegments(numbers []int) []ui.Segment {
	shown := numbers[:min(len(numbers), shownPullRequests)]
	labels := make([]string, len(shown))
	for index, number := range shown {
		labels[index] = fmt.Sprintf("#%d", number)
	}
	segments := ui.PlainSegments(strings.Join(labels, " "))
	if rest := len(numbers) - len(shown); rest > 0 {
		segments = append(segments, ui.Segment{Text: fmt.Sprintf(" +%d", rest), Style: ui.DimStyle})
	}
	return segments
}
