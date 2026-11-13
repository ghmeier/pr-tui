package github

import (
	"reflect"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

const cannedChangedFiles = `[
  {"filename": "kept.py", "status": "modified"},
  {"filename": "deleted.py", "status": "removed"},
  {"filename": "tests/added.py", "status": "added"}
]`

const cannedComments = `[
  {"id": 10, "in_reply_to_id": null, "path": "first.py", "line": 2, "side": "RIGHT",
   "user": {"login": "reviewer"}, "body": "Why this change?", "created_at": "2026-11-10T09:00:00Z"},
  {"id": 11, "in_reply_to_id": 10, "path": "first.py", "line": 2, "side": "RIGHT",
   "user": {"login": "person"}, "body": "Because it was wrong.", "created_at": "2026-11-10T09:30:00Z"},
  {"id": 12, "in_reply_to_id": null, "path": "first.py", "line": null, "side": "LEFT",
   "user": {"login": "reviewer"}, "body": "Outdated remark", "created_at": "2026-11-09T09:00:00Z"}
]`

func TestDiffReturnsTheTextOfGhPrDiff(t *testing.T) {
	pr := fixturePullRequest(gazebo)
	stub := stubShell(t, "diff --git a/x b/x\n")

	text, err := providerFor(gazebo).Diff(pr)

	if err != nil || text != "diff --git a/x b/x\n" {
		t.Fatalf("text = %q, err = %v", text, err)
	}
	stub.assertCommands(t, []string{"gh", "pr", "diff", pr.URL})
}

func TestDiffReturnsTheCommandFailure(t *testing.T) {
	stubShellWith(t, func([]string) ([]byte, error) { return nil, errorFromGh })

	_, err := providerFor(gazebo).Diff(fixturePullRequest(gazebo))

	if err != errorFromGh {
		t.Fatalf("error = %v", err)
	}
}

func TestChangedFilesListsOnlyFilesThatStillExistOnTheBranch(t *testing.T) {
	pr := fixturePullRequest(partners)
	stub := stubShell(t, cannedChangedFiles)

	files, err := providerFor(partners).ChangedFiles(pr)

	if expected := []string{"kept.py", "tests/added.py"}; err != nil || !reflect.DeepEqual(files, expected) {
		t.Fatalf("files = %v, err = %v, expected %v", files, err, expected)
	}
	stub.assertCommands(t, []string{"gh", "api", "repos/" + partners.FullName() + "/pulls/7/files?per_page=100"})
}

func TestDescriptionReadsTheBodyWithUnixLineEndings(t *testing.T) {
	pr := fixturePullRequest(partners)
	stub := stubShell(t, `{"body": "Original description\r\n\r\nSecond paragraph"}`)

	body, err := providerFor(partners).Description(pr)

	if err != nil || body != "Original description\n\nSecond paragraph" {
		t.Fatalf("body = %q, err = %v", body, err)
	}
	stub.assertCommands(t, []string{"gh", "api", "repos/" + partners.FullName() + "/pulls/7"})
}

func TestSaveDescriptionPatchesTheBodyThroughAJSONFileOnTheOwnersRepository(t *testing.T) {
	pr := fixturePullRequest(partners)
	stub := stubShell(t, "")

	err := providerFor(partners).SaveDescription(pr, "Edited \"description\"")

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{"gh", "api", "--method", "PATCH", "repos/" + partners.FullName() + "/pulls/7"})
	stub.assertInputs(t, `{"body":"Edited \"description\""}`)
}

func TestSaveDescriptionReturnsTheCommandFailure(t *testing.T) {
	stubShellWith(t, func([]string) ([]byte, error) { return nil, errorFromGh })

	err := providerFor(gazebo).SaveDescription(fixturePullRequest(gazebo), "body")

	if err != errorFromGh {
		t.Fatalf("error = %v", err)
	}
}

func TestCommentsReadAuthorSideAndThreading(t *testing.T) {
	stub := stubShell(t, cannedComments)

	comments, err := providerFor(gazebo).Comments(fixturePullRequest(gazebo))

	expected := []pullrequests.ReviewComment{
		{ID: 10, Path: "first.py", Line: 2, Side: pullrequests.CommentSideNew, Author: "reviewer", Body: "Why this change?",
			CreatedAt: time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)},
		{ID: 11, InReplyTo: 10, Path: "first.py", Line: 2, Side: pullrequests.CommentSideNew, Author: "person", Body: "Because it was wrong.",
			CreatedAt: time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC)},
		{ID: 12, Path: "first.py", Line: 0, Side: pullrequests.CommentSideOld, Author: "reviewer", Body: "Outdated remark",
			CreatedAt: time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)},
	}
	if err != nil || !reflect.DeepEqual(comments, expected) {
		t.Fatalf("comments = %+v, err = %v, expected %+v", comments, err, expected)
	}
	stub.assertCommands(t, []string{"gh", "api", "repos/" + gazebo.FullName() + "/pulls/7/comments?per_page=100"})
}

func TestPostCommentOnTheNewSideSendsTheHeadCommitAndRight(t *testing.T) {
	pr := fixturePullRequest(partners)
	stub := stubShell(t, "")
	target := pullrequests.CommentTarget{Path: "first.py", Line: 2, Side: pullrequests.CommentSideNew}

	err := providerFor(partners).PostComment(pr, target, "Looks good")

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{"gh", "api", "--method", "POST", "repos/" + partners.FullName() + "/pulls/7/comments"})
	stub.assertInputs(t, `{"body":"Looks good","commit_id":"`+pr.HeadCommit+`","line":2,"path":"first.py","side":"RIGHT"}`)
}

func TestPostCommentOnTheOldSideSendsLeft(t *testing.T) {
	pr := fixturePullRequest(gazebo)
	stub := stubShell(t, "")
	target := pullrequests.CommentTarget{Path: "first.py", Line: 2, Side: pullrequests.CommentSideOld}

	err := providerFor(gazebo).PostComment(pr, target, "Why remove this?")

	if err != nil {
		t.Fatal(err)
	}
	stub.assertInputs(t, `{"body":"Why remove this?","commit_id":"`+pr.HeadCommit+`","line":2,"path":"first.py","side":"LEFT"}`)
}

func TestPostCommentAsAReplySendsOnlyTheBodyAndTheCommentItAnswers(t *testing.T) {
	pr := fixturePullRequest(gazebo)
	stub := stubShell(t, "")
	target := pullrequests.CommentTarget{Path: "first.py", Line: 2, ReplyTo: 10, ReplyAuthor: "reviewer"}

	err := providerFor(gazebo).PostComment(pr, target, "Thanks")

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{"gh", "api", "--method", "POST", "repos/" + gazebo.FullName() + "/pulls/7/comments"})
	stub.assertInputs(t, `{"body":"Thanks","in_reply_to":10}`)
}

func TestSetDraftFalseMarksThePullRequestReady(t *testing.T) {
	pr := fixturePullRequest(gazebo)
	stub := stubShell(t, "")

	err := providerFor(gazebo).SetDraft(pr, false)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{"gh", "pr", "ready", pr.URL})
}

func TestSetDraftTrueConvertsThePullRequestBackToDraft(t *testing.T) {
	pr := fixturePullRequest(gazebo)
	stub := stubShell(t, "")

	err := providerFor(gazebo).SetDraft(pr, true)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{"gh", "pr", "ready", pr.URL, "--undo"})
}

func TestSetDraftReturnsTheCommandFailure(t *testing.T) {
	stubShellWith(t, func([]string) ([]byte, error) { return nil, errorFromGh })

	err := providerFor(gazebo).SetDraft(fixturePullRequest(gazebo), false)

	if err != errorFromGh {
		t.Fatalf("error = %v", err)
	}
}

func TestSetAutoMergeEnablesSquashAutoMerge(t *testing.T) {
	pr := fixturePullRequest(gazebo)
	stub := stubShell(t, "")

	err := providerFor(gazebo).SetAutoMerge(pr, true)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{"gh", "pr", "merge", pr.URL, "--auto", "--squash"})
}

func TestSetAutoMergeDisablesAutoMerge(t *testing.T) {
	pr := fixturePullRequest(gazebo)
	stub := stubShell(t, "")

	err := providerFor(gazebo).SetAutoMerge(pr, false)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{"gh", "pr", "merge", pr.URL, "--disable-auto"})
}

func TestRequestTeamReviewGoesToTheOwnerOfThePullRequest(t *testing.T) {
	pr := fixturePullRequest(partners)
	stub := stubShell(t, "")

	err := providerFor(partners).RequestTeamReview(pr)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{
		"gh", "api", "--method", "POST", "repos/" + partners.FullName() + "/pulls/7/requested_reviewers",
		"-f", "team_reviewers[]=" + reviewTeamName,
	})
}
