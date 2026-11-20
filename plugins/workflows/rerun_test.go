package workflows

import (
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// runWithStatus is the fixture run that has the status; the fixture has one of each of these.
func runWithStatus(provider *fakeProvider, status RunStatus) Run {
	for _, runs := range provider.runs {
		for _, run := range runs {
			if run.Status == status {
				return run
			}
		}
	}
	panic("the fixture has no run with that status")
}

func startedRunsModel(t *testing.T, provider *fakeProvider, status RunStatus) *plugin.Model {
	t.Helper()
	return startedModel(t, provider, runsWithStatus(status.name()))
}

func TestRerunKeyOnARunStillGoingShowsANoticeAndAsksNothing(t *testing.T) {
	for _, status := range []RunStatus{RunStatusRunning, RunStatusQueued} {
		t.Run(status.name(), func(t *testing.T) {
			provider := newFakeProvider()
			run := runWithStatus(provider, status)
			model := startedRunsModel(t, provider, status)

			plugintest.Press(t, model, "x", "1", "y")

			assertShows(t, model, run.Workflow+" is still running")
			assertHides(t, model, rerunAllJobs)
			if reruns := provider.rerunRequests(); len(reruns) != 0 {
				t.Fatalf("re-ran %v", reruns)
			}
		})
	}
}

func TestRerunKeyOnAFailedRunOffersBothWaysToRunItAgain(t *testing.T) {
	provider := newFakeProvider()
	run := runWithStatus(provider, RunStatusFailed)
	model := startedRunsModel(t, provider, RunStatusFailed)

	plugintest.Press(t, model, "x")

	assertShows(t, model, "Re-run "+run.Workflow+" in "+run.Repository.Name, rerunFailedJobs, rerunAllJobs)
}

func TestRerunKeyOnASucceededRunOffersOnlyToRunAllJobs(t *testing.T) {
	provider := newFakeProvider()
	model := startedRunsModel(t, provider, RunStatusSucceeded)

	plugintest.Press(t, model, "x")

	assertShows(t, model, rerunAllJobs)
	assertHides(t, model, rerunFailedJobs)
}

func TestPickingTheFailedJobsAsksToConfirmAndThenRerunsOnlyThose(t *testing.T) {
	provider := newFakeProvider()
	run := runWithStatus(provider, RunStatusFailed)
	model := startedRunsModel(t, provider, RunStatusFailed)

	plugintest.Press(t, model, "x", "1")

	assertShows(t, model, "Re-run the failed jobs of "+run.Workflow+" "+run.Title+"?")
	if reruns := provider.rerunRequests(); len(reruns) != 0 {
		t.Fatalf("re-ran %v before the confirmation", reruns)
	}

	plugintest.Press(t, model, "y")

	reruns := provider.rerunRequests()
	if len(reruns) != 1 || reruns[0].run.ID != run.ID || !reruns[0].failedOnly {
		t.Fatalf("reruns = %+v", reruns)
	}
}

func TestPickingAllJobsOfAFailedRunRerunsThemAll(t *testing.T) {
	provider := newFakeProvider()
	run := runWithStatus(provider, RunStatusFailed)
	model := startedRunsModel(t, provider, RunStatusFailed)

	plugintest.Press(t, model, "x", "2")

	assertShows(t, model, "Re-run all jobs of "+run.Workflow+" "+run.Title+"?")

	plugintest.Press(t, model, "y")

	reruns := provider.rerunRequests()
	if len(reruns) != 1 || reruns[0].run.ID != run.ID || reruns[0].failedOnly {
		t.Fatalf("reruns = %+v", reruns)
	}
}

func TestDecliningTheRerunConfirmationRunsNothingAgain(t *testing.T) {
	provider := newFakeProvider()
	model := startedRunsModel(t, provider, RunStatusFailed)

	plugintest.Press(t, model, "x", "1", "n")

	if reruns := provider.rerunRequests(); len(reruns) != 0 {
		t.Fatalf("re-ran %v", reruns)
	}
}
