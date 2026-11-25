package claude

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/plugins/agentsessions"
)

func sessionsOf(t *testing.T) []agentsessions.Session {
	t.Helper()
	sessions, err := provider{relayModel: "haiku"}.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

func TestSessionsMapsTheStatusOfEachListedSessionIncludingABackgroundState(t *testing.T) {
	fake := installFakeClaude(t)
	background := map[string]any{"sessionId": "id-blocked", "name": "background", "cwd": "/work", "kind": "background", "startedAt": startedAt.UnixMilli(), "state": "blocked"}
	fake.agents = []map[string]any{listed("id-idle", "idle one", "/work", "idle"), listed("id-busy", "busy one", "/work", "busy"), background, listed("id-odd", "odd one", "/work", "starting")}

	sessions := sessionsOf(t)

	expected := []agentsessions.Status{agentsessions.StatusIdle, agentsessions.StatusBusy, agentsessions.StatusBlocked, agentsessions.StatusUnknown}
	for index, status := range expected {
		if sessions[index].Status != status || sessions[index].ID != fake.agents[index]["sessionId"] || sessions[index].Name != fake.agents[index]["name"] {
			t.Fatalf("session %d = %+v, expected status %q", index, sessions[index], status)
		}
	}
}

func TestSessionsTakeDirectoryBranchAndActivityFromTheNewestTranscriptEntries(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", "storefront-1", "/launch", "idle")}
	older, newer := startedAt.Add(time.Minute), startedAt.Add(time.Hour)
	fake.writeTranscript(t, "id-1", entry("/launch", "main", older, "first"), entry("/worktree", "feature/sign-in", newer, "second"))
	fake.commonDirectories["/worktree"] = "/checkouts/storefront/.git"

	sessions := sessionsOf(t)

	session := sessions[0]
	if session.Branch != "feature/sign-in" || !session.LastActive.Equal(newer) || session.Repository != "storefront" {
		t.Fatalf("session = %+v", session)
	}
	if len(fake.gitDirectories) != 1 || fake.gitDirectories[0] != "/worktree" {
		t.Fatalf("git ran in %v, expected the newest directory", fake.gitDirectories)
	}
}

func TestSessionsKeepASessionWithoutATranscriptWithNoReferences(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", "storefront-1", "/launch", "idle")}
	fake.commonDirectories["/launch"] = "/checkouts/storefront/.git"

	sessions := sessionsOf(t)

	session := sessions[0]
	if len(sessions) != 1 || len(session.PullRequests) != 0 || len(session.Documents) != 0 || session.Branch != "" {
		t.Fatalf("session = %+v", session)
	}
	if !session.LastActive.Equal(startedAt) || session.Repository != "storefront" {
		t.Fatalf("session = %+v, expected the start time and the repository of the launch directory", session)
	}
}

func TestSessionsCollectPullRequestsAndDocumentsNewestFirstWithoutDuplicates(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", "storefront-1", "/launch", "idle")}
	fake.writeTranscript(t, "id-1",
		entry("/launch", "main", startedAt, "see https://github.com/acme/storefront/pull/12 and .apl/plan/2026-11-01-sign-in.md"),
		entry("/launch", "main", startedAt, "also https://github.com/acme/billing/pull/7 and .apl/research/passwords.md"),
		entry("/launch", "main", startedAt, "again https://github.com/acme/storefront/pull/12/files and .apl/plan/2026-11-01-sign-in.md"),
	)

	sessions := sessionsOf(t)

	expectedPullRequests := []agentsessions.PullRequestReference{{Repository: "acme/storefront", Number: 12}, {Repository: "acme/billing", Number: 7}}
	if len(sessions[0].PullRequests) != 2 || sessions[0].PullRequests[0] != expectedPullRequests[0] || sessions[0].PullRequests[1] != expectedPullRequests[1] {
		t.Fatalf("pull requests = %v, expected %v", sessions[0].PullRequests, expectedPullRequests)
	}
	expectedDocuments := []string{"plan/2026-11-01-sign-in.md", "research/passwords.md"}
	if len(sessions[0].Documents) != 2 || sessions[0].Documents[0] != expectedDocuments[0] || sessions[0].Documents[1] != expectedDocuments[1] {
		t.Fatalf("documents = %v, expected %v", sessions[0].Documents, expectedDocuments)
	}
}

func TestSessionsIgnoreABarePullRequestNumber(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", "storefront-1", "/launch", "idle")}
	fake.writeTranscript(t, "id-1", entry("/launch", "main", startedAt, "fixed #123 and acme/storefront#45"))

	sessions := sessionsOf(t)

	if len(sessions[0].PullRequests) != 0 {
		t.Fatalf("pull requests = %v", sessions[0].PullRequests)
	}
}

func TestSessionsReadOnlyTheLastTwoMebibytesOfATranscript(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", "storefront-1", "/launch", "idle")}
	padding := entry("/launch", "main", startedAt, strings.Repeat("x", 1000))
	lines := []string{entry("/launch", "main", startedAt, "early https://github.com/acme/storefront/pull/1")}
	for size := 0; size < transcriptTail; size += len(padding) + 1 {
		lines = append(lines, padding)
	}
	lines = append(lines, entry("/launch", "main", startedAt, "late https://github.com/acme/storefront/pull/2"))
	fake.writeTranscript(t, "id-1", lines...)

	sessions := sessionsOf(t)

	if len(sessions[0].PullRequests) != 1 || sessions[0].PullRequests[0].Number != 2 {
		t.Fatalf("pull requests = %v, expected only the late mention", sessions[0].PullRequests)
	}
}

func TestSessionsNameTheRepositoryAfterTheParentOfTheGitCommonDirectory(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", "storefront-1", "/checkouts/storefront.feature", "idle")}
	fake.commonDirectories["/checkouts/storefront.feature"] = "/checkouts/storefront/.git"

	sessions := sessionsOf(t)

	if sessions[0].Repository != "storefront" {
		t.Fatalf("repository = %q", sessions[0].Repository)
	}
}

func TestSessionsLeaveTheRepositoryEmptyWhenGitFails(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", "scratch-1", "/not/a/repository", "idle")}

	sessions := sessionsOf(t)

	if len(sessions) != 1 || sessions[0].Repository != "" {
		t.Fatalf("sessions = %+v", sessions)
	}
}

var recipient = agentsessions.Session{ID: "id-1", Name: "storefront-1"}

func TestSendRunsTheRelayWithTheExactArgumentsAndPromptOnStandardInput(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", recipient.Name, "/launch", "idle")}

	err := provider{relayModel: "haiku"}.Send(recipient, "Please rename the helper.\nThanks.")

	expectedArguments := []string{
		"claude", "-p", "--safe-mode", "--model", "haiku", "--effort", "low", "--no-session-persistence", "--name", "prs",
		"--strict-mcp-config", "--tools=SendMessage", "--allowed-tools=SendMessage", "--output-format", "json", "--system-prompt", relayInstructions,
	}
	if err != nil || len(fake.relayRuns) != 1 || strings.Join(fake.relayRuns[0], "\x00") != strings.Join(expectedArguments, "\x00") {
		t.Fatalf("error = %v, relay runs = %q", err, fake.relayRuns)
	}
	if expected := "Recipient: storefront-1\nMessage:\nPlease rename the helper.\nThanks."; fake.relayInputs[0] != expected {
		t.Fatalf("standard input = %q, expected %q", fake.relayInputs[0], expected)
	}
}

func TestSendUsesTheConfiguredRelayModel(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", recipient.Name, "/launch", "idle")}

	err := provider{relayModel: "sonnet"}.Send(recipient, "hello")

	if err != nil || fake.relayRuns[0][4] != "sonnet" {
		t.Fatalf("error = %v, arguments = %q", err, fake.relayRuns[0])
	}
}

func TestSendFailsWithTheResultTextWhenTheRelayDoesNotReportSent(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", recipient.Name, "/launch", "idle")}
	fake.relayOutput = `{"is_error":false,"result":"not sent: no agent named storefront-1"}`

	err := provider{relayModel: "haiku"}.Send(recipient, "hello")

	if err == nil || err.Error() != "not sent: no agent named storefront-1" {
		t.Fatalf("error = %v", err)
	}
}

func TestSendAcceptsASentResultIgnoringCaseAndTrailingPunctuation(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", recipient.Name, "/launch", "idle")}
	fake.relayOutput = `{"is_error":false,"result":" Sent.\n"}`

	err := provider{relayModel: "haiku"}.Send(recipient, "hello")

	if err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestSendFailsWhenTheRelayReportsAnError(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", recipient.Name, "/launch", "idle")}
	fake.relayOutput = `{"is_error":true,"result":"sent"}`

	err := provider{relayModel: "haiku"}.Send(recipient, "hello")

	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestSendFailsWithAPlainMessageWhenTheRelayPrintsSomethingThatIsNotJSON(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", recipient.Name, "/launch", "idle")}
	fake.relayOutput = "Warning: something\nsent"

	err := provider{relayModel: "haiku"}.Send(recipient, "hello")

	if err == nil || !strings.Contains(err.Error(), "Warning: something") || strings.Contains(err.Error(), "\n") {
		t.Fatalf("error = %v", err)
	}
}

func TestSendFailsWhenTheRelayCannotRun(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", recipient.Name, "/launch", "idle")}
	fake.relayError = errors.New("not logged in")

	err := provider{relayModel: "haiku"}.Send(recipient, "hello")

	if !errors.Is(err, fake.relayError) {
		t.Fatalf("error = %v", err)
	}
}

func TestSendRefusesWithoutRunningTheRelayWhenTwoSessionsShareTheName(t *testing.T) {
	fake := installFakeClaude(t)
	fake.agents = []map[string]any{listed("id-1", recipient.Name, "/launch", "idle"), listed("id-2", recipient.Name, "/other", "busy")}

	err := provider{relayModel: "haiku"}.Send(recipient, "hello")

	if err == nil || err.Error() != "Two Claude sessions are named storefront-1; rename one with /rename" {
		t.Fatalf("error = %v", err)
	}
	if len(fake.relayRuns) != 0 {
		t.Fatalf("the relay ran: %q", fake.relayRuns)
	}
}
