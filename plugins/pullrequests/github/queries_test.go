package github

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

const cannedPullRequests = `{"data": {
  "viewer": {"login": "viewer"},
  "repository0": {"pullRequests": {"nodes": [
    {"number": 1, "title": "Draft from a person", "url": "https://example.test/gazebo/1", "isDraft": true, "headRefOid": "first-commit", "headRefName": "feature/first",
     "updatedAt": "2026-11-10T10:00:00Z", "reviewDecision": null,
     "author": {"login": "person", "__typename": "User"},
     "reviewRequests": {"nodes": []}},
    {"number": 2, "title": "Dependency update", "url": "https://example.test/gazebo/2", "isDraft": false, "headRefName": "renovate/dependency",
     "updatedAt": "2026-11-10T11:00:00Z", "reviewDecision": "APPROVED",
     "author": {"login": "renovate", "__typename": "Bot"},
     "reviewRequests": {"nodes": []}}
  ]}},
  "repository1": {"pullRequests": {"nodes": [
    {"number": 3, "title": "Ready with reviewers", "url": "https://example.test/u2/3", "isDraft": false, "isInMergeQueue": true,
     "updatedAt": "2026-11-10T12:00:00Z", "reviewDecision": "CHANGES_REQUESTED", "autoMergeRequest": {"enabledAt": "2026-11-10T09:00:00Z"},
     "author": {"login": "person", "__typename": "User"},
     "reviewRequests": {"nodes": [
       {"requestedReviewer": {"__typename": "User", "login": "viewer"}},
       {"requestedReviewer": {"__typename": "Team", "slug": "engineering"}},
       {"requestedReviewer": null},
       {"requestedReviewer": {"__typename": "User", "login": "someone-else"}}
     ]}},
    {"number": 4, "title": "From a deleted account", "url": "https://example.test/u2/4", "isDraft": false,
     "updatedAt": "2026-11-10T09:00:00Z", "reviewDecision": "REVIEW_REQUIRED", "author": null,
     "reviewRequests": {"nodes": []}}
  ]}}
}}`

func loadCannedPullRequests(t *testing.T) (pullRequests []pullrequests.PullRequest, viewer string, stub *stubbedGh) {
	t.Helper()
	stub = stubShell(t, cannedPullRequests)
	pullRequests, viewer, err := providerFor(gazebo, partners).PullRequests()
	if err != nil {
		t.Fatal(err)
	}
	return pullRequests, viewer, stub
}

func pullRequestNumbered(t *testing.T, pullRequests []pullrequests.PullRequest, number int) pullrequests.PullRequest {
	t.Helper()
	for _, pr := range pullRequests {
		if pr.Number == number {
			return pr
		}
	}
	t.Fatalf("no pull request %d in %v", number, pullRequests)
	return pullrequests.PullRequest{}
}

func TestPullRequestsAsksForEveryRepositoryInOneGraphQLQuery(t *testing.T) {
	_, _, stub := loadCannedPullRequests(t)

	if len(stub.commands) != 1 {
		t.Fatalf("commands = %v, expected one", stub.commands)
	}
	command := stub.commands[0]
	if !reflect.DeepEqual(command[:4], []string{"gh", "api", "graphql", "-f"}) || !strings.HasPrefix(command[4], "query=query { viewer { login }") {
		t.Fatalf("command = %v", command)
	}
}

func TestPullRequestsQueryNamesEachRepositoryWithItsOwnOwner(t *testing.T) {
	_, _, stub := loadCannedPullRequests(t)

	query := stub.commands[0][4]

	for index, source := range []pullrequests.Repository{gazebo, partners} {
		expected := fmt.Sprintf("repository%d: repository(owner: %q, name: %q)", index, source.Owner, source.Name)
		if !strings.Contains(query, expected) {
			t.Fatalf("query is missing %s:\n%s", expected, query)
		}
	}
	wrongOwner := fmt.Sprintf("owner: %q, name: %q", gazebo.Owner, partners.Name)
	if strings.Contains(query, wrongOwner) {
		t.Fatalf("query asks for %s under the wrong owner:\n%s", partners.Name, query)
	}
}

func TestPullRequestsReturnsTheViewerLogin(t *testing.T) {
	_, viewer, _ := loadCannedPullRequests(t)

	if viewer != "viewer" {
		t.Fatalf("viewer = %q", viewer)
	}
}

func TestPullRequestsAreSortedNewestFirstAcrossRepositories(t *testing.T) {
	pullRequests, _, _ := loadCannedPullRequests(t)

	var numbers []int
	for _, pr := range pullRequests {
		numbers = append(numbers, pr.Number)
	}

	if expected := []int{3, 2, 1, 4}; !reflect.DeepEqual(numbers, expected) {
		t.Fatalf("numbers = %v, expected %v", numbers, expected)
	}
}

func TestPullRequestsAttributeEachPullRequestToTheRepositoryItWasAskedFor(t *testing.T) {
	pullRequests, _, _ := loadCannedPullRequests(t)

	if got := pullRequestNumbered(t, pullRequests, 1).Repository; got != gazebo {
		t.Fatalf("repository of 1 = %v, expected %v", got, gazebo)
	}
	if got := pullRequestNumbered(t, pullRequests, 3).Repository; got != partners {
		t.Fatalf("repository of 3 = %v, expected %v", got, partners)
	}
}

func TestPullRequestsReadDraftHeadCommitAndAuthor(t *testing.T) {
	pullRequests, _, _ := loadCannedPullRequests(t)

	draft := pullRequestNumbered(t, pullRequests, 1)

	expected := pullrequests.PullRequest{
		Repository: gazebo, Number: 1, Title: "Draft from a person", URL: "https://example.test/gazebo/1",
		Author: "person", IsDraft: true, HeadCommit: "first-commit", HeadBranch: "feature/first", UpdatedAt: time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(draft, expected) {
		t.Fatalf("pull request = %+v, expected %+v", draft, expected)
	}
}

func TestPullRequestsReadTheHeadBranchOfEachPullRequest(t *testing.T) {
	pullRequests, _, _ := loadCannedPullRequests(t)

	first, second := pullRequestNumbered(t, pullRequests, 1), pullRequestNumbered(t, pullRequests, 2)

	if first.HeadBranch != "feature/first" || second.HeadBranch != "renovate/dependency" {
		t.Fatalf("head branches = %q and %q", first.HeadBranch, second.HeadBranch)
	}
}

func TestPullRequestsMarkBotAuthorsAndNotPeople(t *testing.T) {
	pullRequests, _, _ := loadCannedPullRequests(t)

	if !pullRequestNumbered(t, pullRequests, 2).IsBot || pullRequestNumbered(t, pullRequests, 1).IsBot {
		t.Fatalf("only the pull request by renovate should be a bot's")
	}
}

func TestPullRequestsNameAGhostWhenTheAuthorAccountIsGone(t *testing.T) {
	pullRequests, _, _ := loadCannedPullRequests(t)

	if author := pullRequestNumbered(t, pullRequests, 4).Author; author != "ghost" {
		t.Fatalf("author = %q", author)
	}
}

func TestPullRequestsTranslateReviewDecisions(t *testing.T) {
	pullRequests, _, _ := loadCannedPullRequests(t)

	cases := map[int]pullrequests.ReviewDecision{
		1: pullrequests.ReviewDecisionNone,
		2: pullrequests.ReviewDecisionApproved,
		3: pullrequests.ReviewDecisionChangesRequested,
		4: pullrequests.ReviewDecisionNone,
	}
	for number, expected := range cases {
		if got := pullRequestNumbered(t, pullRequests, number).ReviewDecision; got != expected {
			t.Errorf("review decision of %d = %v, expected %v", number, got, expected)
		}
	}
}

func TestPullRequestsReadAutoMergeAndMergeQueue(t *testing.T) {
	pullRequests, _, _ := loadCannedPullRequests(t)

	queued := pullRequestNumbered(t, pullRequests, 3)
	plain := pullRequestNumbered(t, pullRequests, 1)

	if !queued.AutoMerge || !queued.InMergeQueue || plain.AutoMerge || plain.InMergeQueue {
		t.Fatalf("queued = %+v, plain = %+v", queued, plain)
	}
}

func TestPullRequestsSeparateRequestedUsersFromRequestedTeamsAndSkipEmptyRequests(t *testing.T) {
	pullRequests, _, _ := loadCannedPullRequests(t)

	requested := pullRequestNumbered(t, pullRequests, 3)

	if expected := []string{"viewer", "someone-else"}; !reflect.DeepEqual(requested.RequestedUsers, expected) {
		t.Fatalf("requested users = %v, expected %v", requested.RequestedUsers, expected)
	}
	if expected := []string{reviewTeamName}; !reflect.DeepEqual(requested.RequestedTeams, expected) {
		t.Fatalf("requested teams = %v, expected %v", requested.RequestedTeams, expected)
	}
}

func TestPullRequestsRejectsAResponseThatIsNotJSON(t *testing.T) {
	stubShell(t, "not json")

	_, _, err := providerFor(gazebo).PullRequests()

	if err == nil {
		t.Fatal("expected an error")
	}
}

const cannedLineCounts = `{"data": {"repository": {"pullRequests": {"nodes": [
  {"url": "https://example.test/gazebo/1", "additions": 12, "deletions": 3},
  {"url": "https://example.test/gazebo/2", "additions": 0, "deletions": 40}
]}}}}`

func TestLineCountsAsksForTheRepositoryUnderItsOwnOwner(t *testing.T) {
	stub := stubShell(t, cannedLineCounts)

	_, err := providerFor(gazebo, partners).LineCounts(partners)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{
		"gh", "api", "graphql", "-f", "query=" + lineCountsQuery, "-f", "owner=" + partners.Owner, "-f", "name=" + partners.Name,
	})
}

func TestLineCountsAreKeyedByPullRequestURL(t *testing.T) {
	stubShell(t, cannedLineCounts)

	counts, err := providerFor(gazebo).LineCounts(gazebo)

	expected := map[string]pullrequests.LineCount{
		"https://example.test/gazebo/1": {Additions: 12, Deletions: 3},
		"https://example.test/gazebo/2": {Additions: 0, Deletions: 40},
	}
	if err != nil || !reflect.DeepEqual(counts, expected) {
		t.Fatalf("counts = %v, err = %v, expected %v", counts, err, expected)
	}
}

func cannedCIStates(states ...string) string {
	var nodes []string
	for index, state := range states {
		rollup := "null"
		if state != "" {
			rollup = fmt.Sprintf(`{"state": %q}`, state)
		}
		nodes = append(nodes, fmt.Sprintf(
			`{"url": "https://example.test/gazebo/%d", "commits": {"nodes": [{"commit": {"statusCheckRollup": %s}}]}}`, index, rollup,
		))
	}
	return `{"data": {"repository": {"pullRequests": {"nodes": [` + strings.Join(nodes, ",") + `]}}}}`
}

func TestCIStatesAsksForTheRepositoryUnderItsOwnOwner(t *testing.T) {
	stub := stubShell(t, cannedCIStates("SUCCESS"))

	_, err := providerFor(gazebo, partners).CIStates(partners)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{
		"gh", "api", "graphql", "-f", "query=" + ciStatesQuery, "-f", "owner=" + partners.Owner, "-f", "name=" + partners.Name,
	})
}

func TestCIStatesTranslateEachCheckRollupState(t *testing.T) {
	rollups := []string{"SUCCESS", "FAILURE", "ERROR", "PENDING", "EXPECTED", "", "SOMETHING_NEW"}
	stubShell(t, cannedCIStates(rollups...))

	states, err := providerFor(gazebo).CIStates(gazebo)

	expected := []pullrequests.CIState{
		pullrequests.CIStatePassing, pullrequests.CIStateFailing, pullrequests.CIStateFailing, pullrequests.CIStateRunning,
		pullrequests.CIStateRunning, pullrequests.CIStateNone, pullrequests.CIStateNone,
	}
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range expected {
		url := fmt.Sprintf("https://example.test/gazebo/%d", index)
		if got, present := states[url]; !present || got != want {
			t.Errorf("state of %s = %v (present %v), expected %v", url, got, present, want)
		}
	}
}

func TestCIStatesRecordNoChecksForACommitWithoutAnyStatus(t *testing.T) {
	stubShell(t, `{"data": {"repository": {"pullRequests": {"nodes": [
	  {"url": "https://example.test/gazebo/1", "commits": {"nodes": []}}
	]}}}}`)

	states, err := providerFor(gazebo).CIStates(gazebo)

	if state, present := states["https://example.test/gazebo/1"]; err != nil || !present || state != pullrequests.CIStateNone {
		t.Fatalf("states = %v, err = %v", states, err)
	}
}
