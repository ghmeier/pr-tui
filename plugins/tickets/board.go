package tickets

import (
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/internal/ui"
)

const boardName = "tickets"

// ticketRow is a row of the plugin's own board. Its key is the ticket's id, which stays the
// same across refreshes.
type ticketRow struct{ ticket ticket }

func (row ticketRow) Key() string { return row.ticket.ID }

func (tracker *tracker) board() plugin.Board {
	return plugin.Board{
		Name:    boardName,
		Title:   "Tickets",
		RowNoun: "tickets",
		Load:    tracker.load,
		Columns: []plugin.Column{
			{Header: "Ticket", Cell: func(row plugin.Row, _ any) []ui.Segment {
				return ui.PlainSegments(row.(ticketRow).ticket.ID)
			}},
			{Header: "Title", Flexible: true, Cell: func(row plugin.Row, _ any) []ui.Segment {
				return ui.PlainSegments(row.(ticketRow).ticket.Title)
			}},
			{Header: "Status", Cell: func(row plugin.Row, _ any) []ui.Segment {
				return ui.PlainSegments(row.(ticketRow).ticket.Status)
			}},
		},
		Actions: []plugin.Action{
			{Key: "o", Label: "open", Run: func(selection plugin.Selection) plugin.Effect {
				return tracker.openInBrowser(selection.Row.(ticketRow).ticket)
			}},
		},
		Facets: []plugin.Facet{{
			Title: "Status",
			Options: []plugin.FacetOption{
				{Value: statusOpen, Label: statusOpen},
				{Value: statusInProgress, Label: statusInProgress},
				{Value: statusDone, Label: statusDone},
			},
			Value: func(row plugin.Row) string { return row.(ticketRow).ticket.Status },
		}},
	}
}

// load runs on a background goroutine. A plugin that asks a service for its rows does it here.
func (tracker *tracker) load() ([]plugin.Row, error) {
	rows := make([]plugin.Row, len(tracker.tickets))
	for index, known := range tracker.tickets {
		rows[index] = ticketRow{ticket: known}
	}
	return rows, nil
}

// openInBrowser returns an Effect rather than running anything, so the host shows the notice
// and runs the command on a background goroutine.
func (tracker *tracker) openInBrowser(of ticket) plugin.Effect {
	return plugin.Effect{
		Notice: "Opening " + of.ID + "…",
		Command: func() error {
			_, err := shell.Run("open", tracker.url(of))
			return err
		},
	}
}
