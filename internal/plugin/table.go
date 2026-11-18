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

// columnWidths is how wide each column is drawn: as wide as its header and widest cell, up to
// its MaxWidth, with the flexible column giving up room so that the table fits the width.
func columnWidths(columns []Column, rows [][][]ui.Segment, width int) []int {
	widths := make([]int, len(columns))
	flexibleColumn := -1
	for index, column := range columns {
		widths[index] = lipgloss.Width(column.Header)
		if column.Flexible {
			flexibleColumn = index
		}
	}
	for _, row := range rows {
		for index, cell := range row {
			widths[index] = max(widths[index], lipgloss.Width(ui.PlainText(cell)))
		}
	}
	for index, column := range columns {
		if column.MaxWidth > 0 {
			widths[index] = min(widths[index], column.MaxWidth)
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
	return widths
}

// columnAt finds the column drawn at a horizontal position of the table. The gaps between
// columns and anything past the last column belong to no column.
func columnAt(widths []int, x int) (int, bool) {
	start := 0
	for index, columnWidth := range widths {
		if x >= start && x < start+columnWidth {
			return index, true
		}
		start += columnWidth + len(columnGap)
	}
	return 0, false
}

// renderTable draws the header and the rows from offset that fit in rowCount lines. Pass
// selected = -1 to draw no cursor. The columns supply the headers, the widest each may be
// drawn, and which one shrinks to fit the width.
func renderTable(columns []Column, rows [][][]ui.Segment, width, selected, offset, rowCount int) string {
	widths := columnWidths(columns, rows, width)

	headerCells := make([]string, len(columns))
	for index, column := range columns {
		headerCells[index] = ui.Fit(column.Header, widths[index])
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
