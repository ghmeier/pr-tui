package pullrequests

import (
	"os/exec"
	"path/filepath"
	"slices"
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

	press(t, loadedBoardWith(t, provider), "t")

	draft := provider.pullRequest(1)
	assertCalls(t, provider, draftCall(draft, false), teamReviewCall(draft))
}

func TestReviewKeyOnAReadyPullRequestOnlyRequestsTheTeam(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "down", "t")

	assertCalls(t, provider, teamReviewCall(provider.pullRequest(3)))
}

func TestReviewKeyDoesNotRequestTheTeamWhenMarkingReadyFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SetDraft", errorForTests)
	model := loadedBoardWith(t, provider)

	press(t, model, "t")

	assertCalls(t, provider, draftCall(provider.pullRequest(1), false))
	if !strings.Contains(view(model), "Failed: "+errorForTests.Error()) {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestReviewKeyIsNotRegisteredWithoutAReviewTeam(t *testing.T) {
	provider := newFakeProvider()
	provider.reviewTeam = ""
	model := loadedBoardWith(t, provider)

	press(t, model, "t")

	assertCalls(t, provider)
	if help := helpLine(model); strings.Contains(help, "request") {
		t.Fatalf("help = %q", help)
	}
}

func TestHelpOffersTheReviewKeyWithTheTeamNameWhenOneIsConfigured(t *testing.T) {
	model := loadedBoard(t)

	help := helpLine(model)

	if !strings.Contains(help, "t request "+testReviewTeam) {
		t.Fatalf("help = %q", help)
	}
}

func TestHelpListsTheActionsInTheOrderOfTheBoardAndNamesTheConfiguredEditor(t *testing.T) {
	model := loadedBoard(t)
	plugintest.Resize(t, model, 200, 30)

	help := helpLine(model)

	expected := "enter diff · p description · o open · v " + testEditorName + " · d draft / ready · t request " + testReviewTeam +
		" · m merge · a approve · f filters · r refresh · q quit"
	if help != expected {
		t.Fatalf("help = %q, expected %q", help, expected)
	}
}

func TestMergeKeyOffersMergingNowOrEnablingAutoMergeWhenAutoMergeIsOff(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "m")

	asked := view(model)
	for _, expected := range []string{"Merge " + provider.pullRequest(1).label(), mergeNowOption, enableAutoMergeOption} {
		if !strings.Contains(asked, expected) {
			t.Fatalf("view should contain %q:\n%s", expected, asked)
		}
	}
	if strings.Contains(asked, disableAutoMergeOption) {
		t.Fatalf("view offers disabling auto-merge that is off:\n%s", asked)
	}
	assertCalls(t, provider)
}

func TestMergeKeyOffersDisablingAutoMergeWhenAutoMergeIsOn(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "down", "m")

	asked := view(model)
	if !strings.Contains(asked, mergeNowOption) || !strings.Contains(asked, disableAutoMergeOption) {
		t.Fatalf("view:\n%s", asked)
	}
	if strings.Contains(asked, enableAutoMergeOption) {
		t.Fatalf("view offers enabling auto-merge that is on:\n%s", asked)
	}
}

func TestChoosingToEnableAutoMergeEnablesItWithoutAsking(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(1)

	press(t, model, "m", "2")

	assertCalls(t, provider, autoMergeCall(selected, true))
}

func TestChoosingToDisableAutoMergeDisablesItWithoutAsking(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(3)

	press(t, model, "down", "m", "2")

	assertCalls(t, provider, autoMergeCall(selected, false))
}

func TestChoosingToMergeNowAsksBeforeMerging(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(1)

	press(t, model, "m", "1")
	asked := view(model)
	callsWhileAsking := provider.recorded()
	press(t, model, "y")

	if !strings.Contains(asked, "Merge "+selected.label()+" now (squash)?") || len(callsWhileAsking) != 0 {
		t.Fatalf("view while asking:\n%s\ncalls = %v", asked, callsWhileAsking)
	}
	assertCalls(t, provider, mergeCall(selected))
}

func TestDecliningTheMergeConfirmationMergesNothing(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "m", "1", "n")

	assertCalls(t, provider)
	if strings.Contains(view(model), "now (squash)?") {
		t.Fatalf("the question is still shown:\n%s", view(model))
	}
}

func TestApproveKeyAsksBeforeApprovingSomeoneElsesPullRequest(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	selected := provider.pullRequest(1)

	press(t, model, "a")
	asked := view(model)
	callsWhileAsking := provider.recorded()
	press(t, model, "y")

	if !strings.Contains(asked, selected.approveQuestion()) || len(callsWhileAsking) != 0 {
		t.Fatalf("view while asking:\n%s\ncalls = %v", asked, callsWhileAsking)
	}
	assertCalls(t, provider, approveCall(selected))
}

func TestDecliningApproveChangesNothing(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "a", "n")

	assertCalls(t, provider)
}

func TestApproveKeyNamesThePullRequestInItsNoticeWhileItWorks(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "a")
	model.Update(keyMessage("y"))

	if notice := view(model); !strings.Contains(notice, provider.pullRequest(1).approveNotice()) {
		t.Fatalf("view:\n%s", notice)
	}
}

func TestApproveKeyOnTheViewersOwnPullRequestOnlyShowsANotice(t *testing.T) {
	provider := newFakeProvider()
	provider.pullRequests[0].Author = testViewer
	model := loadedBoardWith(t, provider)

	press(t, model, "a")

	assertCalls(t, provider)
	if notice := view(model); !strings.Contains(notice, approveOwnPullRequestNotice) || strings.Contains(notice, "Approve ") {
		t.Fatalf("view:\n%s", notice)
	}
}

func TestReviewKeyIsNoLongerBoundToE(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "e")

	assertCalls(t, provider)
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
	program := "/Applications/Some Editor.app/Contents/bin/edit"

	press(t, newSetup(provider).withEditorCommand(program).started(t), "down", "v")

	assertCommands(t, commands, append([]string{
		"wt", "-C", filepath.Join(testCloneRoot, provider.pullRequest(3).Repository.Name), "switch", "pr:3",
		"--no-cd", "-x", program, "--", ".",
	}, provider.changedFiles...))
}

func TestEditorKeyExplainsWhenWorktrunkIsNotInstalled(t *testing.T) {
	failShell(t, &exec.Error{Name: "wt", Err: exec.ErrNotFound})
	model := loadedBoard(t)

	press(t, model, "v")

	if !strings.Contains(view(model), "Failed: "+errWorktrunkMissing.Error()) {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestEditorKeyShowsTheFailureWhenTheWorktreeCannotBeOpened(t *testing.T) {
	failShell(t, errorForTests)
	model := loadedBoard(t)

	press(t, model, "v")

	if !strings.Contains(view(model), "Failed: "+errorForTests.Error()) {
		t.Fatalf("view:\n%s", view(model))
	}
}

// rowRefreshingActions are the keys of each action that changes one pull request, which the
// board answers by reloading that pull request alone.
var rowRefreshingActions = map[string][]string{
	"draft":             {"d"},
	"request review":    {"t"},
	"approve":           {"a", "y"},
	"merge now":         {"m", "1", "y"},
	"enable auto-merge": {"m", "2"},
}

func TestActionsReloadOnlyTheChangedPullRequest(t *testing.T) {
	for name, keys := range rowRefreshingActions {
		t.Run(name, func(t *testing.T) {
			provider := newFakeProvider()
			model := loadedBoardWith(t, provider)
			selected := provider.pullRequest(1)
			listingsBefore, _ := provider.loads()

			press(t, model, keys...)

			listings, reloaded := provider.loads()
			if listings != listingsBefore {
				t.Fatalf("every pull request was loaded %d more times", listings-listingsBefore)
			}
			if expected := []string{selected.URL}; !slices.Equal(reloaded, expected) {
				t.Fatalf("reloaded %v, expected %v", reloaded, expected)
			}
		})
	}
}

func TestActionThatFailsReloadsNothing(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("Approve", errorForTests)
	model := loadedBoardWith(t, provider)
	listingsBefore, _ := provider.loads()

	press(t, model, "a", "y")

	listings, reloaded := provider.loads()
	if listings != listingsBefore || len(reloaded) != 0 {
		t.Fatalf("loaded every pull request %d more times and reloaded %v", listings-listingsBefore, reloaded)
	}
}

func TestMergedPullRequestDisappearsFromTheBoard(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	merged, others := provider.pullRequest(1), []PullRequest{provider.pullRequest(3)}

	press(t, model, "m", "1", "y")

	if strings.Contains(view(model), merged.Title) {
		t.Fatalf("the merged pull request is still listed:\n%s", view(model))
	}
	for _, other := range others {
		if !strings.Contains(view(model), other.Title) {
			t.Fatalf("%q is no longer listed:\n%s", other.Title, view(model))
		}
	}
}

func TestPullRequestThatStaysOpenAfterAnActionStaysOnTheBoard(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "m", "2")

	assertShown(t, model, provider, provider.pullRequest(1), provider.pullRequest(3))
}

func TestReloadedPullRequestShowsItsNewState(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	provider.pullRequests[0].Title = "Retitled"

	press(t, model, "d")

	if !strings.Contains(view(model), provider.pullRequests[0].Title) {
		t.Fatalf("the row was not reloaded:\n%s", view(model))
	}
}

func TestReloadFailureShowsTheErrorAndKeepsTheRow(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("PullRequest", errorForTests)
	model := loadedBoardWith(t, provider)

	press(t, model, "d")

	if !strings.Contains(view(model), "Refresh failed: "+errorForTests.Error()) {
		t.Fatalf("view:\n%s", view(model))
	}
	assertShown(t, model, provider, provider.pullRequest(1), provider.pullRequest(3))
}
