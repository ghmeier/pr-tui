package plugin

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// PrintOnce prints every board, with its summary line and its full table, after the column
// fetches have finished, instead of running the interactive program.
func (host *Host) PrintOnce(out io.Writer, width int) error {
	model := host.Model()
	model.width = width
	for index, b := range model.boards {
		rows, err := b.spec.Load()
		if err != nil {
			if len(model.boards) > 1 {
				return fmt.Errorf("%s: %w", b.spec.Title, err)
			}
			return err
		}
		commands := model.finishLoad(index, rows)
		messages := make(chan tea.Msg, len(commands))
		for _, command := range commands {
			go func() { messages <- command() }()
		}
		for range commands {
			model.update(<-messages)
		}

		if len(model.boards) > 1 {
			if index > 0 {
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out, ui.BoldStyle.Render(b.spec.Title))
		}
		fmt.Fprintln(out, facetSummary(b.facets))
		fmt.Fprintln(out, b.table(width, -1, 0, len(b.visible)))
	}
	return nil
}
