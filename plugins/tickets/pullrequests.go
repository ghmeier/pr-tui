package tickets

import (
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

const ticketColumnHeader = "Ticket"

// pullRequestColumn is the Ticket column of the pull request board. Its cells are empty until
// the fetch has delivered a ticket, and stay empty for a pull request whose branch names none.
func (tracker *tracker) pullRequestColumn() plugin.Column {
	return plugin.Column{
		Header: ticketColumnHeader,
		Cell: func(_ plugin.Row, data any) []ui.Segment {
			found, loaded := data.(ticket)
			if !loaded {
				return nil
			}
			return ui.PlainSegments(found.ID)
		},
		// Load runs on the update loop, so it only reads the branches off the rows. The fetch it
		// returns does the matching on a background goroutine.
		Load: func(rows []plugin.Row) []plugin.Fetch {
			branches := make(map[string]string, len(rows))
			for _, row := range rows {
				if pullRequestRow, isPullRequest := row.(pullrequests.Row); isPullRequest {
					branches[row.Key()] = pullRequestRow.PullRequest.HeadBranch
				}
			}
			return []plugin.Fetch{func() (map[string]any, error) {
				matched := map[string]any{}
				for key, branch := range branches {
					if found, hasTicket := tracker.ticketForBranch(branch); hasTicket {
						matched[key] = found
					}
				}
				return matched, nil
			}}
		},
	}
}

// openTicketOfPullRequest is the t key of the pull request board. It reads the ticket that the
// Ticket column loaded for the selected row.
func (tracker *tracker) openTicketOfPullRequest() plugin.Action {
	return plugin.Action{
		Key:   "t",
		Label: "ticket",
		Run: func(selection plugin.Selection) plugin.Effect {
			found, matched := selection.Data(ticketColumnHeader).(ticket)
			if !matched {
				return plugin.Effect{Notice: "No ticket matches this branch"}
			}
			return tracker.openInBrowser(found)
		},
	}
}
