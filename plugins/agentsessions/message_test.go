package agentsessions

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// prefillFor is what the editor opens with for the pull request, without the empty line to write on.
func prefillFor(fixture boardFixture) string {
	pullRequest := fixture.signIn
	return fmt.Sprintf("About pull request %s/%s#%d: %s\nURL: %s\nBranch: %s",
		pullRequest.Repository.Owner, pullRequest.Repository.Name, pullRequest.Number, pullRequest.Title, pullRequest.URL, pullRequest.HeadBranch)
}

func TestMessageActionSaysSoWhenNoSessionMentionsThePullRequest(t *testing.T) {
	fixture := newBoardFixture()
	provider := &fakeProvider{sessions: []Session{fixture.mentioning("storefront-1", fixture.signIn)}}
	model := fixture.start(t, provider)
	editor := editorWrites(t, "anything")

	plugintest.Press(t, model, "down", "c")

	if status := statusLine(model); !strings.Contains(status, "No live Claude session mentions this pull request") {
		t.Fatalf("status = %q", status)
	}
	if len(editor.opened) != 0 || len(provider.sent) != 0 {
		t.Fatalf("editor opened %d times, %d messages sent", len(editor.opened), len(provider.sent))
	}
}

func TestMessageActionOpensTheEditorOnTheLineNamingThePullRequest(t *testing.T) {
	fixture := newBoardFixture()
	model := fixture.start(t, &fakeProvider{sessions: []Session{fixture.mentioning("storefront-1", fixture.signIn)}})
	editor := editorWrites(t, "")

	plugintest.Press(t, model, "c")

	if len(editor.opened) != 1 || editor.opened[0] != prefillFor(fixture)+"\n\n" {
		t.Fatalf("editor opened with %q", editor.opened)
	}
}

func TestMessageActionSendsTheEditedTextToTheSession(t *testing.T) {
	fixture := newBoardFixture()
	session := fixture.mentioning("storefront-1", fixture.signIn)
	provider := &fakeProvider{sessions: []Session{session}}
	model := fixture.start(t, provider)
	edited := prefillFor(fixture) + "\n\nPlease rename the helper."
	editorWrites(t, edited)

	plugintest.Press(t, model, "c")

	if len(provider.sent) != 1 || provider.sent[0].session.ID != session.ID || provider.sent[0].text != edited {
		t.Fatalf("sent = %+v, expected %q to %s", provider.sent, edited, session.Name)
	}
}

func TestMessageActionSendsNothingWhenTheTextIsSavedUnchanged(t *testing.T) {
	fixture := newBoardFixture()
	provider := &fakeProvider{sessions: []Session{fixture.mentioning("storefront-1", fixture.signIn)}}
	model := fixture.start(t, provider)
	editorWrites(t, "")

	plugintest.Press(t, model, "c")

	if len(provider.sent) != 0 || !strings.Contains(statusLine(model), "Nothing sent") {
		t.Fatalf("sent = %+v, status = %q", provider.sent, statusLine(model))
	}
}

func TestMessageActionSendsNothingWhenTheTextIsEmptied(t *testing.T) {
	fixture := newBoardFixture()
	provider := &fakeProvider{sessions: []Session{fixture.mentioning("storefront-1", fixture.signIn)}}
	model := fixture.start(t, provider)
	editorWrites(t, " \n")

	plugintest.Press(t, model, "c")

	if len(provider.sent) != 0 || !strings.Contains(statusLine(model), "Nothing sent") {
		t.Fatalf("sent = %+v, status = %q", provider.sent, statusLine(model))
	}
}

func TestMessageActionAsksWhichSessionWhenSeveralMatchAndSendsToTheChosenOne(t *testing.T) {
	fixture := newBoardFixture()
	first := fixture.mentioning("storefront-1", fixture.signIn)
	second := fixture.mentioning("storefront-2", fixture.signIn)
	second.LastActive = first.LastActive.Add(-1)
	second.Status = StatusBusy
	provider := &fakeProvider{sessions: []Session{first, second}}
	model := fixture.start(t, provider)
	edited := prefillFor(fixture) + "\n\nLooks good."
	editorWrites(t, edited)

	plugintest.Press(t, model, "c")

	status := statusLine(model)
	if !strings.Contains(status, "Message which session?") || !strings.Contains(status, first.Name) || !strings.Contains(status, second.Name+" busy") {
		t.Fatalf("status = %q", status)
	}
	if len(provider.sent) != 0 {
		t.Fatalf("sent = %+v before a session was chosen", provider.sent)
	}

	plugintest.Press(t, model, "2")

	if len(provider.sent) != 1 || provider.sent[0].session.ID != second.ID || provider.sent[0].text != edited {
		t.Fatalf("sent = %+v, expected %q to %s", provider.sent, edited, second.Name)
	}
}

func TestMessageActionShowsTheFailureWhenTheSendFails(t *testing.T) {
	fixture := newBoardFixture()
	provider := &fakeProvider{sessions: []Session{fixture.mentioning("storefront-1", fixture.signIn)}, sendErr: errors.New("no such session")}
	model := fixture.start(t, provider)
	editorWrites(t, prefillFor(fixture)+"\n\nHello.")

	plugintest.Press(t, model, "c")

	if status := statusLine(model); !strings.Contains(status, "Failed: no such session") {
		t.Fatalf("status = %q", status)
	}
}
