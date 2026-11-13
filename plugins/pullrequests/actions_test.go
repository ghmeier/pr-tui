package pullrequests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

func TestOpenRunsOpenWithTheSelectedUrl(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)

	press(t, loadedBoardWith(t, provider), "down", "o")

	assertCommands(t, commands, []string{"open", provider.pullRequest(3).URL})
}

func TestDraftKeyMarksDraftReadyWithoutAsking(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "d")

	assertCalls(t, provider, draftCall(provider.pullRequest(1), false))
}

func TestDraftKeyConvertsReadyBackToDraftWithoutAsking(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "down", "d")

	assertCalls(t, provider, draftCall(provider.pullRequest(3), true))
}

func TestDraftKeyNamesThePullRequestInItsNoticeWhileItWorks(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	model.Update(keyMessage("d"))

	if notice := view(model); !strings.Contains(notice, provider.pullRequest(1).toggleDraftNotice()) {
		t.Fatalf("view:\n%s", notice)
	}
}

func TestDraftKeyReloadsTheBoardWhenTheProviderAccepts(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	provider.pullRequests[0].IsDraft = false
	provider.pullRequests[0].Title = "Now ready"

	press(t, model, "d")

	if !strings.Contains(view(model), "Now ready") {
		t.Fatalf("the board did not reload:\n%s", view(model))
	}
}

func TestDraftKeyShowsTheFailureWhenTheProviderRefuses(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SetDraft", errorForTests)

	model := loadedBoardWith(t, provider)
	press(t, model, "d")

	if !strings.Contains(view(model), "Failed: "+errorForTests.Error()) {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestReviewKeyOnADraftMarksItReadyThenRequestsTheTeam(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "e")

	draft := provider.pullRequest(1)
	assertCalls(t, provider, draftCall(draft, false), teamReviewCall(draft))
}

func TestReviewKeyOnAReadyPullRequestOnlyRequestsTheTeam(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "down", "e")

	assertCalls(t, provider, teamReviewCall(provider.pullRequest(3)))
}

func TestReviewKeyDoesNotRequestTheTeamWhenMarkingReadyFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SetDraft", errorForTests)
	model := loadedBoardWith(t, provider)

	press(t, model, "e")

	assertCalls(t, provider, draftCall(provider.pullRequest(1), false))
	if !strings.Contains(view(model), "Failed: "+errorForTests.Error()) {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestReviewKeyIsNotRegisteredWithoutAReviewTeam(t *testing.T) {
	provider := newFakeProvider()
	provider.reviewTeam = ""
	model := loadedBoardWith(t, provider)

	press(t, model, "e")

	assertCalls(t, provider)
	if help := helpLine(model); strings.Contains(help, "request") {
		t.Fatalf("help = %q", help)
	}
}

func TestHelpOffersTheReviewKeyWithTheTeamNameWhenOneIsConfigured(t *testing.T) {
	model := loadedBoard(t)

	help := helpLine(model)

	if !strings.Contains(help, "e request "+testReviewTeam) {
		t.Fatalf("help = %q", help)
	}
}

func TestHelpListsTheActionsInTheOrderOfTheBoardAndNamesTheConfiguredEditor(t *testing.T) {
	model := loadedBoard(t)
	plugintest.Resize(t, model, 200, 30)

	help := helpLine(model)

	expected := "enter diff · p description · o open · v " + testEditorName + " · d draft / ready · e request " + testReviewTeam +
		" · m auto-merge · f filters · r refresh · q quit"
	if help != expected {
		t.Fatalf("help = %q, expected %q", help, expected)
	}
}

func TestAutoMergeKeyAsksBeforeEnablingAutoMerge(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(1)

	press(t, model, "m")
	asked := view(model)
	callsWhileAsking := provider.recorded()
	press(t, model, "y")

	if !strings.Contains(asked, "Enable auto-merge (squash) on "+selected.label()+"?") || len(callsWhileAsking) != 0 {
		t.Fatalf("view while asking:\n%s\ncalls = %v", asked, callsWhileAsking)
	}
	assertCalls(t, provider, autoMergeCall(selected, true))
}

func TestAutoMergeKeyDisablesAutoMergeWhenAlreadyEnabled(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(3)

	press(t, model, "down", "m")
	asked := view(model)
	press(t, model, "y")

	if !strings.Contains(asked, "Disable auto-merge on "+selected.label()+"?") {
		t.Fatalf("view while asking:\n%s", asked)
	}
	assertCalls(t, provider, autoMergeCall(selected, false))
}

func TestDecliningAutoMergeChangesNothing(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "m", "n")

	assertCalls(t, provider)
	if strings.Contains(view(model), "auto-merge (squash)") {
		t.Fatalf("the question is still shown:\n%s", view(model))
	}
}

func TestEditorKeyOpensTheWorktreeAndTheProvidersChangedFiles(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)
	model := loadedBoardWith(t, provider)

	press(t, model, "down", "v")

	assertCommands(t, commands, append([]string{
		"wt", "-C", filepath.Join(testCloneRoot, provider.pullRequest(3).Repository.Name), "switch", "pr:3",
		"--no-cd", "-x", testEditorCommand[0], "--", testEditorCommand[1], ".",
	}, provider.changedFiles...))
	if strings.Contains(view(model), "Opening") {
		t.Fatalf("the notice should be gone once the editor opened:\n%s", view(model))
	}
}

func TestEditorKeyNamesTheEditorInItsNoticeWhileItWorks(t *testing.T) {
	provider := newFakeProvider()
	recordShell(t)
	model := loadedBoardWith(t, provider)

	model.Update(keyMessage("v"))

	expected := "Opening " + provider.pullRequest(1).label() + " in " + testEditorName + "…"
	if !strings.Contains(view(model), expected) {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestEditorKeyStillOpensTheWorktreeWhenTheChangedFilesCannotBeListed(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("ChangedFiles", errorForTests)
	commands := recordShell(t)

	press(t, loadedBoardWith(t, provider), "down", "v")

	assertCommands(t, commands, []string{
		"wt", "-C", filepath.Join(testCloneRoot, provider.pullRequest(3).Repository.Name), "switch", "pr:3",
		"--no-cd", "-x", testEditorCommand[0], "--", testEditorCommand[1], ".",
	})
}

func TestEditorKeyPassesAStringCommandAsTheOnlyProgramWordEvenWithSpaces(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)
	program := "/Applications/Visual Studio Code - Insiders.app/Contents/Resources/app/bin/code"

	press(t, newSetup(provider).withEditorCommand(program).started(t), "down", "v")

	assertCommands(t, commands, append([]string{
		"wt", "-C", filepath.Join(testCloneRoot, provider.pullRequest(3).Repository.Name), "switch", "pr:3",
		"--no-cd", "-x", program, "--", ".",
	}, provider.changedFiles...))
}

func TestEditorKeyShowsTheFailureWhenTheWorktreeCannotBeOpened(t *testing.T) {
	failShell(t, errorForTests)
	model := loadedBoard(t)

	press(t, model, "v")

	if !strings.Contains(view(model), "Failed: "+errorForTests.Error()) {
		t.Fatalf("view:\n%s", view(model))
	}
}
