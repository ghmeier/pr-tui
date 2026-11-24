package pullrequests

import (
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

// addedLineTarget is the comment target of the added line that pressToTheAddedLine moves to.
var addedLineTarget = CommentTarget{Path: "first.py", Line: 2, Side: CommentSideNew}

func draftAt(target CommentTarget, body string) DraftComment {
	return DraftComment{Path: target.Path, Line: target.Line, Side: target.Side, Body: body}
}

// postedThreadRows are the presses that move from the diff's first line to the first posted
// comment of the thread on the added line.
var postedThreadRows = []string{"enter", "j", "j", "j", "j", "j", "j", "j"}

func assertShows(t *testing.T, model *plugin.Model, expected ...string) {
	t.Helper()
	for _, text := range expected {
		if !strings.Contains(view(model), text) {
			t.Fatalf("view is missing %q:\n%s", text, view(model))
		}
	}
}

func assertDoesNotShow(t *testing.T, model *plugin.Model, unexpected ...string) {
	t.Helper()
	for _, text := range unexpected {
		if strings.Contains(view(model), text) {
			t.Fatalf("view should not contain %q:\n%s", text, view(model))
		}
	}
}

func TestCommentOnACodeLineAddsAPendingCommentWithoutPosting(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Looks good")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c")...)

	assertCalls(t, provider)
	assertShows(t, model, "Added to your review (1 pending) · a submits it", "┌ pending", "│ Looks good", "· 1 pending")
	assertDoesNotShow(t, model, "outdated")
}

func TestPendingCommentIsSubmittedWithTheLineAndSideItWasWrittenOn(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Looks good", "Overall")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "a", "3", "y")...)

	review := Review{Event: ReviewEventComment, Body: "Overall", Comments: []DraftComment{draftAt(addedLineTarget, "Looks good")}}
	assertCalls(t, provider, reviewCall(provider.pullRequest(1), review))
}

func TestLeftArrowMovesAPendingCommentToTheRemovedLine(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Why remove this?", "")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "left", "c", "a", "3", "y")...)

	removedLineTarget := CommentTarget{Path: "first.py", Line: 2, Side: CommentSideOld}
	review := Review{Event: ReviewEventComment, Comments: []DraftComment{draftAt(removedLineTarget, "Why remove this?")}}
	assertCalls(t, provider, reviewCall(provider.pullRequest(1), review))
}

func TestSavingAnEmptyCommentAddsNothing(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "\n")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c")...)

	assertShows(t, model, "Comment discarded.")
	assertDoesNotShow(t, model, "pending")
}

func TestCommentKeyOnAHeaderLineOpensNoEditorAndSaysWhy(t *testing.T) {
	session := editorWrites(t, "Never used")
	model := loadedBoard(t)

	press(t, model, "enter", "c")

	assertShows(t, model, "Move to a code line to comment on it.")
	if len(session.opened) != 0 {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestCommentKeyOnAPendingCommentEditsIt(t *testing.T) {
	editorWrites(t, "First draft", "Second draft")
	model := loadedBoard(t)

	press(t, model, append(pressToTheAddedLine(), "c", "j", "j", "j", "j", "j", "c")...)

	assertShows(t, model, "Pending comment updated.", "│ Second draft", "· 1 pending")
	assertDoesNotShow(t, model, "First draft")
}

func TestCommentKeyOnTheLineOfAPendingCommentEditsItInsteadOfAddingAnother(t *testing.T) {
	session := editorWrites(t, "First draft", "Second draft")
	model := loadedBoard(t)

	press(t, model, append(pressToTheAddedLine(), "c", "c")...)

	if len(session.opened) != 2 || session.opened[1] != "First draft" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
	assertShows(t, model, "│ Second draft", "· 1 pending")
}

func TestSavingAnEmptyTextOnAPendingCommentRemovesIt(t *testing.T) {
	editorWrites(t, "First draft", "\n")
	model := loadedBoard(t)

	press(t, model, append(pressToTheAddedLine(), "c", "j", "j", "j", "j", "j", "c")...)

	assertShows(t, model, "Pending comment removed.")
	assertDoesNotShow(t, model, "First draft", "pending")
}

func TestCommentKeyOnAPostedThreadPostsTheReplyAtOnceAndAddsNoPendingComment(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Thanks")
	model := loadedBoardWith(t, provider)

	press(t, model, append(postedThreadRows, "c")...)

	target := CommentTarget{Path: "first.py", Line: 2, Side: CommentSideNew, ReplyTo: provider.comments[0].ID, ReplyAuthor: provider.comments[0].Author}
	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), target, "Thanks"))
	assertShows(t, model, "Comment posted on first.py line 2 (new).")
	assertDoesNotShow(t, model, "pending")
}

func TestEmptyReplyPostsNothing(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "\n")
	model := loadedBoardWith(t, provider)

	press(t, model, append(postedThreadRows, "c")...)

	assertCalls(t, provider)
	assertShows(t, model, "Comment discarded.")
}

func TestFailedReplyIsOfferedAgainOnTheNextComment(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("PostComment", errorForTests)
	session := editorWrites(t, "Thanks", "")
	model := loadedBoardWith(t, provider)

	press(t, model, append(postedThreadRows, "c")...)
	notice := view(model)
	press(t, model, "c")

	if !strings.Contains(notice, "Failed: "+errorForTests.Error()+" · press c to try again") {
		t.Fatalf("view:\n%s", notice)
	}
	if len(session.opened) != 2 || session.opened[0] != "" || session.opened[1] != "Thanks" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestPostedReplyIsNotOfferedAgain(t *testing.T) {
	session := editorWrites(t, "Thanks", "")
	model := loadedBoard(t)

	press(t, model, append(postedThreadRows, "c", "c")...)

	if len(session.opened) != 2 || session.opened[1] != "" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestPendingCommentIsDrawnAfterThePostedThreadOnItsLineWithAPendingLabel(t *testing.T) {
	editorWrites(t, "Draft text")
	model := loadedBoard(t)
	press(t, model, append(pressToTheAddedLine(), "c")...)
	viewLines := strings.Split(view(model), "\n")

	added := -1
	for index, line := range viewLines {
		if strings.Contains(line, "2 +added") {
			added = index
		}
	}
	if added < 0 {
		t.Fatalf("view has no added line:\n%s", view(model))
	}
	expectedRows := []string{"┌ reviewer", "│ Why this change?", "├ person", "│ Because it was wrong.", "┌ pending", "│ Draft text", "3 +added again"}
	for offset, expected := range expectedRows {
		if !strings.Contains(viewLines[added+1+offset], expected) {
			t.Fatalf("row %d after the added line = %q, expected %q", offset+1, viewLines[added+1+offset], expected)
		}
	}
}

func TestHeaderCountsThePendingCommentsOnlyWhenThereAreSome(t *testing.T) {
	editorWrites(t, "First", "Second")
	model := loadedBoard(t)
	press(t, model, "enter")
	withoutAny := view(model)

	press(t, model, pressToTheAddedLine()[1:]...)
	press(t, model, "c")
	withOne := view(model)
	press(t, model, "j", "j", "j", "j", "j", "j", "j", "c")

	if strings.Contains(withoutAny, "pending") {
		t.Fatalf("view:\n%s", withoutAny)
	}
	if !strings.Contains(withOne, "· 1 pending") {
		t.Fatalf("view:\n%s", withOne)
	}
	assertShows(t, model, "· 2 pending")
}

func TestReviewKeyOnSomeoneElsesPullRequestOffersApproveRequestChangesAndComment(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "a")

	assertShows(t, model, "Review "+provider.pullRequest(1).label(), "approve", "request changes", "comment")
	assertCalls(t, provider)
}

func TestReviewKeyOnTheViewersOwnPullRequestGoesStraightToTheEditorAsACommentReview(t *testing.T) {
	provider := newFakeProvider()
	provider.pullRequests[0].Author = testViewer
	session := editorWrites(t, "Thoughts")
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(1)

	press(t, model, "a")
	asked := view(model)
	press(t, model, "y")

	if len(session.opened) != 1 {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
	if !strings.Contains(asked, "Submit review of "+selected.label()+": comment?") || strings.Contains(asked, "approve") {
		t.Fatalf("view while asking:\n%s", asked)
	}
	assertCalls(t, provider, reviewCall(selected, Review{Event: ReviewEventComment, Body: "Thoughts"}))
}

func TestRequestingChangesWithAnEmptyBodySubmitsNothingAndSaysWhy(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "a", "2")

	assertCalls(t, provider)
	assertShows(t, model, "A review that requests changes needs a body.")
	assertDoesNotShow(t, model, "Submit review of")
}

func TestCommentReviewWithoutABodyOrPendingCommentsSubmitsNothingAndSaysWhy(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "a", "3")

	assertCalls(t, provider)
	assertShows(t, model, "A comment review needs a body or pending comments.")
}

func TestCommentReviewWithPendingCommentsAndNoBodyIsAcceptedAndNamesTheirCount(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Looks good", "\n")
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(1)

	press(t, model, append(pressToTheAddedLine(), "c", "a", "3")...)
	asked := view(model)
	press(t, model, "y")

	if !strings.Contains(asked, "Submit review of "+selected.label()+": comment, with 1 pending comment?") {
		t.Fatalf("view while asking:\n%s", asked)
	}
	assertCalls(t, provider, reviewCall(selected, Review{Event: ReviewEventComment, Comments: []DraftComment{draftAt(addedLineTarget, "Looks good")}}))
}

func TestApprovalWithoutABodyIsAskedWithoutMentioningPendingComments(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "")
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(1)

	press(t, model, "a", "1")
	asked := view(model)
	press(t, model, "y")

	if !strings.Contains(asked, "Submit review of "+selected.label()+": approve?") {
		t.Fatalf("view while asking:\n%s", asked)
	}
	assertCalls(t, provider, reviewCall(selected, Review{Event: ReviewEventApprove}))
}

func TestConfirmingSubmitsTheEventTheBodyAndAllDrafts(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "First", "Second", "Needs work")
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(1)

	press(t, model, append(pressToTheAddedLine(), "c", "left", "c", "a", "2", "y")...)

	removedLineTarget := CommentTarget{Path: "first.py", Line: 2, Side: CommentSideOld}
	review := Review{
		Event: ReviewEventRequestChanges, Body: "Needs work",
		Comments: []DraftComment{draftAt(addedLineTarget, "First"), draftAt(removedLineTarget, "Second")},
	}
	assertCalls(t, provider, reviewCall(selected, review))
}

func TestSubmittingNamesThePullRequestInItsNoticeWhileItWorks(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "")
	model := loadedBoardWith(t, provider)

	press(t, model, "a", "1")
	model.Update(keyMessage("y"))

	assertShows(t, model, "Submitting review of "+provider.pullRequest(1).label())
}

func TestDecliningTheReviewSubmitsNothingAndKeepsTheDraftsAndTheBody(t *testing.T) {
	provider := newFakeProvider()
	session := editorWrites(t, "Draft", "My body", "")
	model := loadedBoardWith(t, provider)
	press(t, model, append(pressToTheAddedLine(), "c", "a", "3", "n")...)

	assertCalls(t, provider)
	assertShows(t, model, "· 1 pending")
	press(t, model, "a", "3")

	if len(session.opened) != 3 || session.opened[2] != "My body" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestSuccessfulSubmissionClearsTheDraftsAndTheBody(t *testing.T) {
	provider := newFakeProvider()
	session := editorWrites(t, "Draft", "My body", "")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "a", "3", "y")...)
	assertDoesNotShow(t, model, "pending")
	press(t, model, "a", "3")

	if len(session.opened) != 3 || session.opened[2] != "" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestFailedSubmissionKeepsTheDraftsAndTheBody(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SubmitReview", errorForTests)
	session := editorWrites(t, "Draft", "My body", "")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "a", "3", "y")...)
	assertShows(t, model, "Failed: "+errorForTests.Error(), "· 1 pending")
	press(t, model, "a", "3")

	if len(session.opened) != 3 || session.opened[2] != "My body" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestReviewKeyOnAScreenAsksThenSubmits(t *testing.T) {
	for name, opener := range screenOpeners {
		t.Run(name, func(t *testing.T) {
			provider := newFakeProvider()
			editorWrites(t, "")
			model := loadedBoardWith(t, provider)
			selected := provider.pullRequest(1)
			press(t, model, opener...)

			press(t, model, "a", "1")
			asked := view(model)
			press(t, model, "y")

			if !strings.Contains(asked, "Submit review of "+selected.label()+": approve?") {
				t.Fatalf("view while asking:\n%s", asked)
			}
			assertCalls(t, provider, reviewCall(selected, Review{Event: ReviewEventApprove}))
		})
	}
}

func TestDiffScreenShowsPostedReviewCommentsAsSoonAsTheSubmissionFinishes(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Draft", "")
	model := loadedBoardWith(t, provider)
	press(t, model, append(pressToTheAddedLine(), "c")...)
	provider.comments = append(provider.comments, ReviewComment{
		ID: 20, Path: "first.py", Line: 3, Side: CommentSideNew, Author: "newcomer", Body: "Posted by the review",
	})
	press(t, model, "k")
	assertDoesNotShow(t, model, "Posted by the review")

	press(t, model, "a", "3", "y")

	assertShows(t, model, "Posted by the review")
}

func TestPendingReviewsKeepADraftAddedWhileASubmissionRan(t *testing.T) {
	reviews := newPendingReviews()
	const url = "https://example.test/storefront/1"
	sent := draftAt(addedLineTarget, "Sent")
	meanwhile := draftAt(CommentTarget{Path: "other.py", Line: 5}, "Added meanwhile")
	reviews.save(url, sent)
	reviews.setBody(url, "Body")
	reviews.save(url, meanwhile)

	reviews.submitted(url, Review{Body: "Body", Comments: []DraftComment{sent}})

	if drafts := reviews.drafts(url); len(drafts) != 1 || drafts[0] != meanwhile {
		t.Fatalf("drafts = %+v, expected only %+v", drafts, meanwhile)
	}
	if reviews.body(url) != "" || reviews.submissions(url) != 1 {
		t.Fatalf("body = %q, submissions = %d", reviews.body(url), reviews.submissions(url))
	}
}

func TestPendingReviewsKeepADraftEditedWhileASubmissionRan(t *testing.T) {
	reviews := newPendingReviews()
	const url = "https://example.test/storefront/1"
	sent := draftAt(addedLineTarget, "Sent")
	edited := draftAt(addedLineTarget, "Edited meanwhile")
	reviews.save(url, sent)
	reviews.save(url, edited)

	reviews.submitted(url, Review{Comments: []DraftComment{sent}})

	if drafts := reviews.drafts(url); len(drafts) != 1 || drafts[0] != edited {
		t.Fatalf("drafts = %+v, expected only %+v", drafts, edited)
	}
}

func TestPendingReviewsReplaceTheDraftOfTheSameTargetAndKeepThoseOfOthers(t *testing.T) {
	reviews := newPendingReviews()
	const url = "https://example.test/storefront/1"
	otherSide := CommentTarget{Path: addedLineTarget.Path, Line: addedLineTarget.Line, Side: CommentSideOld}
	reviews.save(url, draftAt(addedLineTarget, "First"))
	reviews.save(url, draftAt(otherSide, "Old side"))

	count := reviews.save(url, draftAt(addedLineTarget, "Second"))

	if found, _ := reviews.draftFor(url, addedLineTarget); count != 2 || found.Body != "Second" {
		t.Fatalf("count = %d, draft = %+v", count, found)
	}
	if found, _ := reviews.draftFor(url, otherSide); found.Body != "Old side" {
		t.Fatalf("draft = %+v", found)
	}
}

func TestPendingReviewsKeepEachPullRequestsDraftsApart(t *testing.T) {
	reviews := newPendingReviews()
	reviews.save("first", draftAt(addedLineTarget, "Only here"))

	if drafts := reviews.drafts("second"); len(drafts) != 0 {
		t.Fatalf("drafts = %+v", drafts)
	}
}

func TestPendingReviewsChangeTheirVersionOnEveryModification(t *testing.T) {
	reviews := newPendingReviews()
	const url = "https://example.test/storefront/1"
	changes := []func(){
		func() { reviews.save(url, draftAt(addedLineTarget, "Text")) },
		func() { reviews.setBody(url, "Body") },
		func() { reviews.remove(url, addedLineTarget) },
		func() { reviews.submitted(url, Review{}) },
	}

	for index, change := range changes {
		before := reviews.currentVersion()
		change()
		if reviews.currentVersion() == before {
			t.Fatalf("change %d left the version at %d", index, before)
		}
	}
}

// diffWithoutTheAddedLine keeps first.py to its one unchanged line, so that comments written on
// its second line are outdated, and leaves the other file as it was.
const diffWithoutTheAddedLine = `diff --git a/first.py b/first.py
index 111..222 100644
--- a/first.py
+++ b/first.py
@@ -1 +1 @@
 unchanged
diff --git a/old_name.py b/second/new_name.py
similarity index 90%
rename from old_name.py
rename to second/new_name.py
--- a/old_name.py
+++ b/second/new_name.py
@@ -1 +1 @@
--- looks like a header
+++ looks like a header
`

// reopenedDiffAfterAPush writes a pending comment on the added line, closes the diff, and opens it
// again with the provider now serving the given diff.
func reopenedDiffAfterAPush(t *testing.T, newDiff string) (*plugin.Model, *editorSession) {
	t.Helper()
	provider := newFakeProvider()
	session := editorWrites(t, "Draft", "\n")
	model := loadedBoardWith(t, provider)
	press(t, model, append(pressToTheAddedLine(), "c", "esc")...)
	provider.diff = newDiff
	press(t, model, "enter")
	return model, session
}

func TestPendingCommentWhoseLineLeftTheDiffIsDrawnAtTheTopOfItsFileAsOutdated(t *testing.T) {
	model, _ := reopenedDiffAfterAPush(t, diffWithoutTheAddedLine)

	assertShows(t, model, "┌ pending · outdated · first.py:2", "│ Draft", "· 1 pending")
}

func TestPendingCommentWhoseFileLeftTheDiffIsDrawnAtTheTopOfTheFirstFile(t *testing.T) {
	_, onlyTheSecondFile, _ := strings.Cut(diffWithoutTheAddedLine, "diff --git a/old_name.py")
	model, _ := reopenedDiffAfterAPush(t, "diff --git a/old_name.py"+onlyTheSecondFile)

	assertShows(t, model, "second/new_name.py", "┌ pending · outdated · first.py:2", "│ Draft")
}

func TestOutdatedPendingCommentIsNotDrawnOnAnotherFileThanItsOwn(t *testing.T) {
	model, _ := reopenedDiffAfterAPush(t, diffWithoutTheAddedLine)

	press(t, model, "down")

	assertDoesNotShow(t, model, "outdated")
}

func TestCommentKeyOnAnOutdatedPendingCommentEditsIt(t *testing.T) {
	model, session := reopenedDiffAfterAPush(t, diffWithoutTheAddedLine)

	press(t, model, "c")

	if len(session.opened) != 2 || session.opened[1] != "Draft" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
	assertShows(t, model, "Pending comment removed.")
	assertDoesNotShow(t, model, "outdated", "pending")
}

func TestSecondSubmissionStartedWhileTheFirstIsRunningFailsAndSendsNothing(t *testing.T) {
	provider := newFakeProvider()
	pr := provider.pullRequest(1)
	board := &pullRequestBoard{provider: provider, reviews: newPendingReviews()}
	effect := board.submitReview(plugin.Selection{Row: Row{PullRequest: pr}}, Review{Event: ReviewEventApprove})
	var secondFailure error
	provider.whileSubmitting = func() {
		provider.whileSubmitting = nil
		secondFailure = effect.Command()
	}

	firstFailure := effect.Command()

	if firstFailure != nil {
		t.Fatalf("the first submission failed: %v", firstFailure)
	}
	if expected := "a review of " + pr.label() + " is already being submitted"; secondFailure == nil || secondFailure.Error() != expected {
		t.Fatalf("the second submission returned %v, expected %q", secondFailure, expected)
	}
	assertCalls(t, provider, reviewCall(pr, Review{Event: ReviewEventApprove}))
}

func TestSubmissionCanStartAgainOnceTheFirstHasFailed(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SubmitReview", errorForTests)
	pr := provider.pullRequest(1)
	board := &pullRequestBoard{provider: provider, reviews: newPendingReviews()}
	effect := board.submitReview(plugin.Selection{Row: Row{PullRequest: pr}}, Review{Event: ReviewEventApprove})

	effect.Command()
	secondFailure := effect.Command()

	if secondFailure != errorForTests {
		t.Fatalf("the second submission returned %v", secondFailure)
	}
	if len(provider.recorded()) != 2 {
		t.Fatalf("calls = %+v", provider.recorded())
	}
}

func TestPendingReviewsKeepABodyWrittenWhileASubmissionRan(t *testing.T) {
	reviews := newPendingReviews()
	const url = "https://example.test/storefront/1"
	reviews.setBody(url, "Sent body")
	reviews.setBody(url, "Written meanwhile")

	reviews.submitted(url, Review{Body: "Sent body"})

	if reviews.body(url) != "Written meanwhile" {
		t.Fatalf("body = %q", reviews.body(url))
	}
}
