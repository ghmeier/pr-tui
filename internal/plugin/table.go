package plugin

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/ui"
)

const (
	columnGap            = "  "
	minimumFlexibleWidth = 20
)

// renderTable draws the header and the rows from offset that fit in rowCount lines. Pass
// selected = -1 to draw no cursor. The flexible column is the index of the column that
// shrinks to fit the width, or -1 when none does.
func renderTable(headers []string, flexibleColumn int, rows [][][]ui.Segment, width, selected, offset, rowCount int) string {
	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = lipgloss.Width(header)
	}
	for _, row := range rows {
		for index, cell := range row {
			widths[index] = max(widths[index], lipgloss.Width(ui.PlainText(cell)))
		}
	}
	if flexibleColumn >= 0 {
		widthOfOtherColumns := len(columnGap) * (len(widths) - 1)
		for index, columnWidth := range widths {
			if index != flexibleColumn {
				widthOfOtherColumns += columnWidth
			}
		}
		widths[flexibleColumn] = max(minimumFlexibleWidth, min(widths[flexibleColumn], width-widthOfOtherColumns))
	}

	headerCells := make([]string, len(headers))
	for index, header := range headers {
		headerCells[index] = ui.Fit(header, widths[index])
	}
	lines := []string{ui.BoldStyle.Render(ansi.Truncate(strings.Join(headerCells, columnGap), width, "…"))}

	end := min(len(rows), offset+max(rowCount, 0))
	for rowIndex := offset; rowIndex < end; rowIndex++ {
		cells := make([]string, len(widths))
		for index, cell := range rows[rowIndex] {
			if rowIndex == selected {
				cells[index] = ui.Fit(ui.PlainText(cell), widths[index])
			} else {
				cells[index] = ui.Fit(ui.StyledText(cell), widths[index])
			}
		}
		line := ansi.Truncate(strings.Join(cells, columnGap), width, "…")
		if rowIndex == selected {
			line = ui.SelectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
