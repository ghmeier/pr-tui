package pullrequests

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

func (event ReviewEvent) label() string {
	switch event {
	case ReviewEventRequestChanges:
		return "request changes"
	case ReviewEventComment:
		return "comment"
	}
	return "approve"
}

var reviewEvents = []ReviewEvent{ReviewEventApprove, ReviewEventRequestChanges, ReviewEventComment}

// review starts a review of the pull request. GitHub refuses approving or requesting changes
// on one's own pull request, so there the only choice is a comment review and none is asked.
func (board *pullRequestBoard) review(selection plugin.Selection) plugin.Effect {
	row := selection.Row.(Row)
	pr := row.PullRequest
	if pr.Author == row.Viewer {
		return board.writeReviewBody(selection, ReviewEventComment)
	}
	options := make([]string, len(reviewEvents))
	for index, event := range reviewEvents {
		options[index] = event.label()
	}
	return plugin.Effect{Choice: &plugin.Choice{
		Question: "Review " + pr.label(),
		Options:  options,
		Pick: func(option string) plugin.Effect {
			for _, event := range reviewEvents {
				if event.label() == option {
					return board.writeReviewBody(selection, event)
				}
			}
			return plugin.Effect{}
		},
	}}
}

// writeReviewBody opens the editor on the body written last time, which is kept until a review
// is submitted so that declining or a failure does not lose it.
func (board *pullRequestBoard) writeReviewBody(selection plugin.Selection, event ReviewEvent) plugin.Effect {
	pr := pullRequestOf(selection)
	return plugin.Effect{Edit: &plugin.Edit{
		Text: board.reviews.body(pr.URL),
		Done: func(body string) tea.Cmd {
			board.reviews.setBody(pr.URL, body)
			review := Review{Event: event, Body: body, Comments: board.reviews.drafts(pr.URL)}
			if refusal := review.refusal(); refusal != "" {
				return selection.Run(plugin.Effect{Notice: refusal})
			}
			return selection.Run(board.submitReview(selection, review))
		},
	}}
}

// refusal says why GitHub would reject the review, or is empty when it would accept it.
func (review Review) refusal() string {
	switch {
	case review.Event == ReviewEventRequestChanges && review.Body == "":
		return "A review that requests changes needs a body."
	case review.Event == ReviewEventComment && review.Body == "" && len(review.Comments) == 0:
		return "A comment review needs a body or pending comments."
	}
	return ""
}

// submitReview takes the review as it was when the body was written. Only the drafts that were
// sent are removed on success, because the reviewer can add more while the submission runs.
func (board *pullRequestBoard) submitReview(selection plugin.Selection, review Review) plugin.Effect {
	pr := pullRequestOf(selection)
	return plugin.Effect{
		Confirm: review.question(pr),
		Notice:  "Submitting review of " + pr.label() + "…",
		Command: func() error {
			if err := board.provider.SubmitReview(pr, review); err != nil {
				return err
			}
			board.reviews.submitted(pr.URL, review.Comments)
			return nil
		},
		RefreshRow: selection.Row,
	}
}

func (review Review) question(pr PullRequest) string {
	question := "Submit review of " + pr.label() + ": " + review.Event.label()
	switch len(review.Comments) {
	case 0:
	case 1:
		question += ", with 1 pending comment"
	default:
		question += fmt.Sprintf(", with %d pending comments", len(review.Comments))
	}
	return question + "?"
}
