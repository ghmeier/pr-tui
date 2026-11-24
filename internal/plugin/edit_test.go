package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// editorSession is a replaced editor, and what it was opened with.
type editorSession struct {
	opened    []string
	paths     []string
	directory string
}

// editorSaves replaces the editor with one that saves the given text, or fails with the given
// error. Temporary files go to a directory that the test removes.
func editorSaves(t *testing.T, text string, failure error) *editorSession {
	t.Helper()
	session := &editorSession{directory: t.TempDir()}
	t.Setenv("TMPDIR", session.directory)
	restore := ReplaceEditorRunner(func(path string, finished func(error) tea.Msg) tea.Cmd {
		return func() tea.Msg {
			opened, _ := os.ReadFile(path)
			session.opened = append(session.opened, string(opened))
			session.paths = append(session.paths, path)
			if failure == nil {
				os.WriteFile(path, []byte(text), 0o600)
			}
			return finished(failure)
		}
	})
	t.Cleanup(restore)
	return session
}

func (session *editorSession) leftFiles(t *testing.T) []string {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(session.directory, "*"))
	if err != nil {
		t.Fatal(err)
	}
	return left
}

// editBoard is a board whose action on "x" asks for an edit of the given text.
func editBoard(edit *Edit) Board {
	return boardWithEffect(func(fakeRow) Effect { return Effect{Edit: edit} })
}

// flatten runs a command and everything a batch in it contains, and returns the messages.
func flatten(command tea.Cmd) []tea.Msg {
	if command == nil {
		return nil
	}
	message := command()
	batch, isBatch := message.(tea.BatchMsg)
	if !isBatch {
		return []tea.Msg{message}
	}
	var messages []tea.Msg
	for _, member := range batch {
		messages = append(messages, flatten(member)...)
	}
	return messages
}

func TestEditOpensTheEditorWithTheTextAndHandsDoneTheSavedTextTrimmed(t *testing.T) {
	session := editorSaves(t, "\n  saved text  \n\n", nil)
	var received []string
	model := modelForBoard(t, editBoard(&Edit{Text: "original text", Done: func(text string) tea.Cmd {
		received = append(received, text)
		return nil
	}}))

	plugintest.Press(t, model, "x")

	if len(session.opened) != 1 || session.opened[0] != "original text" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
	if len(received) != 1 || received[0] != "saved text" {
		t.Fatalf("Done received %q", received)
	}
}

func TestEditRemovesItsTemporaryFileAfterDoneRuns(t *testing.T) {
	session := editorSaves(t, "saved", nil)
	var leftWhileDoneRuns []string
	model := modelForBoard(t, editBoard(&Edit{Text: "original", Done: func(string) tea.Cmd {
		leftWhileDoneRuns = session.leftFiles(t)
		return nil
	}}))

	plugintest.Press(t, model, "x")

	if len(session.paths) != 1 || !strings.HasPrefix(filepath.Base(session.paths[0]), "prs-edit-") {
		t.Fatalf("the editor was given %q", session.paths)
	}
	if left := session.leftFiles(t); len(left) != 0 {
		t.Fatalf("files left behind: %v", left)
	}
	if len(leftWhileDoneRuns) != 0 {
		t.Fatalf("files left while Done ran: %v", leftWhileDoneRuns)
	}
}

func TestEditThatFailsShowsEditorFailedAndDoesNotCallDone(t *testing.T) {
	session := editorSaves(t, "", errorWithTwoLines)
	called := false
	model := modelForBoard(t, editBoard(&Edit{Text: "original", Done: func(string) tea.Cmd {
		called = true
		return nil
	}}))

	plugintest.Press(t, model, "x")

	if status := statusLine(model); !strings.Contains(status, "Editor failed: first line") || strings.Contains(status, "second line") {
		t.Fatalf("status = %q", status)
	}
	if called {
		t.Fatalf("Done was called")
	}
	if left := session.leftFiles(t); len(left) != 0 {
		t.Fatalf("files left behind: %v", left)
	}
}

func TestEditThatCannotWriteItsTemporaryFileShowsEditorFailedAndOpensNoEditor(t *testing.T) {
	session := editorSaves(t, "saved", nil)
	t.Setenv("TMPDIR", filepath.Join(session.directory, "missing"))
	model := modelForBoard(t, editBoard(&Edit{Text: "original", Done: func(string) tea.Cmd { return nil }}))

	plugintest.Press(t, model, "x")

	if !strings.Contains(statusLine(model), "Editor failed: ") || len(session.opened) != 0 {
		t.Fatalf("status = %q, editor opened %d times", statusLine(model), len(session.opened))
	}
}

func TestEditTurnsMouseReportingBackOnAfterTheEditorReturns(t *testing.T) {
	for name, failure := range map[string]error{"when it saves": nil, "when it fails": errorForTests} {
		t.Run(name, func(t *testing.T) {
			editorSaves(t, "saved", failure)
			model := modelForBoard(t, editBoard(&Edit{Text: "original", Done: func(string) tea.Cmd { return nil }}))
			_, opening := model.Update(plugintest.KeyMessage("x"))
			finished := opening()

			_, afterwards := model.Update(finished)

			if !containsMessage(flatten(afterwards), tea.EnableMouseCellMotion()) {
				t.Fatalf("messages after the editor returned: %v", flatten(afterwards))
			}
		})
	}
}

func containsMessage(messages []tea.Msg, expected tea.Msg) bool {
	for _, message := range messages {
		if message == expected {
			return true
		}
	}
	return false
}

func TestEditDoneCommandRuns(t *testing.T) {
	editorSaves(t, "saved", nil)
	count := &counter{}
	model := modelForBoard(t, editBoard(&Edit{Text: "original", Done: func(string) tea.Cmd {
		return func() tea.Msg {
			count.command()
			return nil
		}
	}}))

	plugintest.Press(t, model, "x")

	if count.runs != 1 {
		t.Fatalf("Done's command ran %d times", count.runs)
	}
}

// editingScreen is a screen that asks the host for an edit when any key is pressed.
type editingScreen struct {
	selection Selection
	received  *[]string
}

func (screen editingScreen) Init() tea.Cmd { return nil }

func (screen editingScreen) Update(message tea.Msg) (Screen, tea.Cmd) {
	if _, isKey := message.(tea.KeyMsg); !isKey {
		return screen, nil
	}
	return screen, screen.selection.Run(Effect{Edit: &Edit{Text: "from the screen", Done: func(text string) tea.Cmd {
		*screen.received = append(*screen.received, text)
		return nil
	}}})
}

func (screen editingScreen) View(width, height int) string { return "editing screen" }

func TestEditRunFromAScreenHandsDoneTheSavedText(t *testing.T) {
	session := editorSaves(t, "saved from the screen", nil)
	var received []string
	board := boardWithEffect(func(fakeRow) Effect { return Effect{} })
	board.Actions = []Action{{Key: "x", Run: func(selection Selection) Effect {
		return Effect{Screen: editingScreen{selection: selection, received: &received}}
	}}}
	model := modelForBoard(t, board)

	plugintest.Press(t, model, "x", "e")

	if len(session.opened) != 1 || session.opened[0] != "from the screen" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
	if len(received) != 1 || received[0] != "saved from the screen" {
		t.Fatalf("Done received %q", received)
	}
}

func TestEditAfterAConfirmationOpensTheEditorOnlyOnceItIsAccepted(t *testing.T) {
	session := editorSaves(t, "saved", nil)
	model := modelForBoard(t, boardWithEffect(func(fakeRow) Effect {
		return Effect{Confirm: "Edit it?", Edit: &Edit{Text: "original"}}
	}))

	plugintest.Press(t, model, "x")
	askedBefore := len(session.opened)
	plugintest.Press(t, model, "y")

	if askedBefore != 0 || len(session.opened) != 1 {
		t.Fatalf("the editor was opened %d times before the answer and %d after", askedBefore, len(session.opened))
	}
	if strings.Contains(statusLine(model), "Working…") {
		t.Fatalf("status = %q", statusLine(model))
	}
}
