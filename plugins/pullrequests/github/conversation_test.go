package github

import (
	"strings"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

const (
	cannedIssueComments = `{"user": {"login": "person"}, "body": "Ready for another look.", "created_at": "2026-11-10T12:00:00Z"}
{"user": {"login": "dependabot[bot]"}, "body": "Bumped.", "created_at": "2026-11-10T08:00:00Z"}`

	// Only some of these reviews have text that was submitted: the empty approval, the dismissed
	// review, and the pending one are not part of the conversation.
	cannedReviews = `{"user": {"login": "reviewer"}, "body": "Please rename this.", "state": "CHANGES_REQUESTED", "submitted_at": "2026-11-10T09:00:00Z"}
{"user": {"login": "reviewer"}, "body": "", "state": "APPROVED", "submitted_at": "2026-11-10T10:00:00Z"}
{"user": {"login": "reviewer"}, "body": "Looks good now.", "state": "APPROVED", "submitted_at": "2026-11-10T11:00:00Z"}
{"user": {"login": "reviewer"}, "body": "Dismissed text", "state": "DISMISSED", "submitted_at": "2026-11-10T11:30:00Z"}
{"user": {"login": "reviewer"}, "body": "Just a note", "state": "COMMENTED", "submitted_at": "2026-11-10T07:00:00Z"}
{"user": {"login": "reviewer"}, "body": "Unsent", "state": "PENDING", "submitted_at": null}`
)

// stubConversation answers the comments endpoint and the reviews endpoint with their own text.
func stubConversation(t *testing.T, comments, reviews string) *stubbedGh {
	t.Helper()
	return stubShellWith(t, func(arguments []string) ([]byte, error) {
		if strings.Contains(strings.Join(arguments, " "), "/reviews") {
			return []byte(reviews), nil
		}
		return []byte(comments), nil
	})
}

func TestConversationAsksForEveryPageOfTheCommentsAndOfTheReviews(t *testing.T) {
	stub := stubConversation(t, "", "")

	_, err := providerFor(storefront).Conversation(fixturePullRequest(storefront))

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommandsInAnyOrder(t,
		[]string{"gh", "api", "--paginate", "repos/" + storefront.FullName() + "/issues/7/comments?per_page=100", "--jq", ".[]"},
		[]string{"gh", "api", "--paginate", "repos/" + storefront.FullName() + "/pulls/7/reviews?per_page=100", "--jq", ".[]"},
	)
}

func TestConversationListsCommentsAndReviewTextOldestFirst(t *testing.T) {
	stubConversation(t, cannedIssueComments, cannedReviews)
	at := func(hour int) time.Time { return time.Date(2026, 11, 10, hour, 0, 0, 0, time.UTC) }

	entries, err := providerFor(storefront).Conversation(fixturePullRequest(storefront))

	expected := []pullrequests.ConversationEntry{
		{Author: "reviewer", Body: "Just a note", Created: at(7), Review: pullrequests.ReviewStateCommented},
		{Author: "dependabot[bot]", Body: "Bumped.", Created: at(8)},
		{Author: "reviewer", Body: "Please rename this.", Created: at(9), Review: pullrequests.ReviewStateChangesRequested},
		{Author: "reviewer", Body: "Looks good now.", Created: at(11), Review: pullrequests.ReviewStateApproved},
		{Author: "person", Body: "Ready for another look.", Created: at(12)},
	}
	if err != nil || !equalEntries(entries, expected) {
		t.Fatalf("entries = %+v, err = %v, expected %+v", entries, err, expected)
	}
}

func TestConversationLeavesOutReviewsWithoutTextAndDismissedOrPendingReviews(t *testing.T) {
	stubConversation(t, "", cannedReviews)

	entries, err := providerFor(storefront).Conversation(fixturePullRequest(storefront))

	for _, entry := range entries {
		if entry.Body == "" || entry.Body == "Dismissed text" || entry.Body == "Unsent" {
			t.Errorf("the conversation has %+v", entry)
		}
	}
	if err != nil || len(entries) != 3 {
		t.Fatalf("entries = %+v, err = %v", entries, err)
	}
}

func TestConversationReadsCommentsFromEveryPageOfTheOutput(t *testing.T) {
	stubConversation(t, cannedIssueComments+"\n"+cannedIssueComments, "")

	entries, err := providerFor(storefront).Conversation(fixturePullRequest(storefront))

	if err != nil || len(entries) != 4 {
		t.Fatalf("entries = %+v, err = %v", entries, err)
	}
}

func TestConversationFailsWhenEitherRequestFails(t *testing.T) {
	for _, failing := range []string{"/issues/", "/reviews"} {
		stubShellWith(t, func(arguments []string) ([]byte, error) {
			if strings.Contains(strings.Join(arguments, " "), failing) {
				return nil, errorFromGh
			}
			return nil, nil
		})

		_, err := providerFor(storefront).Conversation(fixturePullRequest(storefront))

		if err != errorFromGh {
			t.Errorf("with %s failing, err = %v", failing, err)
		}
	}
}

func equalEntries(got, expected []pullrequests.ConversationEntry) bool {
	if len(got) != len(expected) {
		return false
	}
	for index := range got {
		if got[index] != expected[index] {
			return false
		}
	}
	return true
}
