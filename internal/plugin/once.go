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
	for index, board := range model.boards {
		rows, err := board.spec.Load()
		if err != nil {
			if len(model.boards) > 1 {
				return fmt.Errorf("%s: %w", board.spec.Title, err)
			}
			return err
		}
		board.loadGeneration++
		commands := model.finishLoad(index, rows, board.loadGeneration)
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
			fmt.Fprintln(out, ui.BoldStyle.Render(board.spec.Title))
		}
		fmt.Fprintln(out, facetSummary(board.facets))
		fmt.Fprintln(out, board.table(width, -1, 0, len(board.visible)))
	}
	return nil
}
