package workflows

import (
	"strings"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// openedScreen opens the screen of the newest run, which is the storefront deploy with two jobs.
func openedScreen(t *testing.T, provider *fakeProvider) *plugin.Model {
	t.Helper()
	model := startedModel(t, provider, "")
	plugintest.Press(t, model, "enter")
	return model
}

func TestRunScreenHeaderNamesTheRepositoryWorkflowTitleAndStatus(t *testing.T) {
	provider := newFakeProvider()
	run := provider.runs[storefrontRepository][1]

	model := openedScreen(t, provider)

	lines := strings.Split(screen(model), "\n")
	if lines[0] != run.Repository.FullName()+"  "+run.Workflow || !strings.Contains(lines[1], run.Title) ||
		!strings.Contains(lines[2], run.Status.mark()+" "+run.Status.name()) {
		t.Fatalf("header = %q", lines[:3])
	}
}

func TestRunScreenListsEveryJobWithItsMarkAndDuration(t *testing.T) {
	provider := newFakeProvider()
	jobs := provider.jobs[provider.runs[storefrontRepository][1].ID]

	model := openedScreen(t, provider)

	if line := lineWith(model, jobs[0].Name); !strings.Contains(line, RunStatusSucceeded.mark()) || !strings.Contains(line, "1m32s") {
		t.Fatalf("the first job is drawn as %q", line)
	}
	if line := lineWith(model, jobs[1].Name); !strings.Contains(line, RunStatusFailed.mark()) || !strings.Contains(line, "12m") {
		t.Fatalf("the second job is drawn as %q", line)
	}
}

func TestRunScreenShowsTheStepsOfOnlyTheSelectedJob(t *testing.T) {
	provider := newFakeProvider()
	jobs := provider.jobs[provider.runs[storefrontRepository][1].ID]

	model := openedScreen(t, provider)

	for _, step := range jobs[0].Steps {
		if !strings.Contains(screen(model), step.Name) {
			t.Fatalf("the steps of the selected job do not include %q:%s", step.Name, describe(model))
		}
	}
	if strings.Contains(screen(model), jobs[1].Steps[0].Name) {
		t.Fatalf("the steps of the other job are shown:%s", describe(model))
	}
}

func TestMovingTheCursorShowsTheStepsOfTheJobItReaches(t *testing.T) {
	provider := newFakeProvider()
	jobs := provider.jobs[provider.runs[storefrontRepository][1].ID]

	model := openedScreen(t, provider)
	plugintest.Press(t, model, "j")

	assertShows(t, model, jobs[1].Steps[0].Name)
	assertHides(t, model, jobs[0].Steps[0].Name)

	plugintest.Press(t, model, "k")

	assertShows(t, model, jobs[0].Steps[0].Name)
	assertHides(t, model, jobs[1].Steps[0].Name)
}

func TestTheCursorStopsAtTheLastJob(t *testing.T) {
	provider := newFakeProvider()
	jobs := provider.jobs[provider.runs[storefrontRepository][1].ID]

	model := openedScreen(t, provider)
	plugintest.Press(t, model, "down", "down", "down")

	assertShows(t, model, jobs[1].Steps[0].Name)
}

func TestOpenKeyOpensTheSelectedJobAndFallsBackToTheRun(t *testing.T) {
	calls := stubOpen(t)
	provider := newFakeProvider()
	run := provider.runs[storefrontRepository][1]
	jobs := provider.jobs[run.ID]

	model := openedScreen(t, provider)
	plugintest.Press(t, model, "j", "o")

	if len(*calls) != 1 || (*calls)[0][1] != jobs[1].URL {
		t.Fatalf("commands run = %v", *calls)
	}

	provider.jobs[run.ID] = nil
	plugintest.Press(t, model, "r", "o")

	if len(*calls) != 2 || (*calls)[1][1] != run.URL {
		t.Fatalf("commands run = %v", *calls)
	}
}

func TestReloadKeyAsksTheProviderForTheJobsAgain(t *testing.T) {
	provider := newFakeProvider()

	model := openedScreen(t, provider)
	before := provider.loadsOfJobs()
	plugintest.Press(t, model, "r")

	if before != 1 || provider.loadsOfJobs() != 2 {
		t.Fatalf("jobs were loaded %d times, then %d", before, provider.loadsOfJobs())
	}
}

func TestRunScreenShowsTheFirstLineOfAFailureToLoadTheJobs(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("Jobs", errorForTests)

	model := openedScreen(t, provider)

	assertShows(t, model, "Failed: provider failed")
	assertHides(t, model, "second line", "Loading jobs")
}

func TestRunScreenSaysWhenARunHasNoJobs(t *testing.T) {
	provider := newFakeProvider()

	model := startedModel(t, provider, "")
	plugintest.Press(t, model, "down", "enter")

	assertShows(t, model, "This run has no jobs.")
}

func TestRunScreenShowsAPlaceholderUntilTheJobsArrive(t *testing.T) {
	provider := newFakeProvider()
	run := provider.runs[storefrontRepository][1]
	opened := newRunScreen(provider, run, plugin.Selection{})

	view := opened.View(100, 20)

	if !strings.Contains(view, "Loading jobs") || strings.Contains(view, provider.jobs[run.ID][0].Name) {
		t.Fatalf("view = %q", view)
	}
}

func TestRunScreenIgnoresTheJobsOfAnotherRun(t *testing.T) {
	provider := newFakeProvider()
	run := provider.runs[storefrontRepository][1]
	opened := newRunScreen(provider, run, plugin.Selection{})

	opened.Update(jobsLoadedMessage{runURL: "https://example.test/other", jobs: provider.jobs[run.ID]})

	if !opened.loading || len(opened.jobs) != 0 {
		t.Fatalf("the screen took the jobs of another run: loading = %v, jobs = %v", opened.loading, opened.jobs)
	}
}

func TestEscapeAndQCloseTheRunScreen(t *testing.T) {
	for _, key := range []string{"esc", "q"} {
		t.Run(key, func(t *testing.T) {
			model := openedScreen(t, newFakeProvider())

			plugintest.Press(t, model, key)

			assertHides(t, model, "esc back")
		})
	}
}

func TestRunScreenScrollsToKeepTheCursorVisible(t *testing.T) {
	provider := newFakeProvider()
	run := provider.runs[storefrontRepository][1]
	var jobs []Job
	for _, name := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel"} {
		jobs = append(jobs, Job{Name: name, Status: RunStatusSucceeded, StartedAt: at(12, 0), CompletedAt: at(12, 0).Add(time.Minute)})
	}
	opened := newRunScreen(provider, run, plugin.Selection{})
	opened.Update(jobsLoadedMessage{runURL: run.URL, jobs: jobs})
	for range 7 {
		opened.Update(plugintest.KeyMessage("j"))
	}

	view := opened.View(100, 9)

	if !strings.Contains(view, "hotel") || strings.Contains(view, "alpha") {
		t.Fatalf("view = %q", view)
	}
}
