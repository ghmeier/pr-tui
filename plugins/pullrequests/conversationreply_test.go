package pullrequests

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

func descriptionWithConversation(t *testing.T, provider *fakeProvider) *plugin.Model {
	t.Helper()
	provider.conversation = fixtureConversation()
	model := loadedBoardWith(t, provider)
	press(t, model, "p")
	return model
}

// markedHeaders are the lines of the view that carry the cursor marker.
func markedHeaders(model *plugin.Model) []string {
	var marked []string
	for _, line := range strings.Split(view(model), "\n") {
		if strings.Contains(line, ui.CursorMarker) {
			marked = append(marked, line)
		}
	}
	return marked
}

func assertOnlyMarked(t *testing.T, model *plugin.Model, entry ConversationEntry) {
	t.Helper()
	marked := markedHeaders(model)
	if len(marked) != 1 || !strings.HasPrefix(marked[0], ui.CursorMarker+" "+entry.Author) {
		t.Fatalf("marked lines = %q, expected only the header of %s:\n%s", marked, entry.Author, view(model))
	}
}

func replyTargetFor(entry ConversationEntry) CommentTarget {
	return CommentTarget{Conversation: true, ReplyAuthor: entry.Author}
}

func TestJSelectsTheFirstCommentAndMarksItsHeader(t *testing.T) {
	provider := newFakeProvider()
	model := descriptionWithConversation(t, provider)
	assertDoesNotShow(t, model, ui.CursorMarker)

	press(t, model, "j")

	assertOnlyMarked(t, model, provider.conversation[0])
}

func TestJMovesToTheNextCommentAndKMovesBackUntilNothingIsSelected(t *testing.T) {
	provider := newFakeProvider()
	model := descriptionWithConversation(t, provider)

	press(t, model, "j", "j")
	assertOnlyMarked(t, model, provider.conversation[1])
	press(t, model, "k")
	assertOnlyMarked(t, model, provider.conversation[0])
	press(t, model, "k", "k")

	assertDoesNotShow(t, model, ui.CursorMarker)
}

func TestJStopsAtTheLastComment(t *testing.T) {
	provider := newFakeProvider()
	model := descriptionWithConversation(t, provider)

	press(t, model, slices.Repeat([]string{"j"}, len(provider.conversation)+3)...)

	assertOnlyMarked(t, model, provider.conversation[len(provider.conversation)-1])
}

func TestJAndKDoNothingWithoutComments(t *testing.T) {
	model := loadedBoardWith(t, newFakeProvider())
	press(t, model, "p")
	before := view(model)

	press(t, model, "j", "k")

	if view(model) != before {
		t.Fatalf("before:\n%s\nafter:\n%s", before, view(model))
	}
}

func TestArrowKeysScrollWithoutChangingTheSelectedComment(t *testing.T) {
	provider := newFakeProvider()
	provider.description = strings.Repeat("line of the description\n\n", 40)
	model := descriptionWithConversation(t, provider)
	plugintest.Resize(t, model, 100, 20)
	press(t, model, "end", "j")
	selected := view(model)

	press(t, model, "up")
	scrolled := view(model)
	press(t, model, "down", "j")

	if scrolled == selected {
		t.Fatalf("the up arrow did not scroll:\n%s", selected)
	}
	assertOnlyMarked(t, model, provider.conversation[1])
}

func TestJScrollsALowerCommentIntoView(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = nil
	for number := 1; number <= 20; number++ {
		provider.conversation = append(provider.conversation, ConversationEntry{
			Author: "person" + strings.Repeat("x", number), Body: strings.Repeat("word ", number*3), Created: time.Now(),
		})
	}
	last := provider.conversation[len(provider.conversation)-1]
	model := loadedBoardWith(t, provider)
	plugintest.Resize(t, model, 100, 20)
	press(t, model, "p")
	assertDoesNotShow(t, model, last.Author)

	press(t, model, slices.Repeat([]string{"j"}, len(provider.conversation))...)

	assertOnlyMarked(t, model, last)
}

func TestSelectingAnEntryKeepsItsHeaderAndTopOfItsBodyOnScreen(t *testing.T) {
	provider := newFakeProvider()
	provider.description = strings.Repeat("line of the description\n\n", 40)
	provider.conversation = fixtureConversation()
	provider.conversation[0].Body = "A remark that should be visible"
	model := loadedBoardWith(t, provider)
	plugintest.Resize(t, model, 100, 12)
	press(t, model, "p")
	assertDoesNotShow(t, model, provider.conversation[0].Body)

	press(t, model, "j")

	assertOnlyMarked(t, model, provider.conversation[0])
	assertShows(t, model, provider.conversation[0].Body)
}

func TestCommentOnASelectedEntryOpensAQuoteOfItsFirstFiveNonEmptyLines(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = fixtureConversation()
	lines := []string{"one", "", "two", "three", "four", "five", "six"}
	provider.conversation[0].Body = strings.Join(lines, "\n")
	session := editorWrites(t, "")
	model := loadedBoardWith(t, provider)
	press(t, model, "p")

	press(t, model, "j", "c")

	expected := "@" + provider.conversation[0].Author + "\n\n> one\n> two\n> three\n> four\n> five\n> …"
	if len(session.opened) != 1 || session.opened[0] != expected {
		t.Fatalf("the editor was opened with %q, expected %q", session.opened, expected)
	}
}

func TestCommentOnAShortEntryQuotesItWithoutAnEllipsis(t *testing.T) {
	provider := newFakeProvider()
	session := editorWrites(t, "")
	model := descriptionWithConversation(t, provider)

	press(t, model, "j", "c")

	entry := provider.conversation[0]
	if expected := "@" + entry.Author + "\n\n> " + entry.Body; len(session.opened) != 1 || session.opened[0] != expected {
		t.Fatalf("the editor was opened with %q, expected %q", session.opened, expected)
	}
}

func TestCommentWithNothingSelectedOpensAnEmptyEditor(t *testing.T) {
	session := editorWrites(t, "")
	model := descriptionWithConversation(t, newFakeProvider())

	press(t, model, "c")

	if len(session.opened) != 1 || session.opened[0] != "" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestSavingAReplyOffersPostingAndOtherPluginsButNotTheReview(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = fixtureConversation()
	recipient := newCommentRecipient()
	editorWrites(t, "Thanks")
	model := modelWithRecipient(t, provider, recipient)

	press(t, model, "p", "j", "c")

	assertShows(t, model, commentQuestion, postOptionLine, "2 "+recipient.label)
	assertDoesNotShow(t, model, addToReviewOption)
	assertCalls(t, provider)
}

func TestSavingAReplyWithNothingOfferedPostsItAtOnce(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Thanks")
	model := descriptionWithConversation(t, provider)

	press(t, model, "j", "c")

	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), replyTargetFor(provider.conversation[0]), "Thanks"))
	assertShows(t, model, "Comment posted.")
}

func TestPostingANewCommentUsesAConversationTargetWithoutAReply(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "A new remark")
	model := descriptionWithConversation(t, provider)

	press(t, model, "c")

	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), CommentTarget{Conversation: true}, "A new remark"))
}

func TestPostingLoadsTheConversationAgainAndKeepsTheSelectedEntry(t *testing.T) {
	provider := newFakeProvider()
	recipient := newCommentRecipient()
	editorWrites(t, "Thanks")
	provider.conversation = fixtureConversation()
	model := modelWithRecipient(t, provider, recipient)
	press(t, model, "p", "j", "j", "c")
	posted := ConversationEntry{Author: "me", Body: "Thanks", Created: time.Now()}
	provider.conversation = append(slices.Clone(provider.conversation), posted)

	press(t, model, "1")

	assertShows(t, model, posted.Body)
	assertOnlyMarked(t, model, provider.conversation[1])
}

func TestChoosingAnotherPluginsActionHandsItTheReplyTarget(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = fixtureConversation()
	recipient := newCommentRecipient()
	editorWrites(t, "Thanks")
	model := modelWithRecipient(t, provider, recipient)

	press(t, model, "p", "j", "c", "2")

	expected := Comment{PullRequest: provider.pullRequest(1), Target: replyTargetFor(provider.conversation[0]), Body: "Thanks"}
	if len(recipient.received) != 1 || !reflect.DeepEqual(recipient.received[0], expected) {
		t.Fatalf("the action received %v, expected %v", recipient.received, expected)
	}
	assertCalls(t, provider)
}

func TestEscapingTheChoiceKeepsTheReplyForTheNextComment(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = fixtureConversation()
	session := editorWrites(t, "Thanks", "")
	model := modelWithRecipient(t, provider, newCommentRecipient())

	press(t, model, "p", "j", "c", "esc")
	assertShows(t, model, commentKeptMessage)
	press(t, model, "c")

	if len(session.opened) != 2 || session.opened[1] != "Thanks" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
	assertCalls(t, provider)
}

func TestKeptReplyIsNotOfferedForAnotherAuthor(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = fixtureConversation()
	session := editorWrites(t, "Thanks", "")
	model := modelWithRecipient(t, provider, newCommentRecipient())

	press(t, model, "p", "j", "c", "esc", "j", "c")

	entry := provider.conversation[1]
	if expected := "@" + entry.Author + "\n\n> " + entry.Body; len(session.opened) != 2 || session.opened[1] != expected {
		t.Fatalf("the editor was opened with %q, expected %q", session.opened, expected)
	}
}

func TestFailedReplyIsKeptForTheNextComment(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("PostComment", errorForTests)
	session := editorWrites(t, "Thanks", "")
	model := descriptionWithConversation(t, provider)

	press(t, model, "j", "c")
	assertShows(t, model, "Failed: "+errorForTests.Error())
	press(t, model, "c")

	if len(session.opened) != 2 || session.opened[1] != "Thanks" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestSavingTheQuoteUnchangedPostsNothing(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "")
	model := descriptionWithConversation(t, provider)

	press(t, model, "j", "c")

	assertCalls(t, provider)
	assertShows(t, model, "Comment discarded.")
}

func TestHelpLineSaysCommentUntilAnEntryIsSelectedThenReply(t *testing.T) {
	model := descriptionWithConversation(t, newFakeProvider())
	assertShows(t, model, "j/k comment", "↑/↓ scroll", "c comment")
	assertDoesNotShow(t, model, "c reply")

	press(t, model, "j")

	assertShows(t, model, "c reply")
}
