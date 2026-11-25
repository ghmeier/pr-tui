package pullrequests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

func fixtureConversation() []ConversationEntry {
	now := time.Now()
	return []ConversationEntry{
		{Author: "person", Body: "Ready for another look.", Created: now.Add(-3 * time.Hour)},
		{Author: "reviewer", Body: "Please rename this.", Created: now.Add(-2 * 24 * time.Hour), Review: ReviewStateChangesRequested},
		{Author: "approver", Body: "Looks good now.", Created: now.Add(-30 * time.Minute), Review: ReviewStateApproved},
		{Author: "commenter", Body: "Only a note.", Created: now.Add(-time.Hour), Review: ReviewStateCommented},
	}
}

// descriptionOpenedBeforeItsData opens the description screen without delivering what it loads,
// and hands back the messages the provider would have answered with.
func descriptionOpenedBeforeItsData(t *testing.T, provider *fakeProvider) (model *plugin.Model, loaded descriptionLoadedMessage, conversation func() conversationLoadedMessage) {
	t.Helper()
	model = loadedBoardWith(t, provider)
	model.Update(keyMessage("p"))
	pr := provider.pullRequest(1)
	description, _ := provider.Description(pr)
	loaded = descriptionLoadedMessage{pullRequest: pr, description: description}
	conversation = func() conversationLoadedMessage {
		entries, err := provider.Conversation(pr)
		return conversationLoadedMessage{pullRequest: pr, entries: entries, err: err}
	}
	return model, loaded, conversation
}

func TestDescriptionShowsBeforeTheCommentsHaveArrived(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = fixtureConversation()
	model, loaded, _ := descriptionOpenedBeforeItsData(t, provider)

	plugintest.Send(t, model, loaded)

	shown := view(model)
	if !strings.Contains(shown, "Original description") || !strings.Contains(shown, "Loading comments…") {
		t.Fatalf("view:\n%s", shown)
	}
	if strings.Contains(shown, fixtureConversation()[0].Body) {
		t.Fatalf("view has comments that have not arrived:\n%s", shown)
	}
}

func TestCommentsShowAuthorAgeAndReviewStateInTheirHeadings(t *testing.T) {
	provider := newFakeProvider()
	entries := fixtureConversation()
	provider.conversation = entries
	model := loadedBoardWith(t, provider)

	press(t, model, "p")

	shown := view(model)
	for _, expected := range []string{
		"Comments (4)", entries[0].Author + " · 3h", entries[1].Author + " · 2d · requested changes",
		entries[2].Author + " · 30m · approved", entries[3].Author + " · 1h · reviewed",
		entries[0].Body, entries[1].Body, entries[2].Body, entries[3].Body,
	} {
		if !strings.Contains(shown, expected) {
			t.Errorf("view lacks %q:\n%s", expected, shown)
		}
	}
}

func TestAPlainCommentHasNoReviewStateInItsHeading(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = []ConversationEntry{{Author: "plain-commenter", Body: "A note.", Created: time.Now().Add(-time.Hour)}}
	model := loadedBoardWith(t, provider)

	press(t, model, "p")

	heading := lineWith(t, model, provider.conversation[0].Author)
	if strings.Count(heading, " · ") != 1 {
		t.Fatalf("heading %q has more than the age", heading)
	}
}

func TestCommentsAreShownInTheOrderTheProviderGivesThem(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = fixtureConversation()
	model := loadedBoardWith(t, provider)

	press(t, model, "p")

	shown := view(model)
	if strings.Index(shown, provider.conversation[0].Body) > strings.Index(shown, provider.conversation[1].Body) {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestDescriptionScreenSaysNoCommentsOnlyWhenThereAreNone(t *testing.T) {
	withComments := newFakeProvider()
	withComments.conversation = fixtureConversation()
	withoutComments := loadedBoard(t)
	withCommentsModel := loadedBoardWith(t, withComments)

	press(t, withoutComments, "p")
	press(t, withCommentsModel, "p")

	if !strings.Contains(view(withoutComments), "No comments") || strings.Contains(view(withCommentsModel), "No comments") {
		t.Fatalf("without:\n%s\nwith:\n%s", view(withoutComments), view(withCommentsModel))
	}
}

func TestCommentsThatFailToLoadAreReportedWhileTheDescriptionStillShows(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("Conversation", errorForTests)
	model := loadedBoardWith(t, provider)

	press(t, model, "p")

	shown := view(model)
	if !strings.Contains(shown, "Comments failed to load: "+errorForTests.Error()) || !strings.Contains(shown, "Original description") {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestScrollingToTheEndReachesTheLastComment(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = nil
	for number := 0; number < 20; number++ {
		provider.conversation = append(provider.conversation, ConversationEntry{
			Author: "person", Body: strings.Repeat("word ", number+1), Created: time.Now(),
		})
	}
	provider.conversation[len(provider.conversation)-1].Body = "The final remark."
	last := provider.conversation[len(provider.conversation)-1].Body
	model := loadedBoardWith(t, provider)
	plugintest.Resize(t, model, 100, 20)
	press(t, model, "p")
	before := view(model)

	press(t, model, "end")

	if strings.Contains(before, last) || !strings.Contains(view(model), last) {
		t.Fatalf("before:\n%s\nafter:\n%s", before, view(model))
	}
}

func longDescriptionBoard(t *testing.T) *plugin.Model {
	t.Helper()
	provider := newFakeProvider()
	var body strings.Builder
	for number := 1; number <= 60; number++ {
		fmt.Fprintf(&body, "Paragraph %03d\n\n", number)
	}
	provider.description = body.String()
	model := loadedBoardWith(t, provider)
	press(t, model, "p")
	return model
}

func wheel(t *testing.T, model *plugin.Model, button tea.MouseButton, action tea.MouseAction) {
	t.Helper()
	plugintest.Send(t, model, tea.MouseMsg{Button: button, Action: action})
}

func TestDescriptionScreenWheelScrollsByNotchesAndStopsAtBothEnds(t *testing.T) {
	model := longDescriptionBoard(t)
	top := strings.Split(view(model), "\n")

	wheel(t, model, tea.MouseButtonWheelUp, tea.MouseActionPress)
	if view(model) != strings.Join(top, "\n") {
		t.Fatalf("wheel up at the top moved the view:\n%s", view(model))
	}
	wheel(t, model, tea.MouseButtonWheelDown, tea.MouseActionPress)
	if strings.Split(view(model), "\n")[1] != top[1+ui.WheelNotchLines] {
		t.Fatalf("wheel down did not move by a notch:\n%s", view(model))
	}
	wheel(t, model, tea.MouseButtonWheelUp, tea.MouseActionPress)
	if view(model) != strings.Join(top, "\n") {
		t.Fatalf("wheel up did not return to the top:\n%s", view(model))
	}
	press(t, model, "end")
	bottom := view(model)
	wheel(t, model, tea.MouseButtonWheelDown, tea.MouseActionPress)
	if view(model) != bottom {
		t.Fatalf("wheel down at the end moved the view:\n%s", view(model))
	}
}

func TestDescriptionScreenIgnoresMouseEventsThatAreNotWheelPresses(t *testing.T) {
	model := longDescriptionBoard(t)
	top := view(model)

	wheel(t, model, tea.MouseButtonLeft, tea.MouseActionPress)
	wheel(t, model, tea.MouseButtonWheelDown, tea.MouseActionRelease)

	if view(model) != top {
		t.Fatalf("the view moved:\n%s", view(model))
	}
}

func TestCommentsArrivingLeaveTheLinesBeingReadWhereTheyWere(t *testing.T) {
	provider := newFakeProvider()
	provider.description = strings.Repeat("line of the description\n\n", 40)
	provider.conversation = fixtureConversation()
	model, loaded, conversation := descriptionOpenedBeforeItsData(t, provider)
	plugintest.Resize(t, model, 100, 20)
	plugintest.Send(t, model, loaded)
	press(t, model, "pgdown")
	before := view(model)

	plugintest.Send(t, model, conversation())

	if strings.Contains(before, "Comments") || view(model) != before {
		t.Fatalf("before:\n%s\nafter:\n%s", before, view(model))
	}
}

func TestCommentsAboutAnotherPullRequestAreIgnored(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = fixtureConversation()
	model, loaded, conversation := descriptionOpenedBeforeItsData(t, provider)
	plugintest.Send(t, model, loaded)
	message := conversation()
	message.pullRequest = provider.pullRequest(2)

	plugintest.Send(t, model, message)

	if shown := view(model); !strings.Contains(shown, "Loading comments…") || strings.Contains(shown, message.entries[0].Body) {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestEditingTheDescriptionStartsFromTheDescriptionWithoutTheComments(t *testing.T) {
	provider := newFakeProvider()
	provider.conversation = fixtureConversation()
	session := editorWrites(t, "")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	if len(session.opened) != 1 || session.opened[0] != firstPullRequestText(provider) {
		t.Fatalf("the editor was opened with %q, expected %q", session.opened, firstPullRequestText(provider))
	}
}

func TestSavingTheDescriptionLoadsTheCommentsAgain(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	editorWrites(t, "Edited title\n\nEdited description\n")
	press(t, model, "p")
	provider.conversation = fixtureConversation()[:1]

	press(t, model, "e")

	if shown := view(model); !strings.Contains(shown, provider.conversation[0].Body) || strings.Contains(shown, "No comments") {
		t.Fatalf("view:\n%s", shown)
	}
}
