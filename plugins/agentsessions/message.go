package agentsessions

import (
	"slices"
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
			sessions := sessionsOf(selection)
			if len(sessions) == 0 {
				return plugin.Effect{Notice: "No live agent session mentions this " + subject}
			}
			opening := prefill(selection.Row)
			return withSession(sessions, "Message which session?", func(session Session) plugin.Effect {
				return writeMessage(provider, selection, session, opening)
			})
		},
	}
}

// sessionsOf are the live sessions the column found for the selection's row.
func sessionsOf(selection plugin.Selection) matchedSessions {
	sessions, _ := selection.Data(columnHeader).(matchedSessions)
	return sessions
}

// withSession carries on with the only session, or asks which of the sessions to use first.
func withSession(sessions matchedSessions, question string, next func(session Session) plugin.Effect) plugin.Effect {
	if len(sessions) == 1 {
		return next(sessions[0])
	}
	options := make([]string, len(sessions))
	for index, session := range sessions {
		options[index] = sessionOption(session)
	}
	return plugin.Effect{Choice: &plugin.Choice{
		Question: question,
		Options:  options,
		Cancel:   func() plugin.Effect { return plugin.Effect{Notice: "Nothing sent"} },
		Pick: func(option string) plugin.Effect {
			if index := slices.Index(options, option); index >= 0 {
				return next(sessions[index])
			}
			return plugin.Effect{}
		},
	}}
}

// sendMessage sends the text to the session in the background.
func sendMessage(provider Provider, session Session, text string) plugin.Effect {
	return plugin.Effect{
		Notice:  "Sending to " + session.Name + "…",
		Command: func() error { return provider.Send(session, text) },
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
			return selection.Run(sendMessage(provider, session, text))
		},
	}}
}
