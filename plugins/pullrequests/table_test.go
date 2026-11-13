package pullrequests

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/ui"
)

func TestReviewColumnNamesViewerAndTeamBeforeOtherReviewers(t *testing.T) {
	m := loadedBoard(t)

	review := ui.PlainText(m.visible[1].reviewSegments(m.viewer, m.reviewTeam()))

	if review != "you, engineering, someone-else" {
		t.Fatalf("review column = %q", review)
	}
}

func TestReviewColumnDoesNotSingleOutATeamWhenNoReviewTeamIsConfigured(t *testing.T) {
	provider := newFakeProvider()
	provider.reviewTeam = ""
	m := loadedBoardWith(t, provider)

	review := ui.PlainText(m.visible[1].reviewSegments(m.viewer, m.reviewTeam()))

	if review != "you, someone-else, "+testReviewTeam {
		t.Fatalf("review column = %q", review)
	}
}

func TestStateMarksCoverEachState(t *testing.T) {
	passing, running, failing := CIStatePassing, CIStateRunning, CIStateFailing
	cases := map[string]PullRequest{
		"📝 🟢":    {IsDraft: true, CIState: &passing},
		"✅ ♻️ 🟢": {ReviewDecision: ReviewDecisionApproved, InMergeQueue: true, CIState: &passing},
		"❌ 🟡":    {ReviewDecision: ReviewDecisionChangesRequested, CIState: &running},
		"✋ ⏩ 🟢":  {Author: "person", RequestedUsers: []string{testViewer}, AutoMerge: true, CIState: &passing},
		"🟢":      {Author: "person", RequestedUsers: []string{"someone-else"}, CIState: &passing},
		"🔴":      {CIState: &failing},
	}

	for expected, pr := range cases {
		if marks := pr.stateMarks(testViewer, testReviewTeam); marks != expected {
			t.Errorf("state marks = %q, expected %q", marks, expected)
		}
	}
}

func TestStateMarksShowAnEllipsisUntilTheCIStateIsLoaded(t *testing.T) {
	notLoaded := PullRequest{}
	none := CIStateNone
	withoutChecks := PullRequest{CIState: &none}

	if marks := notLoaded.stateMarks(testViewer, testReviewTeam); marks != "…" {
		t.Fatalf("state marks before loading = %q", marks)
	}
	if marks := withoutChecks.stateMarks(testViewer, testReviewTeam); marks != "" {
		t.Fatalf("state marks without checks = %q", marks)
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

func TestTableDrawsTheHeaderAndOneRowPerPullRequestWithoutACursorWhenNoneIsSelected(t *testing.T) {
	m := loadedBoard(t)

	table := ansi.Strip(renderTable(m.visible, m.viewer, m.reviewTeam(), 200, -1, 0, len(m.visible)))

	lines := strings.Split(table, "\n")
	if len(lines) != 1+len(m.visible) {
		t.Fatalf("table has %d lines:\n%s", len(lines), table)
	}
	for _, header := range columnHeaders {
		if !strings.Contains(lines[0], header) {
			t.Fatalf("header %q is missing %q", lines[0], header)
		}
	}
}

func TestIntervalTextUsesWholeMinutesWhenPossibleAndSecondsOtherwise(t *testing.T) {
	cases := map[time.Duration]string{
		time.Second:      "second",
		2 * time.Second:  "2 seconds",
		time.Minute:      "minute",
		7 * time.Minute:  "7 minutes",
		90 * time.Second: "90 seconds",
		30 * time.Second: "30 seconds",
	}

	for interval, expected := range cases {
		if text := intervalText(interval); text != expected {
			t.Errorf("intervalText(%v) = %q, expected %q", interval, text, expected)
		}
	}
}
