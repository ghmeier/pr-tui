package agentsessions

import (
	"fmt"
	"strings"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

// worksOnPullRequest is true when the session has mentioned the pull request, or is on its head
// branch in a checkout of the same repository. The repository is compared by name, because that
// is all a session knows of its checkout.
func worksOnPullRequest(session Session, pullRequest pullrequests.PullRequest) bool {
	repository := pullRequest.Repository
	for _, mentioned := range session.PullRequests {
		if mentioned.Repository == repository.Owner+"/"+repository.Name && mentioned.Number == pullRequest.Number {
			return true
		}
	}
	return session.Branch != "" && session.Branch == pullRequest.HeadBranch && session.Repository == repository.Name
}

func pullRequestColumn(provider Provider) plugin.Column {
	return sessionColumn(provider, pullrequests.StateColumn, func(rows []plugin.Row) map[string]matcher {
		matchers := map[string]matcher{}
		for _, row := range rows {
			if pullRequestRow, isPullRequest := row.(pullrequests.Row); isPullRequest {
				pullRequest := pullRequestRow.PullRequest
				matchers[row.Key()] = func(session Session) bool { return worksOnPullRequest(session, pullRequest) }
			}
		}
		return matchers
	})
}

func pullRequestMessageAction(provider Provider) plugin.Action {
	return messageAction(provider, func(row plugin.Row) string {
		return pullRequestPrefill(row.(pullrequests.Row).PullRequest)
	})
}

// commentAction sends a comment written on the diff screen to a session that works on the pull
// request. The text is already written, so no editor opens.
func commentAction(provider Provider) plugin.Action {
	return plugin.Action{
		Takes:   pullrequests.CommentKind,
		Label:   "send to agent session",
		Applies: hasSessions,
		Run: func(selection plugin.Selection) plugin.Effect {
			comment, isComment := selection.Value.(pullrequests.Comment)
			if !isComment {
				return plugin.Effect{Notice: "Nothing sent: this is not a comment"}
			}
			text := commentMessage(comment)
			return withSession(sessionsOf(selection), "Send to which session?", func(session Session) plugin.Effect {
				return sendMessage(provider, session, text)
			})
		},
	}
}

func hasSessions(selection plugin.Selection) bool {
	return len(sessionsOf(selection)) > 0
}

// commentMessage is the text sent for a comment: the pull request it is on, where in the diff it
// is, and the comment.
func commentMessage(comment pullrequests.Comment) string {
	return strings.TrimRight(pullRequestPrefill(comment.PullRequest), "\n") + "\n" + commentLines(comment.Target) + "\n\n" + comment.Body
}

// commentLines say where the comment is, which is a line of a file or the conversation, and who it answers.
func commentLines(target pullrequests.CommentTarget) string {
	if target.Conversation {
		if target.ReplyAuthor != "" {
			return "Comment on the conversation, in reply to " + target.ReplyAuthor
		}
		return "Comment on the conversation"
	}
	lines := fmt.Sprintf("File: %s (whole file)", target.Path)
	if !target.WholeFile {
		side := "new"
		if target.Side == pullrequests.CommentSideOld {
			side = "old"
		}
		lines = fmt.Sprintf("File: %s, line %d (%s side)", target.Path, target.Line, side)
	}
	if target.ReplyTo != 0 {
		lines += fmt.Sprintf("\nIn reply to %s's comment", target.ReplyAuthor)
	}
	return lines
}

// pullRequestPrefill is what a message about a pull request starts with: enough to tell the
// receiving session exactly which pull request the text is about, and an empty line to write on.
func pullRequestPrefill(pullRequest pullrequests.PullRequest) string {
	repository := pullRequest.Repository
	return fmt.Sprintf("About pull request %s/%s#%d: %s\nURL: %s\nBranch: %s\n\n",
		repository.Owner, repository.Name, pullRequest.Number, pullRequest.Title, pullRequest.URL, pullRequest.HeadBranch)
}
