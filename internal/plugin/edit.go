package plugin

import (
	"cmp"
	"os"
	"os/exec"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// RunEditor hands the terminal to an editor on the file at path, and delivers finished's message
// once the editor exits. Tests replace it, because they must never hand the terminal to an editor.
var RunEditor = func(path string, finished func(error) tea.Msg) tea.Cmd {
	editor := strings.Fields(cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"))
	return tea.ExecProcess(exec.Command(editor[0], append(editor[1:], path)...), finished)
}

// writingNotice is shown while the configured text command runs beside prs.
const writingNotice = "Writing in your editor…"

// runTextCommand runs the configured command beside prs on a background goroutine, so the
// screen keeps updating until the command exits.
func runTextCommand(command []string, path string, finished func(error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		_, err := shell.Run(append(slices.Clone(command), path)...)
		return finished(err)
	}
}

// editFinishedMessage says the editor started for an edit has exited.
type editFinishedMessage struct {
	board int
	edit  *Edit
	path  string
	err   error
}

// startEdit writes the text to a temporary file and opens it in the editor.
func (m *Model) startEdit(index int, edit *Edit) tea.Cmd {
	file, err := os.CreateTemp("", "prs-edit-*.md")
	if err == nil {
		_, err = file.WriteString(edit.Text)
		err = cmp.Or(err, file.Close())
	}
	if err != nil {
		m.boards[index].reportFailure("Editor failed: " + ui.FirstLine(err))
		return nil
	}
	finished := func(err error) tea.Msg {
		return editFinishedMessage{board: index, edit: edit, path: file.Name(), err: err}
	}
	if len(m.textCommand) == 0 {
		return RunEditor(file.Name(), finished)
	}
	if m.boards[index].notice == "" {
		m.boards[index].notice = writingNotice
	}
	return runTextCommand(m.textCommand, file.Name(), finished)
}

// finishEdit reads back what the editor saved and hands it to the edit's Done. The editor
// switched off the terminal's mouse reporting on the way out and nothing switches it back on, so
// it is turned on again whatever happened.
func (m *Model) finishEdit(message editFinishedMessage) tea.Cmd {
	saved, err := os.ReadFile(message.path)
	os.Remove(message.path)
	if board := m.boards[message.board]; board.notice == writingNotice {
		board.notice = ""
	}
	if err = cmp.Or(message.err, err); err != nil {
		m.boards[message.board].reportFailure("Editor failed: " + ui.FirstLine(err))
		return tea.EnableMouseCellMotion
	}
	var next tea.Cmd
	if message.edit.Done != nil {
		next = message.edit.Done(strings.TrimSpace(string(saved)))
	}
	return tea.Batch(tea.EnableMouseCellMotion, next)
}
