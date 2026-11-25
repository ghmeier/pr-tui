package github

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

type conversationUser struct {
	Login string
	Type  string
}

func reviewStateFrom(text string) (state pullrequests.ReviewState, submitted bool) {
	switch text {
	case "APPROVED":
		return pullrequests.ReviewStateApproved, true
	case "CHANGES_REQUESTED":
		return pullrequests.ReviewStateChangesRequested, true
	case "COMMENTED":
		return pullrequests.ReviewStateCommented, true
	}
	return pullrequests.ReviewStateNone, false
}

// Conversation merges the pull request's own comments with the text of its submitted reviews. The
// two come from different endpoints, so they are fetched together. A review with no text, such as
// a bare approval, has nothing to read, and a dismissed or still pending review is not part of the
// conversation yet.
func (github *provider) Conversation(pr pullrequests.PullRequest) ([]pullrequests.ConversationEntry, error) {
	repository := fmt.Sprintf("repos/%s", pr.Repository.FullName())
	var comments, reviews []pullrequests.ConversationEntry
	var commentsErr, reviewsErr error
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		comments, commentsErr = github.issueComments(fmt.Sprintf("%s/issues/%d/comments?per_page=100", repository, pr.Number))
	}()
	go func() {
		defer group.Done()
		reviews, reviewsErr = github.submittedReviews(apiEndpoint(pr) + "/reviews?per_page=100")
	}()
	group.Wait()
	if commentsErr != nil {
		return nil, commentsErr
	}
	if reviewsErr != nil {
		return nil, reviewsErr
	}
	entries := append(comments, reviews...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Created.Before(entries[j].Created) })
	return entries, nil
}

func (github *provider) issueComments(endpoint string) ([]pullrequests.ConversationEntry, error) {
	output, err := shell.Run("gh", "api", "--paginate", endpoint, "--jq", ".[]")
	if err != nil {
		return nil, err
	}
	response, err := decodeEach[struct {
		User      conversationUser
		Body      string
		CreatedAt time.Time `json:"created_at"`
	}](output)
	if err != nil {
		return nil, err
	}
	var entries []pullrequests.ConversationEntry
	for _, comment := range response {
		entries = append(entries, pullrequests.ConversationEntry{
			Author: comment.User.Login, IsBot: comment.User.Type == "Bot", Body: comment.Body, Created: comment.CreatedAt,
		})
	}
	return entries, nil
}

func (github *provider) submittedReviews(endpoint string) ([]pullrequests.ConversationEntry, error) {
	output, err := shell.Run("gh", "api", "--paginate", endpoint, "--jq", ".[]")
	if err != nil {
		return nil, err
	}
	response, err := decodeEach[struct {
		User        conversationUser
		Body        string
		State       string
		SubmittedAt time.Time `json:"submitted_at"`
	}](output)
	if err != nil {
		return nil, err
	}
	var entries []pullrequests.ConversationEntry
	for _, review := range response {
		state, submitted := reviewStateFrom(review.State)
		if !submitted || review.Body == "" {
			continue
		}
		entries = append(entries, pullrequests.ConversationEntry{
			Author: review.User.Login, IsBot: review.User.Type == "Bot", Body: review.Body, Created: review.SubmittedAt, Review: state,
		})
	}
	return entries, nil
}
