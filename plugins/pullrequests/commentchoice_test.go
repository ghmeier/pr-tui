package pullrequests

import (
	"flag"
	"reflect"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

// commentRecipient stands in for a plugin that takes written comments, such as one that sends
// them to an agent.
type commentRecipient struct {
	label   string
	applies bool
	// received is the comments the action was run with.
	received []Comment
}

func newCommentRecipient() *commentRecipient {
	return &commentRecipient{label: "send to a recipient", applies: true}
}

func (*commentRecipient) Name() string { return "recipient" }

func (*commentRecipient) Flags(*flag.FlagSet) {}

func (recipient *commentRecipient) Setup(host *plugin.Host) error {
	host.AddAction(BoardName, plugin.Action{
		Takes:   CommentKind,
		Label:   recipient.label,
		Applies: func(plugin.Selection) bool { return recipient.applies },
		Run: func(selection plugin.Selection) plugin.Effect {
			recipient.received = append(recipient.received, selection.Value.(Comment))
			return plugin.Effect{}
		},
	})
	return nil
}

func modelWithRecipient(t *testing.T, provider *fakeProvider, recipient *commentRecipient) *plugin.Model {
	t.Helper()
	return newSetup(provider).alongside(recipient).started(t)
}

const (
	commentQuestion = "Where should this comment go?"
	postOptionLine  = "1 " + postCommentOption
	reviewOptionRow = chooseAddToReview + " " + addToReviewOption
)

// chooseAddToReview is the key of the second option of the question a new comment asks.
const chooseAddToReview = "2"

func TestNewCommentAsksWhereItGoesWithPostingAndTheReviewAsOptions(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Looks good")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c")...)

	assertShows(t, model, commentQuestion, postOptionLine, reviewOptionRow)
	assertCalls(t, provider)
}

func TestNewCommentAlsoOffersTheActionsOfOtherPluginsAfterTheTwoOptions(t *testing.T) {
	recipient := newCommentRecipient()
	editorWrites(t, "Looks good")
	model := modelWithRecipient(t, newFakeProvider(), recipient)

	press(t, model, append(pressToTheAddedLine(), "c")...)

	assertShows(t, model, postOptionLine, reviewOptionRow, "3 "+recipient.label)
}

func TestNewCommentDoesNotOfferAnActionThatDoesNotApply(t *testing.T) {
	recipient := newCommentRecipient()
	recipient.applies = false
	editorWrites(t, "Looks good")
	model := modelWithRecipient(t, newFakeProvider(), recipient)

	press(t, model, append(pressToTheAddedLine(), "c")...)

	assertShows(t, model, postOptionLine, reviewOptionRow)
	assertDoesNotShow(t, model, recipient.label)
}

func TestChoosingToPostPostsTheNewCommentOnItsLine(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Looks good")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "1")...)

	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), addedLineTarget, "Looks good"))
	assertShows(t, model, "Comment posted on first.py line 2 (new).")
	assertDoesNotShow(t, model, "pending")
}

func TestChoosingToAddToTheReviewKeepsTheNewCommentPendingWithoutPosting(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Looks good")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", chooseAddToReview)...)

	assertCalls(t, provider)
	assertShows(t, model, "Added to your review (1 pending) · a submits it", "│ Looks good")
}

func TestChoosingAnotherPluginsActionRunsItWithTheComment(t *testing.T) {
	provider := newFakeProvider()
	recipient := newCommentRecipient()
	editorWrites(t, "Looks good")
	model := modelWithRecipient(t, provider, recipient)

	press(t, model, append(pressToTheAddedLine(), "c", "3")...)

	expected := Comment{PullRequest: provider.pullRequest(1), Target: addedLineTarget, Body: "Looks good"}
	if len(recipient.received) != 1 || !reflect.DeepEqual(recipient.received[0], expected) {
		t.Fatalf("the action received %v, expected %v", recipient.received, expected)
	}
	assertCalls(t, provider)
	assertDoesNotShow(t, model, "pending")
}

func TestChoosingAnotherPluginsActionDoesNotKeepTheCommentForNextTime(t *testing.T) {
	session := editorWrites(t, "Looks good", "")
	model := modelWithRecipient(t, newFakeProvider(), newCommentRecipient())

	press(t, model, append(pressToTheAddedLine(), "c", "3", "c")...)

	if len(session.opened) != 2 || session.opened[1] != "" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestCancellingTheChoiceKeepsTheTextForTheNextCommentOnTheLine(t *testing.T) {
	provider := newFakeProvider()
	session := editorWrites(t, "Looks good", "")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "esc")...)
	notice := view(model)
	press(t, model, "c")

	assertCalls(t, provider)
	if len(session.opened) != 2 || session.opened[1] != "Looks good" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
	if !strings.Contains(notice, commentKeptMessage) {
		t.Fatalf("view after cancelling:\n%s", notice)
	}
	assertDoesNotShow(t, model, "pending")
}

func TestEditingAPendingCommentAsksNothing(t *testing.T) {
	editorWrites(t, "First draft", "Second draft")
	model := loadedBoard(t)

	press(t, model, append(pressToTheAddedLine(), "c", chooseAddToReview, "c")...)

	assertShows(t, model, "Pending comment updated.", "│ Second draft")
	assertDoesNotShow(t, model, commentQuestion)
}

func TestReplyOffersPostingAndAnotherPluginsActionButNotTheReview(t *testing.T) {
	recipient := newCommentRecipient()
	editorWrites(t, "Thanks")
	model := modelWithRecipient(t, newFakeProvider(), recipient)

	press(t, model, append(postedThreadRows, "c")...)

	assertShows(t, model, commentQuestion, postOptionLine, "2 "+recipient.label)
	assertDoesNotShow(t, model, addToReviewOption)
}

func TestChoosingToPostAReplyPostsItInTheThread(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Thanks")
	model := modelWithRecipient(t, provider, newCommentRecipient())

	press(t, model, append(postedThreadRows, "c", "1")...)

	root := provider.comments[0]
	target := CommentTarget{Path: root.Path, Line: root.Line, Side: root.Side, ReplyTo: root.ID, ReplyAuthor: root.Author}
	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), target, "Thanks"))
}

func TestChoosingAnotherPluginsActionForAReplyPassesTheThreadTarget(t *testing.T) {
	provider := newFakeProvider()
	recipient := newCommentRecipient()
	editorWrites(t, "Thanks")
	model := modelWithRecipient(t, provider, recipient)

	press(t, model, append(postedThreadRows, "c", "2")...)

	root := provider.comments[0]
	expectedTarget := CommentTarget{Path: root.Path, Line: root.Line, Side: root.Side, ReplyTo: root.ID, ReplyAuthor: root.Author}
	if len(recipient.received) != 1 || recipient.received[0].Target != expectedTarget || recipient.received[0].Body != "Thanks" {
		t.Fatalf("the action received %v", recipient.received)
	}
	assertCalls(t, provider)
}

func TestCancellingTheChoiceOfAReplyKeepsItsTextForTheNextReply(t *testing.T) {
	session := editorWrites(t, "Thanks", "")
	model := modelWithRecipient(t, newFakeProvider(), newCommentRecipient())

	press(t, model, append(postedThreadRows, "c", "esc", "c")...)

	if len(session.opened) != 2 || session.opened[1] != "Thanks" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestFailedNewCommentIsOfferedAgainOnTheNextComment(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("PostComment", errorForTests)
	session := editorWrites(t, "Looks good", "")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "1")...)
	press(t, model, "c")

	if len(session.opened) != 2 || session.opened[1] != "Looks good" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}
