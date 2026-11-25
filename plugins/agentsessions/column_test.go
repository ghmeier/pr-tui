package agentsessions

import (
	"strings"
	"testing"
	"time"
)

func TestSessionColumnMarksThePullRequestThatASessionHasMentionedThePullRequest(t *testing.T) {
	fixture := newBoardFixture()
	session := fixture.mentioning("storefront-1", fixture.signIn)

	model := fixture.start(t, &fakeProvider{sessions: []Session{session}})

	if header := lineWith(model, "Review requested"); strings.Contains(header, columnHeader) {
		t.Fatalf("header = %q, expected no column of its own", header)
	}
	if line := lineWith(model, fixture.signIn.Title); !strings.Contains(line, sessionMark) {
		t.Fatalf("line = %q, expected the mark", line)
	}
	if line := lineWith(model, fixture.tidy.Title); strings.Contains(line, sessionMark) {
		t.Fatalf("line = %q, expected no mark for the other pull request", line)
	}
}

func TestSessionColumnMarksThePullRequestOnWhoseBranchASessionWorksInTheSameRepository(t *testing.T) {
	fixture := newBoardFixture()
	session := onBranch("storefront-2", fixture.source.Name, fixture.signIn.HeadBranch)

	model := fixture.start(t, &fakeProvider{sessions: []Session{session}})

	if line := lineWith(model, fixture.signIn.Title); !strings.Contains(line, sessionMark) {
		t.Fatalf("line = %q, expected the mark", line)
	}
}

func TestSessionColumnIgnoresASessionOnTheSameBranchInAnotherRepository(t *testing.T) {
	fixture := newBoardFixture()
	session := onBranch("warehouse-1", "warehouse", fixture.signIn.HeadBranch)

	model := fixture.start(t, &fakeProvider{sessions: []Session{session}})

	if line := lineWith(model, fixture.signIn.Title); line == "" || strings.Contains(line, sessionMark) {
		t.Fatalf("line = %q, expected the pull request without the session", line)
	}
}

func TestSessionColumnIgnoresASessionWithoutABranchWhoseRepositoryIsUnknown(t *testing.T) {
	fixture := newBoardFixture()
	session := Session{ID: "id-scratch", Name: "scratch-1"}
	fixture.signIn.HeadBranch = ""

	model := fixture.start(t, &fakeProvider{sessions: []Session{session}})

	if line := lineWith(model, fixture.signIn.Title); strings.Contains(line, sessionMark) {
		t.Fatalf("line = %q", line)
	}
}

func TestSessionColumnShowsTheMarkOnceHoweverManySessionsMatch(t *testing.T) {
	fixture := newBoardFixture()
	older := fixture.mentioning("storefront-1", fixture.signIn)
	newer := fixture.mentioning("storefront-2", fixture.signIn)
	newer.LastActive = older.LastActive.Add(time.Hour)

	model := fixture.start(t, &fakeProvider{sessions: []Session{older, newer}})

	line := lineWith(model, fixture.signIn.Title)
	if strings.Count(line, sessionMark) != 1 || strings.Contains(line, older.Name) || strings.Contains(line, newer.Name) {
		t.Fatalf("line = %q, expected one mark and no session names", line)
	}
}
