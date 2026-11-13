package pullrequests

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/ui"
)

const (
	columnGap    = "  "
	titleColumn  = 2
	minimumTitle = 20
)

var columnHeaders = []string{"Repo", "#", "Title", "Author", "Lines", "State", "Review requested", "Updated"}

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
// viewer, merge queue and auto-merge, then CI.
func (pr PullRequest) stateMarks(viewer, reviewTeam string) string {
	var marks []string
	switch {
	case pr.IsDraft:
		marks = append(marks, "📝")
	case pr.ReviewDecision == ReviewDecisionApproved:
		marks = append(marks, "✅")
	case pr.ReviewDecision == ReviewDecisionChangesRequested:
		marks = append(marks, "❌")
	}
	if pr.awaitsReviewFrom(viewer, reviewTeam) {
		marks = append(marks, "✋")
	}
	if pr.InMergeQueue {
		marks = append(marks, "♻️")
	}
	if pr.AutoMerge {
		marks = append(marks, "⏩")
	}
	switch {
	case pr.CIState == nil:
		marks = append(marks, "…")
	case *pr.CIState == CIStatePassing:
		marks = append(marks, "🟢")
	case *pr.CIState == CIStateFailing:
		marks = append(marks, "🔴")
	case *pr.CIState == CIStateRunning:
		marks = append(marks, "🟡")
	}
	return strings.Join(marks, " ")
}

func (pr PullRequest) reviewSegments(viewer, reviewTeam string) []ui.Segment {
	var reviewers []ui.Segment
	if slices.Contains(pr.RequestedUsers, viewer) {
		reviewers = append(reviewers, ui.Segment{Text: "you", Style: ui.YouStyle})
	}
	if pr.requestsTeam(reviewTeam) {
		reviewers = append(reviewers, ui.Segment{Text: reviewTeam, Style: ui.TeamStyle})
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

func (pr PullRequest) lineCountSegments() []ui.Segment {
	if pr.LineCount == nil {
		return []ui.Segment{{Text: "…", Style: ui.DimStyle}}
	}
	return lineCountSegments(pr.LineCount.Additions, pr.LineCount.Deletions)
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

func (pr PullRequest) columns(viewer, reviewTeam string) [][]ui.Segment {
	authorStyle := ui.PlainStyle
	if pr.IsBot {
		authorStyle = ui.DimStyle
	}
	return [][]ui.Segment{
		{{Text: pr.Repository.Name, Style: ui.PlainStyle}},
		{{Text: strconv.Itoa(pr.Number), Style: ui.PlainStyle}},
		{{Text: pr.Title, Style: ui.PlainStyle}},
		{{Text: pr.Author, Style: authorStyle}},
		pr.lineCountSegments(),
		{{Text: pr.stateMarks(viewer, reviewTeam), Style: ui.PlainStyle}},
		pr.reviewSegments(viewer, reviewTeam),
		{{Text: pr.age(), Style: ui.PlainStyle}},
	}
}

// renderTable draws the header and the rows from offset that fit in rowCount lines.
// Pass selected = -1 to draw no cursor.
func renderTable(pullRequests []PullRequest, viewer, reviewTeam string, width, selected, offset, rowCount int) string {
	widths := make([]int, len(columnHeaders))
	for index, header := range columnHeaders {
		widths[index] = lipgloss.Width(header)
	}
	rows := make([][][]ui.Segment, len(pullRequests))
	for rowIndex, pr := range pullRequests {
		rows[rowIndex] = pr.columns(viewer, reviewTeam)
		for index, column := range rows[rowIndex] {
			widths[index] = max(widths[index], lipgloss.Width(ui.PlainText(column)))
		}
	}
	widthOfOtherColumns := len(columnGap) * (len(widths) - 1)
	for index, columnWidth := range widths {
		if index != titleColumn {
			widthOfOtherColumns += columnWidth
		}
	}
	widths[titleColumn] = max(minimumTitle, min(widths[titleColumn], width-widthOfOtherColumns))

	headerCells := make([]string, len(columnHeaders))
	for index, header := range columnHeaders {
		headerCells[index] = ui.Fit(header, widths[index])
	}
	lines := []string{ui.BoldStyle.Render(ansi.Truncate(strings.Join(headerCells, columnGap), width, "…"))}

	end := min(len(rows), offset+max(rowCount, 0))
	for rowIndex := offset; rowIndex < end; rowIndex++ {
		cells := make([]string, len(widths))
		for index, column := range rows[rowIndex] {
			if rowIndex == selected {
				cells[index] = ui.Fit(ui.PlainText(column), widths[index])
			} else {
				cells[index] = ui.Fit(ui.StyledText(column), widths[index])
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
