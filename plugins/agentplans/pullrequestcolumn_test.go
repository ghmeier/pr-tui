package agentplans

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
	"github.com/ghmeier/pr-tui/plugins/pullrequests/pullrequeststest"
)

// planFixture is a checkout named storefront with two documents linked to branches, and the
// pull requests that the column is matched against.
type planFixture struct {
	apl      *fakeApl
	checkout string
	// linked is the pull request whose head branch both documents are linked to.
	linked pullrequests.PullRequest
	// unlinked has a head branch that no document is linked to.
	unlinked pullrequests.PullRequest
	// elsewhere belongs to a repository that has no configured checkout.
	elsewhere pullrequests.PullRequest
	first     doc
	second    doc
}

func newPlanFixture(t *testing.T) planFixture {
	t.Helper()
	apl := installFakeApl(t, false)
	source := pullrequests.Repository{Owner: "acme", Name: "storefront"}
	updated := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	example := planFixture{
		apl:   apl,
		first: doc{file: "plan/sign-in.md", kind: "plan", title: "Plan the sign-in form", updated: "2026-11-12", branches: []string{"feature/sign-in"}},
		linked: pullrequests.PullRequest{
			Repository: source, Number: 1, Title: "Add the sign-in form", URL: "https://example.test/pull/1", Author: "alice", HeadBranch: "feature/sign-in", UpdatedAt: updated,
		},
		unlinked: pullrequests.PullRequest{
			Repository: source, Number: 2, Title: "Tidy the build scripts", URL: "https://example.test/pull/2", Author: "bob", HeadBranch: "chore/tidy", UpdatedAt: updated.Add(-time.Hour),
		},
		elsewhere: pullrequests.PullRequest{
			Repository: pullrequests.Repository{Owner: "acme", Name: "unconfigured"}, Number: 3, Title: "Rename the helpers", URL: "https://example.test/pull/3", Author: "carol",
			HeadBranch: "feature/sign-in", UpdatedAt: updated.Add(-2 * time.Hour),
		},
		second: doc{file: "research/passwords.md", kind: "research", title: "Research password rules", updated: "2026-11-11", branches: []string{"feature/sign-in"}},
	}
	example.checkout = apl.newCheckout(t, source.Name, example.first)
	return example
}

// withBothDocs lists the second document in the checkout too, so two documents are linked to
// the same branch.
func (example *planFixture) withBothDocs(t *testing.T) {
	t.Helper()
	example.checkout = example.apl.newCheckout(t, example.linked.Repository.Name, example.first, example.second)
}

// startedWithPullRequests loads the pull request board, with the column added to it, and the
// agent plans board for the fixture's checkout.
func (example planFixture) startedWithPullRequests(t *testing.T) *plugin.Model {
	t.Helper()
	provider := &pullrequeststest.Provider{
		Source: example.linked.Repository,
		Open:   []pullrequests.PullRequest{example.linked, example.unlinked, example.elsewhere},
	}
	topLevel := fmt.Sprintf("plugins = [%q, %q]\n[pull_requests]\nprovider = %q\n",
		pullrequests.BoardName, pluginName, pullrequeststest.Register(t, provider))
	model := hostWithConfiguration(t, topLevel, plugin.Registered(), []string{example.checkout}).Model()
	plugintest.Start(t, model)
	plugintest.Resize(t, model, 200, 30)
	return model
}

func (example planFixture) callsIn(directory string) []call {
	var matching []call
	for _, received := range example.apl.calls {
		if received.directory == directory {
			matching = append(matching, received)
		}
	}
	return matching
}

func TestPlanColumnShowsTheTitleOfTheDocumentLinkedToThePullRequestsBranch(t *testing.T) {
	example := newPlanFixture(t)

	model := example.startedWithPullRequests(t)

	if header := lineWith(model, "Review requested"); !strings.Contains(header, "Plan") {
		t.Fatalf("header = %q", header)
	}
	if line := lineWith(model, example.linked.Title); !strings.Contains(line, example.first.title) {
		t.Fatalf("line = %q, expected the title %q", line, example.first.title)
	}
}

func TestPlanColumnIsEmptyForAPullRequestWhoseBranchHasNoLinkedDocument(t *testing.T) {
	example := newPlanFixture(t)

	model := example.startedWithPullRequests(t)

	if line := lineWith(model, example.unlinked.Title); strings.Contains(line, example.first.title) {
		t.Fatalf("line = %q, expected no document title", line)
	}
}

func TestPlanColumnShowsTheFirstTitleAndCountsTheOtherDocumentsLinkedToTheBranch(t *testing.T) {
	example := newPlanFixture(t)
	example.withBothDocs(t)

	model := example.startedWithPullRequests(t)

	line := lineWith(model, example.linked.Title)
	if !strings.Contains(line, example.first.title+" +1") || strings.Contains(line, example.second.title) {
		t.Fatalf("line = %q, expected %q followed by +1", line, example.first.title)
	}
}

func TestPlanColumnLeavesAPullRequestOfAnUnconfiguredRepositoryEmptyAndDoesNotRunAplForIt(t *testing.T) {
	example := newPlanFixture(t)

	model := example.startedWithPullRequests(t)

	// The board hides the pull request of a repository that is not listed in its filter, so
	// the directories apl ran in are what shows the column skipped it.
	if line := lineWith(model, example.linked.Title); !strings.Contains(line, example.first.title) {
		t.Fatalf("line = %q, expected the title %q", line, example.first.title)
	}
	for _, received := range example.apl.calls {
		if received.directory != example.checkout {
			t.Fatalf("apl ran in %q, which is not a configured checkout", received.directory)
		}
	}
}

func TestPlanColumnListsEachCheckoutWithPullRequestsOnceAndSkipsOthers(t *testing.T) {
	example := newPlanFixture(t)
	idle := example.apl.newCheckout(t, "warehouse")
	provider := &pullrequeststest.Provider{
		Source: example.linked.Repository,
		Open:   []pullrequests.PullRequest{example.linked},
	}
	topLevel := fmt.Sprintf("plugins = [%q, %q]\n[pull_requests]\nprovider = %q\n",
		pullrequests.BoardName, pluginName, pullrequeststest.Register(t, provider))
	model := hostWithConfiguration(t, topLevel, plugin.Registered(), []string{example.checkout, idle}).Model()

	plugintest.Start(t, model)

	// The agent plans board lists every checkout once, and the column lists only the checkout
	// that has a pull request.
	expectedCommand := []string{"apl", "ls", "--json"}
	withPullRequest, without := example.callsIn(example.checkout), example.callsIn(idle)
	if len(withPullRequest) != 2 || len(without) != 1 {
		t.Fatalf("calls in %s = %v, calls in %s = %v", example.checkout, withPullRequest, idle, without)
	}
	for _, received := range append(withPullRequest, without...) {
		if strings.Join(received.arguments, " ") != strings.Join(expectedCommand, " ") {
			t.Fatalf("arguments = %v, expected %v", received.arguments, expectedCommand)
		}
	}
}

func TestPluginStartsWithoutThePlanColumnWhenPullRequestsAreNotEnabled(t *testing.T) {
	example := newPlanFixture(t)

	model := startedModel(t, example.checkout)

	if line := lineWith(model, example.first.title); line == "" {
		t.Fatalf("the board should list %q:\n%s", example.first.title, strings.Join(screenLines(model), "\n"))
	}
}
