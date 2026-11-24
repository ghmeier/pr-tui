package pullrequests

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// splitSeparatorWidth is the width of the separator between the two sides of a side-by-side row.
const splitSeparatorWidth = 3

// dragSelection is the rows of the diff that the pointer was dragged over. The terminal's own
// selection spans the whole screen while prs has the mouse, so the diff screen selects lines
// itself.
type dragSelection struct {
	// anchor is the row the button was pressed on, and end is the row the pointer is on now.
	anchor, end int
	// onOldSide is the side of a side-by-side row that the selection takes its code from.
	onOldSide bool
	// held is true while the button is still down.
	held bool
}

func (selection dragSelection) covers(row int) bool {
	return row >= min(selection.anchor, selection.end) && row <= max(selection.anchor, selection.end)
}

// handleMouse presses and drags select, and the wheel moves the line cursor.
func (screen *diffScreen) handleMouse(event tea.MouseMsg) (plugin.Screen, tea.Cmd) {
	switch {
	case event.Button == tea.MouseButtonWheelUp && event.Action == tea.MouseActionPress:
		return screen.handleKey("up")
	case event.Button == tea.MouseButtonWheelDown && event.Action == tea.MouseActionPress:
		return screen.handleKey("down")
	case event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionPress:
		screen.pressAt(event.X, event.Y)
	case event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionMotion:
		screen.dragTo(event.Y)
	case event.Action == tea.MouseActionRelease:
		return screen, screen.release()
	}
	return screen, nil
}

// bodyRow is the row of the screen's body that a line of the terminal shows, or false for the
// header and everything below the body.
func (screen *diffScreen) bodyRow(y int) (int, bool) {
	row := y - 1
	return row, row >= 0 && row < screen.diffRows()
}

// pressAt moves the cursor to the diff line that was pressed and starts a selection there, or
// shows the file that was pressed.
func (screen *diffScreen) pressAt(x, y int) {
	bodyRow, onBody := screen.bodyRow(y)
	if !onBody || screen.loading || len(screen.files) == 0 {
		return
	}
	screen.drag = nil
	if x < screen.fileListWidth() {
		if file := screen.fileOffset + bodyRow; file < len(screen.files) && file != screen.fileCursor {
			screen.notice = ""
			screen.fileCursor = file
			screen.diffCursor, screen.diffOffset = 0, 0
		}
		return
	}
	row := screen.diffOffset + bodyRow
	if row >= len(screen.rows) {
		return
	}
	screen.notice = ""
	screen.diffCursor = row
	for screen.diffCursor > 0 && screen.rows[screen.diffCursor].part > 0 {
		screen.diffCursor--
	}
	onOldSide := screen.showsSplit() && x-screen.fileListWidth()-splitSeparatorWidth < splitSideWidth(screen.diffWidth())+1
	if screen.showsSplit() {
		screen.commentOnOldSide = onOldSide
	}
	screen.drag = &dragSelection{anchor: row, end: row, onOldSide: onOldSide, held: true}
}

// dragTo extends the selection to the row at the height of the pointer, which stays within the
// rows on screen because the screen does not scroll while dragging.
func (screen *diffScreen) dragTo(y int) {
	if screen.drag == nil || !screen.drag.held {
		return
	}
	bodyRow := max(0, min(y-1, screen.diffRows()-1))
	screen.drag.end = max(0, min(screen.diffOffset+bodyRow, len(screen.rows)-1))
}

// release ends the drag and copies the selected code, unless the pointer never left the row
// it was pressed on, which is a click.
func (screen *diffScreen) release() tea.Cmd {
	if screen.drag == nil || !screen.drag.held {
		return nil
	}
	screen.drag.held = false
	if screen.drag.anchor == screen.drag.end {
		screen.drag = nil
		return nil
	}
	text := screen.selectedCode()
	if text == "" {
		return nil
	}
	return screen.selection.Run(plugin.Effect{Copy: text})
}

// selectedCode is the code of the lines under the selected rows, without their markers and line
// numbers. A line that wraps onto several rows counts once. In the side-by-side layout only the
// chosen side's lines count.
func (screen *diffScreen) selectedCode() string {
	file := screen.files[screen.fileCursor]
	var code []string
	copied := -1
	for index := min(screen.drag.anchor, screen.drag.end); index <= max(screen.drag.anchor, screen.drag.end); index++ {
		row := screen.rows[index]
		line := row.left
		if screen.showsSplit() && !screen.drag.onOldSide {
			line = row.right
		}
		if row.isComment() || line < 0 || line == copied || file.lines[line].kind < contextLine {
			continue
		}
		copied = line
		if text := file.lines[line].text; text != "" {
			code = append(code, text[1:])
		} else {
			code = append(code, "")
		}
	}
	return strings.Join(code, "\n")
}

// highlightSelected shows the row selected: its chosen side in the side-by-side layout, and
// otherwise the whole width.
func (screen *diffScreen) highlightSelected(index int, rendered string, width int) string {
	if screen.drag == nil || !screen.drag.covers(index) {
		return rendered
	}
	row := screen.rows[index]
	if !screen.showsSplit() || row.isComment() || (splitRow{row.left, row.right}).spansBothSides(screen.files[screen.fileCursor]) {
		return ui.SelectedStyle.Render(ui.Fit(ansi.Strip(rendered), width))
	}
	sideWidth := splitSideWidth(width)
	start := 0
	if !screen.drag.onOldSide {
		start = sideWidth + splitSeparatorWidth
	}
	end := start + sideWidth
	side := ui.SelectedStyle.Render(ui.Fit(ansi.Strip(ansi.Cut(rendered, start, end)), sideWidth))
	return ansi.Cut(rendered, 0, start) + side + ansi.TruncateLeft(rendered, end, "")
}
