package agentsessions

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

// messageAction writes a message in the editor and sends it to a session that works on the row.
// The subject names what a row is, for the notice when no session works on it, and prefill is
// the text the editor opens with.
func messageAction(provider Provider, subject string, prefill func(row plugin.Row) string) plugin.Action {
	return plugin.Action{
		Key:   "c",
		Label: "message session",
		Run: func(selection plugin.Selection) plugin.Effect {
			sessions, _ := selection.Data(columnHeader).(matchedSessions)
			if len(sessions) == 0 {
				return plugin.Effect{Notice: "No live Claude session mentions this " + subject}
			}
			opening := prefill(selection.Row)
			if len(sessions) == 1 {
				return writeMessage(provider, selection, sessions[0], opening)
			}
			options := make([]string, len(sessions))
			for index, session := range sessions {
				options[index] = sessionOption(session)
			}
			return plugin.Effect{Choice: &plugin.Choice{
				Question: "Message which session?",
				Options:  options,
				Pick: func(option string) plugin.Effect {
					for index, candidate := range options {
						if candidate == option {
							return writeMessage(provider, selection, sessions[index], opening)
						}
					}
					return plugin.Effect{}
				},
			}}
		},
	}
}

// sessionOption is a session's name, with its status when it is not idle.
func sessionOption(session Session) string {
	if label := statusLabel(session); label != "" {
		return session.Name + " " + label
	}
	return session.Name
}

// writeMessage opens the editor on the opening text. Saving it unchanged, or empty, sends nothing.
func writeMessage(provider Provider, selection plugin.Selection, session Session, opening string) plugin.Effect {
	return plugin.Effect{Edit: &plugin.Edit{
		Text: opening,
		Done: func(text string) tea.Cmd {
			if text == "" || text == strings.TrimSpace(opening) {
				return selection.Run(plugin.Effect{Notice: "Nothing sent"})
			}
			return selection.Run(plugin.Effect{
				Notice:  "Sending to " + session.Name + "…",
				Command: func() error { return provider.Send(session, text) },
			})
		},
	}}
}
