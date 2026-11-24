package plugin

import (
	"cmp"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// RunEditor hands the terminal to an editor on the file at path, and delivers finished's message
// once the editor exits. Tests replace it, because they must never hand the terminal to an editor.
var RunEditor = func(path string, finished func(error) tea.Msg) tea.Cmd {
	editor := strings.Fields(cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"))
	return tea.ExecProcess(exec.Command(editor[0], append(editor[1:], path)...), finished)
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
	return RunEditor(file.Name(), func(err error) tea.Msg {
		return editFinishedMessage{board: index, edit: edit, path: file.Name(), err: err}
	})
}

// finishEdit reads back what the editor saved and hands it to the edit's Done. The editor
// switched off the terminal's mouse reporting on the way out and nothing switches it back on, so
// it is turned on again whatever happened.
func (m *Model) finishEdit(message editFinishedMessage) tea.Cmd {
	saved, err := os.ReadFile(message.path)
	os.Remove(message.path)
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
