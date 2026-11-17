package agentplans

import (
	"errors"
	"strings"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/shell"
)

// statusKey changes the status of a document, on the board and on the document screen.
const statusKey = "s"

// chooseStatus asks which of the statuses its type allows the document should have.
func (settings settings) chooseStatus(row Row) plugin.Effect {
	if len(row.Statuses) == 0 {
		return plugin.Effect{Notice: row.Doc.Type + " docs have no statuses to choose from."}
	}
	return plugin.Effect{Choice: &plugin.Choice{
		Question: "Status of " + row.Doc.File,
		Options:  row.Statuses,
		Current:  row.Doc.Status,
		Pick:     func(status string) plugin.Effect { return settings.setStatus(row, status) },
	}}
}

// setStatus runs apl set in the checkout that holds the document, and reloads the board.
func (settings settings) setStatus(row Row, status string) plugin.Effect {
	if status == row.Doc.Status {
		return plugin.Effect{Notice: row.Doc.File + " already has the status " + status + "."}
	}
	documentCheckout, found := settings.checkoutNamed(row.Repository)
	if !found {
		return plugin.Effect{Notice: "Cannot change the status: " + row.Repository + " is not a configured repository."}
	}
	return plugin.Effect{
		Notice: "Setting " + row.Doc.File + " to " + status + "…",
		Command: func() error {
			_, err := shell.RunIn(documentCheckout.path, "apl", "set", row.Doc.File, "--status", status)
			if err != nil && strings.Contains(err.Error(), "unknown command") {
				return errors.New("changing a status needs apl 0.9.0 or later")
			}
			return err
		},
		Refresh: true,
	}
}
