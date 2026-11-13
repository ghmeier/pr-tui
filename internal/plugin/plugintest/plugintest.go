// Package plugintest drives a host's model in tests, the way the Bubble Tea runtime would:
// every command is run and its message delivered, and every update is drawn. The host's models
// update in place, so the helpers do not return the model.
package plugintest

import (
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func KeyMessage(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// run runs a command and returns the messages it produced. The commands of a batch run at the
// same time, as they do in the program, and their messages come back in the batch's order.
func run(command tea.Cmd) []tea.Msg {
	if command == nil {
		return nil
	}
	message := command()
	batch, isBatch := message.(tea.BatchMsg)
	if !isBatch {
		if message == nil {
			return nil
		}
		return []tea.Msg{message}
	}
	results := make([][]tea.Msg, len(batch))
	var group sync.WaitGroup
	for index, member := range batch {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index] = run(member)
		}()
	}
	group.Wait()
	var messages []tea.Msg
	for _, result := range results {
		messages = append(messages, result...)
	}
	return messages
}

// Settle runs the command, delivers its messages to the model, and does the same for every
// command that follows, until nothing is left to run. A command that blocks, such as a
// timer, blocks the test, so the model's refresh timer is never started here.
func Settle(t *testing.T, model tea.Model, command tea.Cmd) {
	t.Helper()
	for _, message := range run(command) {
		_, next := model.Update(message)
		model.View()
		Settle(t, model, next)
	}
}

// Start loads every board and waits for the column data to arrive.
func Start(t *testing.T, model interface {
	tea.Model
	RefreshAll() tea.Cmd
}) {
	t.Helper()
	Settle(t, model, model.RefreshAll())
}

// Send delivers one message and settles what follows.
func Send(t *testing.T, model tea.Model, message tea.Msg) {
	t.Helper()
	_, command := model.Update(message)
	model.View()
	Settle(t, model, command)
}

// Press presses the keys in order, settling the commands after each.
func Press(t *testing.T, model tea.Model, keys ...string) {
	t.Helper()
	for _, key := range keys {
		Send(t, model, KeyMessage(key))
	}
}

func Resize(t *testing.T, model tea.Model, width, height int) {
	t.Helper()
	Send(t, model, tea.WindowSizeMsg{Width: width, Height: height})
}

// Quits reports whether pressing the key asks the program to quit.
func Quits(model tea.Model, key string) bool {
	_, command := model.Update(KeyMessage(key))
	if command == nil {
		return false
	}
	_, quit := command().(tea.QuitMsg)
	return quit
}
