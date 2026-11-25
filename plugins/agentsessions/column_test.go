package agentsessions

import (
	"strings"
	"testing"
	"time"
)

func TestSessionColumnNamesTheSessionThatHasMentionedThePullRequest(t *testing.T) {
	fixture := newBoardFixture()
	session := fixture.mentioning("storefront-1", fixture.signIn)

	model := fixture.start(t, &fakeProvider{sessions: []Session{session}})

	if header := lineWith(model, "Review requested"); !strings.Contains(header, columnHeader) {
		t.Fatalf("header = %q", header)
	}
	if line := lineWith(model, fixture.signIn.Title); !strings.Contains(line, session.Name) {
		t.Fatalf("line = %q, expected the session %q", line, session.Name)
	}
	if line := lineWith(model, fixture.tidy.Title); strings.Contains(line, session.Name) {
		t.Fatalf("line = %q, expected no session for the other pull request", line)
	}
}

func TestSessionColumnNamesTheSessionOnThePullRequestsBranchInTheSameRepository(t *testing.T) {
	fixture := newBoardFixture()
	session := onBranch("storefront-2", fixture.source.Name, fixture.signIn.HeadBranch)

	model := fixture.start(t, &fakeProvider{sessions: []Session{session}})

	if line := lineWith(model, fixture.signIn.Title); !strings.Contains(line, session.Name) {
		t.Fatalf("line = %q, expected the session %q", line, session.Name)
	}
}

func TestSessionColumnIgnoresASessionOnTheSameBranchInAnotherRepository(t *testing.T) {
	fixture := newBoardFixture()
	session := onBranch("warehouse-1", "warehouse", fixture.signIn.HeadBranch)

	model := fixture.start(t, &fakeProvider{sessions: []Session{session}})

	if line := lineWith(model, fixture.signIn.Title); line == "" || strings.Contains(line, session.Name) {
		t.Fatalf("line = %q, expected the pull request without the session", line)
	}
}

func TestSessionColumnIgnoresASessionWithoutABranchWhoseRepositoryIsUnknown(t *testing.T) {
	fixture := newBoardFixture()
	session := Session{ID: "id-scratch", Name: "scratch-1"}
	fixture.signIn.HeadBranch = ""

	model := fixture.start(t, &fakeProvider{sessions: []Session{session}})

	if line := lineWith(model, fixture.signIn.Title); strings.Contains(line, session.Name) {
		t.Fatalf("line = %q", line)
	}
}

func TestSessionColumnShowsTheStatusOfASessionThatIsNotIdle(t *testing.T) {
	fixture := newBoardFixture()
	busy := fixture.mentioning("storefront-1", fixture.signIn)
	busy.Status = StatusBusy
	idle := fixture.mentioning("storefront-2", fixture.tidy)

	model := fixture.start(t, &fakeProvider{sessions: []Session{busy, idle}})

	if line := lineWith(model, fixture.signIn.Title); !strings.Contains(line, busy.Name+" busy") {
		t.Fatalf("line = %q", line)
	}
	if line := lineWith(model, fixture.tidy.Title); !strings.Contains(line, idle.Name) || strings.Contains(line, idle.Name+" idle") {
		t.Fatalf("line = %q, expected the name without a status", line)
	}
}

func TestSessionColumnShowsTheMostRecentlyActiveSessionFirstAndCountsTheOthers(t *testing.T) {
	fixture := newBoardFixture()
	older := fixture.mentioning("storefront-1", fixture.signIn)
	newer := fixture.mentioning("storefront-2", fixture.signIn)
	newer.LastActive = older.LastActive.Add(time.Hour)

	model := fixture.start(t, &fakeProvider{sessions: []Session{older, newer}})

	line := lineWith(model, fixture.signIn.Title)
	if !strings.Contains(line, newer.Name+" +1") || strings.Contains(line, older.Name) {
		t.Fatalf("line = %q, expected %q followed by +1", line, newer.Name)
	}
}
