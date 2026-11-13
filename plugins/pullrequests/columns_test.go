package pullrequests

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// rowFor is a pull request as the board's row, seen by the fixture's viewer.
func rowFor(pr PullRequest) Row {
	return Row{PullRequest: pr, Viewer: testViewer}
}

func TestReviewColumnNamesViewerAndTeamBeforeOtherReviewers(t *testing.T) {
	row := rowFor(newFakeProvider().pullRequest(3))

	review := ui.PlainText(row.reviewSegments(testReviewTeam))

	if review != "you, engineering, someone-else" {
		t.Fatalf("review column = %q", review)
	}
}

func TestReviewColumnDoesNotSingleOutATeamWhenNoReviewTeamIsConfigured(t *testing.T) {
	row := rowFor(newFakeProvider().pullRequest(3))

	review := ui.PlainText(row.reviewSegments(""))

	if review != "you, someone-else, "+testReviewTeam {
		t.Fatalf("review column = %q", review)
	}
}

func TestStateMarksCoverEachState(t *testing.T) {
	passing, running, failing := CIStatePassing, CIStateRunning, CIStateFailing
	type state struct {
		pr PullRequest
		ci CIState
	}
	cases := map[string]state{
		"📝 🟢":    {PullRequest{IsDraft: true}, passing},
		"✅ ♻️ 🟢": {PullRequest{ReviewDecision: ReviewDecisionApproved, InMergeQueue: true}, passing},
		"❌ 🟡":    {PullRequest{ReviewDecision: ReviewDecisionChangesRequested}, running},
		"✋ ⏩ 🟢":  {PullRequest{Author: "person", RequestedUsers: []string{testViewer}, AutoMerge: true}, passing},
		"🟢":      {PullRequest{Author: "person", RequestedUsers: []string{"someone-else"}}, passing},
		"🔴":      {PullRequest{}, failing},
	}

	for expected, given := range cases {
		if marks := rowFor(given.pr).stateMarks(testReviewTeam, given.ci); marks != expected {
			t.Errorf("state marks = %q, expected %q", marks, expected)
		}
	}
}

func TestStateMarksShowAnEllipsisUntilTheCIStateIsLoaded(t *testing.T) {
	row := rowFor(PullRequest{})

	notLoaded := row.stateMarks(testReviewTeam, nil)
	withoutChecks := row.stateMarks(testReviewTeam, CIStateNone)

	if notLoaded != "…" || withoutChecks != "" {
		t.Fatalf("state marks before loading = %q, without checks = %q", notLoaded, withoutChecks)
	}
}

func TestWaitingMarkShowsForAnotherAuthorsPullRequestRequestingTheReviewTeam(t *testing.T) {
	pr := PullRequest{Author: "person", RequestedTeams: []string{testReviewTeam}}

	if !pr.awaitsReviewFrom(testViewer, testReviewTeam) {
		t.Fatalf("a review requested from %s on another author's pull request should wait on the viewer", testReviewTeam)
	}
}

func TestWaitingMarkIsHiddenOnTheViewersOwnPullRequest(t *testing.T) {
	pr := PullRequest{Author: testViewer, RequestedTeams: []string{testReviewTeam}}

	if pr.awaitsReviewFrom(testViewer, testReviewTeam) {
		t.Fatalf("the viewer's own pull request should not wait on the viewer")
	}
}

func TestWaitingMarkIsHiddenWhenOnlyAnotherTeamIsRequested(t *testing.T) {
	pr := PullRequest{Author: "person", RequestedTeams: []string{"platform-data"}}

	if pr.awaitsReviewFrom(testViewer, testReviewTeam) {
		t.Fatalf("a review requested from another team should not wait on the viewer")
	}
}

func TestWaitingMarkIgnoresTheTeamWhenNoReviewTeamIsConfigured(t *testing.T) {
	teamRequested := PullRequest{Author: "person", RequestedTeams: []string{testReviewTeam}}
	viewerRequested := PullRequest{Author: "person", RequestedUsers: []string{testViewer}}

	if teamRequested.awaitsReviewFrom(testViewer, "") {
		t.Fatalf("a team request should not wait on the viewer when no review team is configured")
	}
	if !viewerRequested.awaitsReviewFrom(testViewer, "") {
		t.Fatalf("a request to the viewer should still wait on the viewer")
	}
}

func TestBoardListsPullRequestsFromPeopleInTheOrderTheProviderReturnsThem(t *testing.T) {
	provider := newFakeProvider()

	model := loadedBoardWith(t, provider)

	assertShown(t, model, provider, provider.pullRequest(1), provider.pullRequest(3))
	view := view(model)
	if strings.Index(view, provider.pullRequest(1).Title) > strings.Index(view, provider.pullRequest(3).Title) {
		t.Fatalf("the pull requests are out of order:\n%s", view)
	}
}

func TestBoardHasTheColumnsOfAPullRequestInOrder(t *testing.T) {
	model := loadedBoard(t)

	header := lineWith(t, model, "Review requested")

	last := -1
	for _, name := range []string{"Repo", "#", "Title", "Author", "Lines", "State", "Review requested", "Updated"} {
		position := strings.Index(header, name)
		if position <= last {
			t.Fatalf("header %q has %q out of order", header, name)
		}
		last = position
	}
}

func TestBoardShowsTheRepositoryNumberAndAuthorOfEachPullRequest(t *testing.T) {
	provider := newFakeProvider()
	pr := provider.pullRequest(3)

	line := lineWith(t, loadedBoardWith(t, provider), pr.Title)

	for _, expected := range []string{pr.Repository.Name, fmt.Sprint(pr.Number), pr.Author, "you, engineering, someone-else"} {
		if !strings.Contains(line, expected) {
			t.Fatalf("line %q should contain %q", line, expected)
		}
	}
}

func TestBoardTruncatesTheTitleNotTheOtherColumnsWhenTheTerminalIsNarrow(t *testing.T) {
	provider := newFakeProvider()
	provider.pullRequests[0].Title = strings.Repeat("long title ", 30)
	model := loadedBoardWith(t, provider)

	plugintest.Resize(t, model, 100, 20)

	line := lineWith(t, model, "long title")
	if width := len([]rune(line)); width > 100 || !strings.Contains(line, provider.pullRequests[0].Author) {
		t.Fatalf("line is %d columns wide: %q", width, line)
	}
}

// loadedWithoutColumnData loads the board's rows only, and returns the command that would fetch
// the slow columns, so that a test can look at the board before that data arrives.
func loadedWithoutColumnData(t *testing.T, provider *fakeProvider) (*plugin.Model, tea.Cmd) {
	t.Helper()
	host, err := newSetup(provider).host(t)
	if err != nil {
		t.Fatal(err)
	}
	model := host.Model()
	_, fetches := model.Update(model.RefreshAll()())
	return model, fetches
}

func TestLinesAndStateColumnsAreUnknownBeforeTheirDataArrives(t *testing.T) {
	provider := newFakeProvider()

	model, fetches := loadedWithoutColumnData(t, provider)
	before := lineWith(t, model, provider.pullRequest(1).Title)
	plugintest.Settle(t, model, fetches)
	after := lineWith(t, model, provider.pullRequest(1).Title)

	if !strings.Contains(before, "…") || strings.Contains(before, "+12") || strings.Contains(before, "🔴") {
		t.Fatalf("line before the data arrives = %q", before)
	}
	if strings.Contains(after, "…") || !strings.Contains(after, "+12 −3") || !strings.Contains(after, "📝 🔴") {
		t.Fatalf("line after the data arrives = %q", after)
	}
}

// expectedLines is how the board writes the line count that the fake provider has for its
// first pull request.
func expectedLines(provider *fakeProvider) string {
	count := provider.lineCounts[provider.pullRequest(1).URL]
	return fmt.Sprintf("+%d −%d", count.Additions, count.Deletions)
}

func TestLinesColumnShowsAdditionsAndDeletions(t *testing.T) {
	provider := newFakeProvider()

	line := lineWith(t, loadedBoardWith(t, provider), provider.pullRequest(1).Title)

	if !strings.Contains(line, expectedLines(provider)) {
		t.Fatalf("line = %q, expected it to contain %q", line, expectedLines(provider))
	}
}

func TestLinesColumnStaysUnknownWhenTheProviderCannotCountLines(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("LineCounts", errorForTests)

	line := lineWith(t, loadedBoardWith(t, provider), provider.pullRequest(1).Title)

	if !strings.Contains(line, "…") || strings.Contains(line, expectedLines(provider)) {
		t.Fatalf("line = %q", line)
	}
}

func TestLinesColumnStaysUnknownForAPullRequestTheProviderHasNoCountFor(t *testing.T) {
	provider := newFakeProvider()

	line := lineWith(t, loadedBoardWith(t, provider), provider.pullRequest(3).Title)

	if !strings.Contains(line, "…") {
		t.Fatalf("line = %q", line)
	}
}

func TestStateColumnShowsCIOnceLoadedAndNothingForAPullRequestWithoutChecks(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	failing := lineWith(t, model, provider.pullRequest(1).Title)
	none := lineWith(t, model, provider.pullRequest(3).Title)

	if !strings.Contains(failing, "📝 🔴") || !strings.Contains(none, "✋ ⏩") || strings.Contains(none, "⏩ …") {
		t.Fatalf("lines = %q and %q", failing, none)
	}
}

func TestSlowColumnsAreFetchedOncePerRepositoryOnEachLoad(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "r")

	for _, source := range provider.repositories {
		lines, states := provider.lineCountRequests[source.FullName()], provider.ciStateRequests[source.FullName()]
		if lines != 2 || states != 2 {
			t.Fatalf("%s was asked for line counts %d times and CI states %d times over two loads", source.FullName(), lines, states)
		}
	}
}
