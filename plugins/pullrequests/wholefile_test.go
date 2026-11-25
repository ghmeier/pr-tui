package pullrequests

import (
	"strings"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

const wholeFileRemark = "Split this file up."

// wholeFileTarget is the target of a comment on the first file of the fixture diff.
var wholeFileTarget = CommentTarget{Path: "first.py", WholeFile: true}

// openedOnTheFileRow opens the first pull request's diff, where the cursor starts on the file row.
func openedOnTheFileRow() []string {
	return []string{"enter"}
}

func TestWholeFileTargetLabelNamesTheFileAndNoLine(t *testing.T) {
	label := wholeFileTarget.label()

	if label != wholeFileTarget.Path+" (whole file)" {
		t.Fatalf("label = %q", label)
	}
}

func TestEachFileOfTheDiffStartsWithAFileRow(t *testing.T) {
	files := parseDiff(fixtureDiff)

	for _, file := range files {
		if file.lines[0].kind != fileLine {
			t.Fatalf("%s starts with a line of kind %d", file.path, file.lines[0].kind)
		}
	}
}

func TestFileRowIsDrawnDimAboveTheFileHeader(t *testing.T) {
	model := loadedBoard(t)

	press(t, model, openedOnTheFileRow()...)

	assertShows(t, model, "▌ "+wholeFileLabel)
}

func TestFileRowCannotBeCopiedWithTheMouseAsCode(t *testing.T) {
	screen := wrappingScreen(wrapDiff(), false, 50, 30)
	screen.drag = &dragSelection{anchor: 0, end: 0}

	code := screen.selectedCode()

	if code != "" {
		t.Fatalf("the file row copied as %q", code)
	}
}

func TestCommentOnTheFileRowOffersPostingAndOfferedActionsButNotTheReview(t *testing.T) {
	recipient := newCommentRecipient()
	editorWrites(t, wholeFileRemark)
	model := modelWithRecipient(t, newFakeProvider(), recipient)

	press(t, model, append(openedOnTheFileRow(), "c")...)

	assertShows(t, model, commentQuestion, postOptionLine, "2 "+recipient.label)
	assertDoesNotShow(t, model, addToReviewOption)
}

func TestCommentOnACodeLineStillOffersTheReview(t *testing.T) {
	editorWrites(t, wholeFileRemark)
	model := modelWithRecipient(t, newFakeProvider(), newCommentRecipient())

	press(t, model, append(pressToTheAddedLine(), "c")...)

	assertShows(t, model, commentQuestion, postOptionLine, reviewOptionRow)
}

func TestChoosingToPostAWholeFileCommentPostsItWithAWholeFileTarget(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, wholeFileRemark)
	model := modelWithRecipient(t, provider, newCommentRecipient())

	press(t, model, append(openedOnTheFileRow(), "c", "1")...)

	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), wholeFileTarget, wholeFileRemark))
	assertShows(t, model, "Comment posted on "+wholeFileTarget.label()+".")
	assertDoesNotShow(t, model, "pending")
}

func TestWholeFileCommentIsPostedAtOnceWhenNoOtherPluginOffersAPlace(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, wholeFileRemark)
	model := loadedBoardWith(t, provider)

	press(t, model, append(openedOnTheFileRow(), "c")...)

	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), wholeFileTarget, wholeFileRemark))
	assertDoesNotShow(t, model, commentQuestion)
}

func TestFailedWholeFileCommentIsOfferedAgainOnTheFileRow(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("PostComment", errorForTests)
	session := editorWrites(t, wholeFileRemark)
	model := loadedBoardWith(t, provider)
	press(t, model, append(openedOnTheFileRow(), "c")...)

	press(t, model, "c")

	if len(session.opened) != 2 || session.opened[1] != wholeFileRemark {
		t.Fatalf("the editor opened with %q", session.opened)
	}
}

func TestWholeFileThreadFromTheProviderIsDrawnUnderTheFileRow(t *testing.T) {
	provider := newFakeProvider()
	provider.comments = append(provider.comments, wholeFileComment())
	model := loadedBoardWith(t, provider)

	press(t, model, openedOnTheFileRow()...)

	fileRow, thread, header := rowOf(t, model, wholeFileLabel), rowOf(t, model, "│ "+provider.comments[3].Body), rowOf(t, model, "--- a/first.py")
	if !(fileRow < thread && thread < header) {
		t.Fatalf("the thread is not between the file row and the file header:\n%s", view(model))
	}
}

func TestWholeFileThreadIsNotDrawnUnderAnotherFile(t *testing.T) {
	provider := newFakeProvider()
	provider.comments = append(provider.comments, wholeFileComment())
	model := loadedBoardWith(t, provider)

	press(t, model, "enter", "j")

	assertDoesNotShow(t, model, provider.comments[3].Body)
}

func TestCommentOnAWholeFileThreadRepliesToIt(t *testing.T) {
	provider := newFakeProvider()
	provider.comments = append(provider.comments, wholeFileComment())
	root := provider.comments[3]
	editorWrites(t, "Agreed.")
	model := loadedBoardWith(t, provider)

	press(t, model, "enter", "down", "c")

	target := CommentTarget{Path: root.Path, WholeFile: true, ReplyTo: root.ID, ReplyAuthor: root.Author}
	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), target, "Agreed."))
}

func TestGroupThreadsKeepsWholeFileCommentsAndStillDropsOutdatedLineComments(t *testing.T) {
	whole := ReviewComment{ID: 1, Path: "first.py", WholeFile: true}
	outdated := ReviewComment{ID: 2, Path: "first.py", Side: CommentSideOld}
	reply := ReviewComment{ID: 3, InReplyTo: whole.ID, Path: "first.py", WholeFile: true}

	threads := groupThreads([]ReviewComment{whole, outdated, reply})

	found := threads[wholeFileThreadKey("first.py")]
	if len(threads) != 1 || len(found) != 1 || found[0].root.ID != whole.ID || len(found[0].replies) != 1 {
		t.Fatalf("threads = %+v", threads)
	}
}

// rowOf is the index of the first line of the view that contains the text.
func rowOf(t *testing.T, model *plugin.Model, text string) int {
	t.Helper()
	for index, line := range strings.Split(view(model), "\n") {
		if strings.Contains(line, text) {
			return index
		}
	}
	t.Fatalf("no line contains %q:\n%s", text, view(model))
	return -1
}

func wholeFileComment() ReviewComment {
	return ReviewComment{ID: 20, Path: "first.py", WholeFile: true, Author: "reviewer", Body: "This file does too much.", CreatedAt: time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)}
}
