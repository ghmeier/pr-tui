package pullrequests

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

var (
	viewerStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	reviewTeamStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
)

// The headers are also how other plugins ask for a column's loaded data through
// Selection.Data.
const (
	linesHeader = "Lines"
	stateHeader = "State"
)

// awaitsReviewFrom reports whether someone else's pull request has a review requested from
// the viewer or from the review team. Without a review team, only the viewer counts.
func (pr PullRequest) awaitsReviewFrom(viewer, reviewTeam string) bool {
	if pr.Author == viewer {
		return false
	}
	return slices.Contains(pr.RequestedUsers, viewer) || pr.requestsTeam(reviewTeam)
}

// requestsTeam is false for an empty reviewTeam, which means no team is configured.
func (pr PullRequest) requestsTeam(reviewTeam string) bool {
	return reviewTeam != "" && slices.Contains(pr.RequestedTeams, reviewTeam)
}

// stateMarks summarises the pull request as emoji: review state, whether it waits on the
// viewer, merge queue and auto-merge, then CI. The data is the loaded CIState, or nil while
// it is still loading.
func (row Row) stateMarks(reviewTeam string, data any) string {
	pr := row.PullRequest
	var marks []string
	switch {
	case pr.IsDraft:
		marks = append(marks, "📝")
	case pr.ReviewDecision == ReviewDecisionApproved:
		marks = append(marks, "✅")
	case pr.ReviewDecision == ReviewDecisionChangesRequested:
		marks = append(marks, "❌")
	}
	if pr.awaitsReviewFrom(row.Viewer, reviewTeam) {
		marks = append(marks, "✋")
	}
	if pr.InMergeQueue {
		marks = append(marks, "♻️")
	}
	if pr.AutoMerge {
		marks = append(marks, "⏩")
	}
	state, loaded := data.(CIState)
	switch {
	case !loaded:
		marks = append(marks, "…")
	case state == CIStatePassing:
		marks = append(marks, "🟢")
	case state == CIStateFailing:
		marks = append(marks, "🔴")
	case state == CIStateRunning:
		marks = append(marks, "🟡")
	}
	return strings.Join(marks, " ")
}

func (row Row) reviewSegments(reviewTeam string) []ui.Segment {
	pr, viewer := row.PullRequest, row.Viewer
	var reviewers []ui.Segment
	if slices.Contains(pr.RequestedUsers, viewer) {
		reviewers = append(reviewers, ui.Segment{Text: "you", Style: viewerStyle})
	}
	if pr.requestsTeam(reviewTeam) {
		reviewers = append(reviewers, ui.Segment{Text: reviewTeam, Style: reviewTeamStyle})
	}
	for _, login := range pr.RequestedUsers {
		if login != viewer {
			reviewers = append(reviewers, ui.Segment{Text: login, Style: ui.DimStyle})
		}
	}
	for _, slug := range pr.RequestedTeams {
		if slug != reviewTeam {
			reviewers = append(reviewers, ui.Segment{Text: slug, Style: ui.DimStyle})
		}
	}
	var segments []ui.Segment
	for index, reviewer := range reviewers {
		if index > 0 {
			segments = append(segments, ui.Segment{Text: ", ", Style: ui.PlainStyle})
		}
		segments = append(segments, reviewer)
	}
	return segments
}

// lineCountCellSegments draws the loaded LineCount, or a placeholder while it is loading.
func lineCountCellSegments(data any) []ui.Segment {
	count, loaded := data.(LineCount)
	if !loaded {
		return []ui.Segment{{Text: "…", Style: ui.DimStyle}}
	}
	return lineCountSegments(count.Additions, count.Deletions)
}

func lineCountSegments(additions, deletions int) []ui.Segment {
	return []ui.Segment{
		{Text: fmt.Sprintf("+%d", additions), Style: ui.GreenStyle},
		{Text: " ", Style: ui.PlainStyle},
		{Text: fmt.Sprintf("−%d", deletions), Style: ui.RedStyle},
	}
}

func (pr PullRequest) age() string {
	return ui.AgeText(pr.UpdatedAt)
}

// columns are the board's own columns. Lines and State load slowly, so each fetches one
// repository at a time and the table shows the pull requests without waiting for them.
func (board *pullRequestBoard) columns() []plugin.Column {
	return []plugin.Column{
		{Header: "Repo", Cell: func(row plugin.Row, _ any) []ui.Segment {
			return ui.PlainSegments(row.(Row).PullRequest.Repository.Name)
		}},
		{Header: "#", Cell: func(row plugin.Row, _ any) []ui.Segment {
			return ui.PlainSegments(strconv.Itoa(row.(Row).PullRequest.Number))
		}},
		{Header: "Title", Flexible: true, Cell: func(row plugin.Row, _ any) []ui.Segment {
			return ui.PlainSegments(row.(Row).PullRequest.Title)
		}},
		{Header: "Author", Cell: func(row plugin.Row, _ any) []ui.Segment {
			pr := row.(Row).PullRequest
			if pr.IsBot {
				return []ui.Segment{{Text: pr.Author, Style: ui.DimStyle}}
			}
			return ui.PlainSegments(pr.Author)
		}},
		{Header: linesHeader, Cell: func(_ plugin.Row, data any) []ui.Segment {
			return lineCountCellSegments(data)
		}, Load: board.loadLineCounts},
		{Header: stateHeader, Cell: func(row plugin.Row, data any) []ui.Segment {
			return ui.PlainSegments(row.(Row).stateMarks(board.reviewTeam, data))
		}, Load: board.loadCIStates},
		{Header: "Review requested", Cell: func(row plugin.Row, _ any) []ui.Segment {
			return row.(Row).reviewSegments(board.reviewTeam)
		}},
		{Header: "Updated", Cell: func(row plugin.Row, _ any) []ui.Segment {
			return ui.PlainSegments(row.(Row).PullRequest.age())
		}},
	}
}

func (board *pullRequestBoard) loadLineCounts([]plugin.Row) []plugin.Fetch {
	return fetchPerRepository(board.provider, Provider.LineCounts)
}

func (board *pullRequestBoard) loadCIStates([]plugin.Row) []plugin.Fetch {
	return fetchPerRepository(board.provider, Provider.CIStates)
}

// fetchPerRepository makes one fetch for each repository, so that a slow repository does not
// hold up the others.
func fetchPerRepository[Value any](provider Provider, byURL func(Provider, Repository) (map[string]Value, error)) []plugin.Fetch {
	var fetches []plugin.Fetch
	for _, source := range provider.Repositories() {
		fetches = append(fetches, func() (map[string]any, error) {
			found, err := byURL(provider, source)
			values := make(map[string]any, len(found))
			for url, value := range found {
				values[url] = value
			}
			return values, err
		})
	}
	return fetches
}
