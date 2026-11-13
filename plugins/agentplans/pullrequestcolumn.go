package agentplans

import (
	"fmt"
	"slices"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

// linkedDocs are the documents linked to one pull request's head branch, in the order apl
// lists them.
type linkedDocs []Doc

// pullRequestColumn is the Plan column of the pull request board. A cell stays empty until the
// fetch has delivered documents, and for a pull request whose branch has none linked.
func (settings settings) pullRequestColumn() plugin.Column {
	return plugin.Column{
		Header: "Plan",
		Cell: func(_ plugin.Row, data any) []ui.Segment {
			docs, loaded := data.(linkedDocs)
			if !loaded || len(docs) == 0 {
				return nil
			}
			segments := ui.PlainSegments(docs[0].Title)
			if rest := len(docs) - 1; rest > 0 {
				segments = append(segments, ui.Segment{Text: fmt.Sprintf(" +%d", rest), Style: ui.DimStyle})
			}
			return segments
		},
		// Load runs on the update loop, so it only reads names and branches off the rows. Each
		// fetch lists one checkout's documents on a background goroutine.
		Load: func(rows []plugin.Row) []plugin.Fetch {
			branchesByKey := map[string]map[string]string{}
			for _, row := range rows {
				pullRequestRow, isPullRequest := row.(pullrequests.Row)
				if !isPullRequest {
					continue
				}
				name := pullRequestRow.PullRequest.Repository.Name
				if branchesByKey[name] == nil {
					branchesByKey[name] = map[string]string{}
				}
				branchesByKey[name][row.Key()] = pullRequestRow.PullRequest.HeadBranch
			}
			var fetches []plugin.Fetch
			for _, checkout := range settings.checkouts {
				if branches := branchesByKey[checkout.name]; len(branches) > 0 {
					fetches = append(fetches, checkout.fetchLinkedDocs(branches))
				}
			}
			return fetches
		},
	}
}

// fetchLinkedDocs delivers, for each pull request key, the documents linked to its branch.
func (checkout checkout) fetchLinkedDocs(branchesByKey map[string]string) plugin.Fetch {
	return func() (map[string]any, error) {
		docs, err := checkout.docs()
		if err != nil {
			return nil, err
		}
		linked := map[string]any{}
		for key, branch := range branchesByKey {
			var matching linkedDocs
			for _, doc := range docs {
				if slices.Contains(doc.Branches, branch) {
					matching = append(matching, doc)
				}
			}
			if len(matching) > 0 {
				linked[key] = matching
			}
		}
		return linked, nil
	}
}
