package agentsessions

import (
	"slices"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// columnHeader is how the message action finds the column's data on a selection.
const columnHeader = "Session"

// matcher says whether a session is working on one row.
type matcher func(session Session) bool

// matchedSessions are the sessions that work on one row, most recently active first.
type matchedSessions []Session

// sessionMark is what a row shows when at least one session works on it.
const sessionMark = "🤖"

// sessionColumn is the Session column of a board, drawn within the board's column named by
// within. A cell stays empty until the fetch has delivered sessions, and for a row that no
// session matches. matchersFor runs on the update
// loop, so it only reads the rows, and returns a matcher for each row key the column applies to.
func sessionColumn(provider Provider, within string, matchersFor func(rows []plugin.Row) map[string]matcher) plugin.Column {
	return plugin.Column{
		Header: columnHeader,
		Within: within,
		Cell: func(_ plugin.Row, data any) []ui.Segment {
			sessions, loaded := data.(matchedSessions)
			if !loaded || len(sessions) == 0 {
				return nil
			}
			return ui.PlainSegments(sessionMark)
		},
		Load: func(rows []plugin.Row) []plugin.Fetch {
			matchers := matchersFor(rows)
			if len(matchers) == 0 {
				return nil
			}
			return []plugin.Fetch{func() (map[string]any, error) { return matchingSessions(provider, matchers) }}
		},
	}
}

// matchingSessions lists the sessions once and delivers, for each row key, the ones that match.
func matchingSessions(provider Provider, matchers map[string]matcher) (map[string]any, error) {
	sessions, err := provider.Sessions()
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(sessions, func(first, second Session) int { return second.LastActive.Compare(first.LastActive) })
	matched := map[string]any{}
	for key, matches := range matchers {
		var found matchedSessions
		for _, session := range sessions {
			if matches(session) {
				found = append(found, session)
			}
		}
		if len(found) > 0 {
			matched[key] = found
		}
	}
	return matched, nil
}

// statusLabel is what a cell says about a session that is not idle, or empty.
func statusLabel(session Session) string {
	if session.Status == StatusIdle || session.Status == StatusUnknown {
		return ""
	}
	return string(session.Status)
}
