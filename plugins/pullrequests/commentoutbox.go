package pullrequests

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

const (
	postCommentOption  = "post to pull request"
	addToReviewOption  = "add to review"
	commentKeptMessage = "Comment kept; c to write it again."
)

// commentOutbox is what the screens that write comments share: the pull request and the
// selection they were opened from, the notice they show, and the comments that were written but
// not yet sent.
type commentOutbox struct {
	board       *pullRequestBoard
	pullRequest PullRequest
	// selection is what the screen was opened from, and how it hands effects to the board.
	selection plugin.Selection
	notice    string
	// keptComments holds the text of each comment that was written but neither sent nor added to
	// the review, such as one that failed to post or whose choice was cancelled, so that the next
	// comment on the same target starts from it.
	keptComments map[CommentTarget]string
	// awaitingOffer is the comment that was handed to another plugin's action and has not yet had
	// its command finish. The text stays in keptComments until then, so that a send that fails or
	// a choice that is cancelled does not lose it.
	awaitingOffer *awaitedOffer
}

func newCommentOutbox(board *pullRequestBoard, selection plugin.Selection) commentOutbox {
	return commentOutbox{
		board: board, pullRequest: pullRequestOf(selection), selection: selection, keptComments: map[CommentTarget]string{},
	}
}

func (outbox *commentOutbox) editComment(text string, done func(text string) tea.Cmd) tea.Cmd {
	// A new comment means the last hand-off was cancelled, since its command never ran.
	outbox.awaitingOffer = nil
	return outbox.selection.Run(plugin.Effect{Edit: &plugin.Edit{Text: text, Done: done}})
}

// askWhere asks what to do with a comment that was just written, and keeps its text until it is
// sent or added to the review, so that cancelling does not lose it. A comment that cannot join a
// review, because addToReview is nil, is posted at once when no other plugin offers a place for
// it, because posting is then the only choice.
func (outbox *commentOutbox) askWhere(target CommentTarget, body string, addToReview func(CommentTarget, string)) tea.Cmd {
	offered := outbox.selection.Offers(CommentKind)
	if addToReview == nil && len(offered) == 0 {
		return outbox.postComment(target, body)
	}
	outbox.keptComments[target] = body
	options := []string{postCommentOption}
	if addToReview != nil {
		options = append(options, addToReviewOption)
	}
	options = append(options, offered...)
	return outbox.selection.Run(plugin.Effect{Choice: &plugin.Choice{
		Question: "Where should this comment go?",
		Options:  options,
		Pick: func(option string) plugin.Effect {
			switch option {
			case postCommentOption:
				return plugin.Effect{Then: outbox.postComment(target, body)}
			case addToReviewOption:
				delete(outbox.keptComments, target)
				addToReview(target, body)
				return plugin.Effect{}
			}
			outbox.awaitingOffer = &awaitedOffer{target: target, option: option}
			outbox.notice = "Handing to " + option + "…"
			comment := Comment{PullRequest: outbox.pullRequest, Target: target, Body: body}
			return plugin.Effect{Then: outbox.selection.RunOffer(option, comment)}
		},
		Cancel: func() plugin.Effect {
			outbox.notice = commentKeptMessage
			return plugin.Effect{}
		},
	}})
}

// awaitedOffer names the comment and the offered action it was handed to.
type awaitedOffer struct {
	target CommentTarget
	option string
}

// finishOffer drops the kept text of a comment handed to another plugin once that plugin's
// command has succeeded, and keeps it for another try when the command failed.
func (outbox *commentOutbox) finishOffer(commandErr error) {
	offer := outbox.awaitingOffer
	if offer == nil {
		return
	}
	outbox.awaitingOffer = nil
	if commandErr != nil {
		outbox.notice = "Failed: " + ui.FirstLine(commandErr) + " · press c to try again"
		return
	}
	delete(outbox.keptComments, offer.target)
	outbox.notice = "Sent with " + offer.option + "."
}

func (outbox *commentOutbox) postComment(target CommentTarget, body string) tea.Cmd {
	delete(outbox.keptComments, target)
	if target.ReplyAuthor != "" {
		outbox.notice = "Posting reply…"
	} else {
		outbox.notice = "Posting comment…"
	}
	return outbox.board.sendComment(outbox.pullRequest, target, body)
}

// finishPost reports whether the comment was posted. One that failed is kept for another try.
func (outbox *commentOutbox) finishPost(message commentPostedMessage) bool {
	if message.err != nil {
		outbox.keptComments[message.target] = message.body
		outbox.notice = "Failed: " + ui.FirstLine(message.err) + " · press c to try again"
		return false
	}
	delete(outbox.keptComments, message.target)
	return true
}
