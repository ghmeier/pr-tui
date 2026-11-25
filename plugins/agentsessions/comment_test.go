package agentsessions

import (
	"flag"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

// commentProbe stands in for the diff screen: a screen opened from a key on the pull request
// board that lists what the board offers for a comment, and sends the comment to the action with
// the label when it receives "o".
type commentProbe struct {
	comment pullrequests.Comment
	// value is what is offered instead of the comment, when it is set.
	value   any
	label   string
	offered []string
}

func (*commentProbe) Name() string { return "probe" }

func (*commentProbe) Flags(*flag.FlagSet) {}

func (probe *commentProbe) Setup(host *plugin.Host) error {
	host.AddAction(pullrequests.BoardName, plugin.Action{Key: "z", Run: func(selection plugin.Selection) plugin.Effect {
		return plugin.Effect{Screen: &probeScreen{probe: probe, selection: selection}}
	}})
	return nil
}

type probeScreen struct {
	probe     *commentProbe
	selection plugin.Selection
}

func (*probeScreen) Init() tea.Cmd { return nil }

func (screen *probeScreen) Update(message tea.Msg) (plugin.Screen, tea.Cmd) {
	if key, isKey := message.(tea.KeyMsg); isKey {
		switch key.String() {
		case "g":
			screen.probe.offered = screen.selection.Offers(pullrequests.CommentKind)
		case "o":
			var value any = screen.probe.comment
			if screen.probe.value != nil {
				value = screen.probe.value
			}
			return screen, screen.selection.RunOffer(screen.probe.label, value)
		case "esc":
			return nil, nil
		}
	}
	return screen, nil
}

func (*probeScreen) View(int, int) string { return "probe screen" }

func (fixture boardFixture) commentOn(pullRequest pullrequests.PullRequest, body string) pullrequests.Comment {
	return pullrequests.Comment{
		PullRequest: pullRequest,
		Target:      pullrequests.CommentTarget{Path: "src/sign_in.py", Line: 42, Side: pullrequests.CommentSideNew},
		Body:        body,
	}
}

func (fixture boardFixture) replyTo(pullRequest pullrequests.PullRequest, author, body string) pullrequests.Comment {
	comment := fixture.commentOn(pullRequest, body)
	comment.Target.ReplyTo = 7
	comment.Target.ReplyAuthor = author
	return comment
}

func TestCommentMessageNamesThePullRequestTheLineAndHasTheCommentLast(t *testing.T) {
	fixture := newBoardFixture()
	comment := fixture.commentOn(fixture.signIn, "Please rename this.")
	target := comment.Target

	text := commentMessage(comment)

	expected := prefillFor(fixture) + fmt.Sprintf("\nFile: %s, line %d (new side)\n\n%s", target.Path, target.Line, comment.Body)
	if text != expected {
		t.Fatalf("message = %q, expected %q", text, expected)
	}
}

func TestCommentMessageOnTheOldSideSaysSo(t *testing.T) {
	fixture := newBoardFixture()
	comment := fixture.commentOn(fixture.signIn, "Why remove this?")
	comment.Target.Side = pullrequests.CommentSideOld

	text := commentMessage(comment)

	if !strings.Contains(text, fmt.Sprintf("File: %s, line %d (old side)", comment.Target.Path, comment.Target.Line)) {
		t.Fatalf("message = %q", text)
	}
}

func TestCommentMessageForAReplyNamesWhoseCommentItAnswers(t *testing.T) {
	fixture := newBoardFixture()
	comment := fixture.replyTo(fixture.signIn, "reviewer", "Done.")
	target := comment.Target

	text := commentMessage(comment)

	expected := prefillFor(fixture) + fmt.Sprintf("\nFile: %s, line %d (new side)\nIn reply to %s's comment\n\n%s", target.Path, target.Line, target.ReplyAuthor, comment.Body)
	if text != expected {
		t.Fatalf("message = %q, expected %q", text, expected)
	}
}

func TestCommentMessageForANewCommentSaysNothingAboutAReply(t *testing.T) {
	fixture := newBoardFixture()

	text := commentMessage(fixture.commentOn(fixture.signIn, "Fine."))

	if strings.Contains(text, "In reply to") {
		t.Fatalf("message = %q", text)
	}
}

// probedModel opens the probe screen on the first pull request, which is the sign-in one.
func probedModel(t *testing.T, fixture boardFixture, provider *fakeProvider, probe *commentProbe) *plugin.Model {
	t.Helper()
	model := fixture.startWith(t, provider, probe)
	plugintest.Press(t, model, "z")
	return model
}

func TestCommentIsNotOfferedWhenNoSessionWorksOnThePullRequest(t *testing.T) {
	fixture := newBoardFixture()
	probe := &commentProbe{}
	provider := &fakeProvider{sessions: []Session{fixture.mentioning("storefront-1", fixture.tidy)}}
	model := probedModel(t, fixture, provider, probe)

	plugintest.Press(t, model, "g")

	if len(probe.offered) != 0 {
		t.Fatalf("offered = %v", probe.offered)
	}
}

func TestCommentIsOfferedWhenASessionWorksOnThePullRequest(t *testing.T) {
	fixture := newBoardFixture()
	probe := &commentProbe{}
	provider := &fakeProvider{sessions: []Session{fixture.mentioning("storefront-1", fixture.signIn)}}
	model := probedModel(t, fixture, provider, probe)

	plugintest.Press(t, model, "g")

	if expected := []string{"send to agent session"}; !slices.Equal(probe.offered, expected) {
		t.Fatalf("offered = %v, expected %v", probe.offered, expected)
	}
}

func TestOfferedCommentGoesToTheOnlySessionAtOnceWithoutAnEditor(t *testing.T) {
	fixture := newBoardFixture()
	session := fixture.mentioning("storefront-1", fixture.signIn)
	provider := &fakeProvider{sessions: []Session{session}}
	probe := &commentProbe{label: "send to agent session", comment: fixture.commentOn(fixture.signIn, "Please rename this.")}
	model := probedModel(t, fixture, provider, probe)
	editor := editorWrites(t, "")

	plugintest.Press(t, model, "o")

	if len(provider.sent) != 1 || provider.sent[0].session.ID != session.ID || provider.sent[0].text != commentMessage(probe.comment) {
		t.Fatalf("sent = %+v", provider.sent)
	}
	if len(editor.opened) != 0 {
		t.Fatalf("the editor opened with %q", editor.opened)
	}
}

func TestOfferedCommentAsksWhichSessionWhenSeveralWorkOnThePullRequest(t *testing.T) {
	fixture := newBoardFixture()
	first := fixture.mentioning("storefront-1", fixture.signIn)
	second := fixture.mentioning("storefront-2", fixture.signIn)
	second.LastActive = first.LastActive.Add(-1)
	provider := &fakeProvider{sessions: []Session{first, second}}
	probe := &commentProbe{label: "send to agent session", comment: fixture.commentOn(fixture.signIn, "Please rename this.")}
	model := probedModel(t, fixture, provider, probe)

	plugintest.Press(t, model, "o")
	asked := screenLines(model)[len(screenLines(model))-1]
	sentBeforeAnswer := len(provider.sent)
	plugintest.Press(t, model, "2")

	if !strings.Contains(asked, "Send to which session?") || !strings.Contains(asked, first.Name) || !strings.Contains(asked, second.Name) || sentBeforeAnswer != 0 {
		t.Fatalf("status = %q, sent before the answer = %d", asked, sentBeforeAnswer)
	}
	if len(provider.sent) != 1 || provider.sent[0].session.ID != second.ID || provider.sent[0].text != commentMessage(probe.comment) {
		t.Fatalf("sent = %+v", provider.sent)
	}
}

func TestOfferedCommentIsNotSentWhenNoSessionWorksOnThePullRequest(t *testing.T) {
	fixture := newBoardFixture()
	provider := &fakeProvider{sessions: []Session{fixture.mentioning("storefront-1", fixture.tidy)}}
	probe := &commentProbe{label: "send to agent session", comment: fixture.commentOn(fixture.signIn, "Please rename this.")}
	model := probedModel(t, fixture, provider, probe)

	plugintest.Press(t, model, "o")

	if len(provider.sent) != 0 {
		t.Fatalf("sent = %+v", provider.sent)
	}
}

func TestEscapingTheSessionChoiceSaysNothingWasSent(t *testing.T) {
	fixture := newBoardFixture()
	first := fixture.mentioning("storefront-1", fixture.signIn)
	second := fixture.mentioning("storefront-2", fixture.signIn)
	provider := &fakeProvider{sessions: []Session{first, second}}
	probe := &commentProbe{label: "send to agent session", comment: fixture.commentOn(fixture.signIn, "Please rename this.")}
	model := probedModel(t, fixture, provider, probe)

	plugintest.Press(t, model, "o", "esc", "esc")

	if len(provider.sent) != 0 {
		t.Fatalf("sent = %+v", provider.sent)
	}
	if lines := strings.Join(screenLines(model), "\n"); !strings.Contains(lines, "Nothing sent") {
		t.Fatalf("screen:\n%s", lines)
	}
}

func TestOfferedValueThatIsNotACommentIsNotSentAndSaysSo(t *testing.T) {
	fixture := newBoardFixture()
	provider := &fakeProvider{sessions: []Session{fixture.mentioning("storefront-1", fixture.signIn)}}
	probe := &commentProbe{label: "send to agent session", value: "not a comment"}
	model := probedModel(t, fixture, provider, probe)

	plugintest.Press(t, model, "o", "esc")

	if len(provider.sent) != 0 {
		t.Fatalf("sent = %+v", provider.sent)
	}
	if lines := strings.Join(screenLines(model), "\n"); !strings.Contains(lines, "this is not a comment") {
		t.Fatalf("screen:\n%s", lines)
	}
}
