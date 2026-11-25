package agentsessions

import (
	"fmt"

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
	return sessionColumn(provider, func(rows []plugin.Row) map[string]matcher {
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
	return messageAction(provider, "pull request", func(row plugin.Row) string {
		return pullRequestPrefill(row.(pullrequests.Row).PullRequest)
	})
}

// pullRequestPrefill is what a message about a pull request starts with: enough to tell the
// receiving session exactly which pull request the text is about, and an empty line to write on.
func pullRequestPrefill(pullRequest pullrequests.PullRequest) string {
	repository := pullRequest.Repository
	return fmt.Sprintf("About pull request %s/%s#%d: %s\nURL: %s\nBranch: %s\n\n",
		repository.Owner, repository.Name, pullRequest.Number, pullRequest.Title, pullRequest.URL, pullRequest.HeadBranch)
}
