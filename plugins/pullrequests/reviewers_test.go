package pullrequests

import (
	"slices"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

// reviewerMenu is the options of the open review menu, in order, without their cursor marker.
func reviewerMenu(model *plugin.Model) []string {
	var options []string
	inMenu := false
	for _, line := range strings.Split(view(model), "\n") {
		switch {
		case strings.Contains(line, "Request review on"):
			inMenu = true
		case strings.Contains(line, "enter pick"):
			return options
		case inMenu:
			text := strings.Trim(line, " │▌")
			if text != "" {
				options = append(options, text)
			}
		}
	}
	return options
}

func statusLineOf(model *plugin.Model) string {
	lines := strings.Split(view(model), "\n")
	return lines[len(lines)-2]
}

func teamOptionOf(team string) string { return team + " (team)" }

func TestReviewMenuListsTheTeamThenItsMembersSortedWithoutTheAuthorOrThoseAlreadyRequested(t *testing.T) {
	provider := newFakeProvider()
	provider.pullRequests[0].RequestedUsers = []string{"bob"}
	model := loadedBoardWith(t, provider)

	press(t, model, "t")

	expected := []string{teamOptionOf(testReviewTeam), "alice", "someone-else", testViewer}
	if options := reviewerMenu(model); !slices.Equal(options, expected) {
		t.Fatalf("options = %v, expected %v", options, expected)
	}
}

func TestReviewMenuAsksAboutThePullRequestItIsFor(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "t")

	if asked := "Request review on " + provider.pullRequest(1).label() + " from"; !strings.Contains(view(model), asked) {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestReviewMenuLeavesOutTheTeamOnceItIsRequested(t *testing.T) {
	provider := newFakeProvider()
	provider.pullRequests[0].RequestedTeams = []string{testReviewTeam}
	model := loadedBoardWith(t, provider)

	press(t, model, "t")

	if options := reviewerMenu(model); slices.Contains(options, teamOptionOf(testReviewTeam)) || len(options) == 0 {
		t.Fatalf("options = %v", options)
	}
}

func TestReviewMenuKeepsTheTeamWhileItIsNotRequested(t *testing.T) {
	model := loadedBoard(t)

	press(t, model, "t")

	if options := reviewerMenu(model); len(options) == 0 || options[0] != teamOptionOf(testReviewTeam) {
		t.Fatalf("options = %v", options)
	}
}

func TestReviewKeyIsHiddenAndDoesNothingWhenTheTeamAndEveryMemberAreRequested(t *testing.T) {
	provider := newFakeProvider()
	provider.pullRequests[2].RequestedUsers = []string{"alice", "bob", testViewer, "someone-else"}
	model := loadedBoardWith(t, provider)
	press(t, model, "down", "down")

	press(t, model, "t")

	if help := helpLine(model); strings.Contains(help, "request") {
		t.Fatalf("help = %q", help)
	}
	if options := reviewerMenu(model); len(options) != 0 {
		t.Fatalf("options = %v", options)
	}
	assertCalls(t, provider)
}

func TestPickingAPersonRequestsReviewFromThem(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "down", "t", "b", "o", "enter")

	assertCalls(t, provider, personReviewCall(provider.pullRequest(3), "bob"))
}

func TestPickingTheTeamRequestsReviewFromTheTeam(t *testing.T) {
	provider := newFakeProvider()
	provider.pullRequests[2].RequestedTeams = nil
	model := loadedBoardWith(t, provider)

	press(t, model, "down", "t", "enter")

	assertCalls(t, provider, teamReviewCall(provider.pullRequest(3)))
}

func TestPickingAPersonOnADraftMarksItReadyFirst(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "t", "down", "enter")

	draft := provider.pullRequest(1)
	assertCalls(t, provider, draftCall(draft, false), personReviewCall(draft, "alice"))
}

func TestPickingNamesTheRequestedReviewerInTheNotice(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "t", "down")
	model.Update(keyMessage("enter"))

	if notice := statusLineOf(model); !strings.Contains(notice, "alice") {
		t.Fatalf("status = %q", notice)
	}
}

func TestMembersAreListedOncePerOwnerNotOncePerRepository(t *testing.T) {
	provider := newFakeProvider()

	loadedBoardWith(t, provider)

	if requests := provider.memberRequestsFor(storefrontRepository.Owner); requests != 1 {
		t.Fatalf("members were requested %d times for %s", requests, storefrontRepository.Owner)
	}
}

func TestMembersAreNotListedWithoutAReviewTeam(t *testing.T) {
	provider := newFakeProvider()
	provider.reviewTeam = ""

	loadedBoardWith(t, provider)

	if requests := provider.memberRequestsFor(storefrontRepository.Owner); requests != 0 {
		t.Fatalf("members were requested %d times", requests)
	}
}

func TestFailingToListMembersStillLoadsThePullRequestsAndOffersTheTeam(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("TeamMembers", errorForTests)
	model := loadedBoardWith(t, provider)

	press(t, model, "t")

	if !strings.Contains(view(model), provider.pullRequest(1).Title) {
		t.Fatalf("the pull requests did not load:\n%s", view(model))
	}
	if options := reviewerMenu(model); !slices.Equal(options, []string{teamOptionOf(testReviewTeam)}) {
		t.Fatalf("options = %v", options)
	}
}

func TestReloadedRowKeepsTheTeamMembersOfThePreviousRow(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "d", "t")

	if options := reviewerMenu(model); !slices.Contains(options, "alice") {
		t.Fatalf("options = %v", options)
	}
}
