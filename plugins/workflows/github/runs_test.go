package github

import (
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/plugins/workflows"
)

const runsResponse = `{"workflow_runs": [
  {"id": 11, "name": "Deploy CI", "path": ".github/workflows/deploy.yaml", "display_title": "Ship the cart",
   "head_branch": "main", "event": "push", "status": "completed", "conclusion": "success",
   "created_at": "2026-11-10T12:00:00Z", "run_started_at": "2026-11-10T12:00:05Z", "updated_at": "2026-11-10T12:12:05Z",
   "html_url": "https://github.com/acme/storefront/actions/runs/11", "actor": {"login": "person"}},
  {"id": 12, "name": "Checks", "path": ".github/workflows/checks.yml@refs/pull/9/merge", "display_title": "Fix the footer",
   "head_branch": "footer-fix", "event": "pull_request", "status": "in_progress", "conclusion": null,
   "created_at": "2026-11-10T13:00:00Z", "run_started_at": "2026-11-10T13:00:00Z", "updated_at": "2026-11-10T13:01:00Z",
   "html_url": "https://github.com/acme/storefront/actions/runs/12", "actor": {"login": "other"}}
]}`

func TestRunsAsksForTheMostRecentRunsOfTheRepository(t *testing.T) {
	stub := stubShell(t, `{"workflow_runs": []}`)

	_, err := newProvider().Runs(workflows.RunQuery{Repository: storefront, Limit: 20})

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, apiCommand("repos/acme/storefront/actions/runs?per_page=20"))
}

func TestRunsOfOneWorkflowUseTheWorkflowsEndpointWithTheSameParameters(t *testing.T) {
	stub := stubShell(t, `{"workflow_runs": []}`)

	newProvider().Runs(workflows.RunQuery{
		Repository: storefront, WorkflowFile: "deploy.yaml", Limit: 20,
		Statuses: []workflows.RunStatus{workflows.RunStatusRunning}, Triggers: []workflows.Trigger{workflows.TriggerPush},
	})
	newProvider().Runs(workflows.RunQuery{Repository: billing, WorkflowFile: "checks.yml", Limit: 5})

	stub.assertCommands(t,
		apiCommand("repos/acme/storefront/actions/workflows/deploy.yaml/runs?per_page=20&status=in_progress&event=push"),
		apiCommand("repos/acme/billing/actions/workflows/checks.yml/runs?per_page=5"),
	)
}

func TestRunsParsesEveryFieldTheBoardUses(t *testing.T) {
	stubShell(t, runsResponse)

	runs, err := newProvider().Runs(workflows.RunQuery{Repository: storefront, Limit: 20})

	expected := []workflows.Run{
		{
			Repository: storefront, ID: 11, URL: "https://github.com/acme/storefront/actions/runs/11", Workflow: "Deploy CI",
			WorkflowFile: "deploy.yaml", Title: "Ship the cart", Branch: "main", Trigger: workflows.TriggerPush,
			Status: workflows.RunStatusSucceeded, Actor: "person",
			CreatedAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC), StartedAt: time.Date(2026, 11, 10, 12, 0, 5, 0, time.UTC),
			UpdatedAt: time.Date(2026, 11, 10, 12, 12, 5, 0, time.UTC),
		},
		{
			Repository: storefront, ID: 12, URL: "https://github.com/acme/storefront/actions/runs/12", Workflow: "Checks",
			WorkflowFile: "checks.yml", Title: "Fix the footer", Branch: "footer-fix", Trigger: workflows.TriggerPullRequest,
			Status: workflows.RunStatusRunning, Actor: "other",
			CreatedAt: time.Date(2026, 11, 10, 13, 0, 0, 0, time.UTC), StartedAt: time.Date(2026, 11, 10, 13, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 11, 10, 13, 1, 0, 0, time.UTC),
		},
	}
	if err != nil || len(runs) != 2 || runs[0] != expected[0] || runs[1] != expected[1] {
		t.Fatalf("runs = %+v, error = %v", runs, err)
	}
}

func TestStatusAndConclusionMapToTheNeutralStatuses(t *testing.T) {
	cases := []struct {
		status, conclusion string
		expected           workflows.RunStatus
	}{
		{"queued", "", workflows.RunStatusQueued},
		{"waiting", "", workflows.RunStatusQueued},
		{"pending", "", workflows.RunStatusQueued},
		{"requested", "", workflows.RunStatusQueued},
		{"in_progress", "", workflows.RunStatusRunning},
		{"completed", "success", workflows.RunStatusSucceeded},
		{"completed", "failure", workflows.RunStatusFailed},
		{"completed", "timed_out", workflows.RunStatusFailed},
		{"completed", "startup_failure", workflows.RunStatusFailed},
		{"completed", "cancelled", workflows.RunStatusCancelled},
		{"completed", "skipped", workflows.RunStatusSkipped},
		{"completed", "neutral", workflows.RunStatusSkipped},
	}

	for _, example := range cases {
		if status := statusOf(example.status, example.conclusion); status != example.expected {
			t.Errorf("statusOf(%q, %q) = %v, expected %v", example.status, example.conclusion, status, example.expected)
		}
	}
}

func TestEventsMapToTheNeutralTriggers(t *testing.T) {
	cases := map[string]workflows.Trigger{
		"push":                workflows.TriggerPush,
		"pull_request":        workflows.TriggerPullRequest,
		"pull_request_target": workflows.TriggerPullRequest,
		"schedule":            workflows.TriggerSchedule,
		"workflow_dispatch":   workflows.TriggerManual,
		"merge_group":         workflows.TriggerMergeQueue,
		"release":             workflows.TriggerOther,
		"":                    workflows.TriggerOther,
	}

	for event, expected := range cases {
		if trigger := triggerOfEvent(event); trigger != expected {
			t.Errorf("triggerOfEvent(%q) = %v, expected %v", event, trigger, expected)
		}
	}
}

func TestRunsAsksGitHubToNarrowByAStatusOnlyWhenOneValueSelectsExactlyIt(t *testing.T) {
	cases := []struct {
		statuses []workflows.RunStatus
		endpoint string
	}{
		{[]workflows.RunStatus{workflows.RunStatusRunning}, "repos/acme/storefront/actions/runs?per_page=20&status=in_progress"},
		{[]workflows.RunStatus{workflows.RunStatusSucceeded}, "repos/acme/storefront/actions/runs?per_page=20&status=success"},
		{[]workflows.RunStatus{workflows.RunStatusCancelled}, "repos/acme/storefront/actions/runs?per_page=20&status=cancelled"},
		{[]workflows.RunStatus{workflows.RunStatusQueued}, "repos/acme/storefront/actions/runs?per_page=20"},
		{[]workflows.RunStatus{workflows.RunStatusFailed}, "repos/acme/storefront/actions/runs?per_page=20"},
		{[]workflows.RunStatus{workflows.RunStatusQueued, workflows.RunStatusRunning}, "repos/acme/storefront/actions/runs?per_page=20"},
	}

	for _, example := range cases {
		stub := stubShell(t, `{"workflow_runs": []}`)

		newProvider().Runs(workflows.RunQuery{Repository: storefront, Statuses: example.statuses, Limit: 20})

		stub.assertCommands(t, apiCommand(example.endpoint))
	}
}

func TestRunsAsksGitHubToNarrowByAnEventOnlyWhenOneTriggerIsAskedForAndOneEventSelectsExactlyIt(t *testing.T) {
	cases := []struct {
		triggers []workflows.Trigger
		endpoint string
	}{
		{[]workflows.Trigger{workflows.TriggerPush}, "repos/acme/storefront/actions/runs?per_page=20&event=push"},
		{[]workflows.Trigger{workflows.TriggerSchedule}, "repos/acme/storefront/actions/runs?per_page=20&event=schedule"},
		{[]workflows.Trigger{workflows.TriggerManual}, "repos/acme/storefront/actions/runs?per_page=20&event=workflow_dispatch"},
		{[]workflows.Trigger{workflows.TriggerMergeQueue}, "repos/acme/storefront/actions/runs?per_page=20&event=merge_group"},
		{[]workflows.Trigger{workflows.TriggerPullRequest}, "repos/acme/storefront/actions/runs?per_page=20"},
		{[]workflows.Trigger{workflows.TriggerPush, workflows.TriggerSchedule}, "repos/acme/storefront/actions/runs?per_page=20"},
	}

	for _, example := range cases {
		stub := stubShell(t, `{"workflow_runs": []}`)

		newProvider().Runs(workflows.RunQuery{Repository: storefront, Triggers: example.triggers, Limit: 20})

		stub.assertCommands(t, apiCommand(example.endpoint))
	}
}

func TestRunsCombinesTheStatusAndTheEventInOneRequest(t *testing.T) {
	stub := stubShell(t, `{"workflow_runs": []}`)

	newProvider().Runs(workflows.RunQuery{
		Repository: billing, Statuses: []workflows.RunStatus{workflows.RunStatusRunning}, Triggers: []workflows.Trigger{workflows.TriggerPush}, Limit: 5,
	})

	stub.assertCommands(t, apiCommand("repos/acme/billing/actions/runs?per_page=5&status=in_progress&event=push"))
}

func TestRunsFiltersByTheStatusesAndTriggersGitHubCouldNotNarrow(t *testing.T) {
	stubShell(t, runsResponse)

	kept, err := newProvider().Runs(workflows.RunQuery{
		Repository: storefront, Statuses: []workflows.RunStatus{workflows.RunStatusSucceeded, workflows.RunStatusQueued}, Limit: 20,
	})
	dropped, _ := newProvider().Runs(workflows.RunQuery{
		Repository: storefront, Statuses: []workflows.RunStatus{workflows.RunStatusFailed}, Triggers: []workflows.Trigger{workflows.TriggerPush, workflows.TriggerSchedule}, Limit: 20,
	})

	if err != nil || len(kept) != 1 || kept[0].ID != 11 {
		t.Fatalf("kept = %+v, error = %v", kept, err)
	}
	if len(dropped) != 0 {
		t.Fatalf("dropped = %+v", dropped)
	}
}

func TestRunsReturnsTheErrorOfGh(t *testing.T) {
	stubShellWith(t, func([]string) ([]byte, error) { return nil, errorFromGh })

	runs, err := newProvider().Runs(workflows.RunQuery{Repository: storefront, Limit: 20})

	if err != errorFromGh || runs != nil {
		t.Fatalf("runs = %v, error = %v", runs, err)
	}
}

func TestLatestRunAsksForTheNewestRunOfTheWorkflowsFile(t *testing.T) {
	stub := stubShell(t, runsResponse)
	workflow := workflows.Workflow{Repository: storefront, Name: "Deploy CI", File: "deploy.yaml"}

	run, exists, err := newProvider().LatestRun(workflow)

	if err != nil || !exists || run.ID != 11 || run.Repository != storefront {
		t.Fatalf("run = %+v, exists = %v, error = %v", run, exists, err)
	}
	stub.assertCommands(t, apiCommand("repos/acme/storefront/actions/workflows/deploy.yaml/runs?per_page=1"))
}

func TestLatestRunReportsAWorkflowThatNeverRan(t *testing.T) {
	stubShell(t, `{"workflow_runs": []}`)

	_, exists, err := newProvider().LatestRun(workflows.Workflow{Repository: storefront, File: "deploy.yaml"})

	if err != nil || exists {
		t.Fatalf("exists = %v, error = %v", exists, err)
	}
}

func TestLatestRunsAsksForTheRepositorysRecentRunsOnceAndKeepsTheNewestOfEachWorkflowFile(t *testing.T) {
	stub := stubShell(t, `{"workflow_runs": [
  {"id": 31, "name": "Deploy CI", "path": ".github/workflows/deploy.yaml", "head_branch": "main", "event": "push",
   "status": "completed", "conclusion": "success", "created_at": "2026-11-10T14:00:00Z"},
  {"id": 32, "name": "Checks", "path": ".github/workflows/checks.yml@refs/pull/9/merge", "head_branch": "fix", "event": "pull_request",
   "status": "in_progress", "conclusion": null, "created_at": "2026-11-10T13:00:00Z"},
  {"id": 33, "name": "Deploy CI", "path": ".github/workflows/deploy.yaml", "head_branch": "main", "event": "push",
   "status": "completed", "conclusion": "failure", "created_at": "2026-11-10T12:00:00Z"}
]}`)

	latest, err := newProvider().LatestRuns(storefront)

	if err != nil || len(latest) != 2 {
		t.Fatalf("latest = %+v, error = %v", latest, err)
	}
	if latest["deploy.yaml"].ID != 31 || latest["checks.yml"].ID != 32 || latest["deploy.yaml"].Repository != storefront {
		t.Fatalf("latest = %+v", latest)
	}
	stub.assertCommands(t, apiCommand("repos/acme/storefront/actions/runs?per_page=100"))
}

func TestLatestRunsFailsWhenGhFails(t *testing.T) {
	stubShellWith(t, func([]string) ([]byte, error) { return nil, errorFromGh })

	latest, err := newProvider().LatestRuns(storefront)

	if err != errorFromGh || latest != nil {
		t.Fatalf("latest = %v, error = %v", latest, err)
	}
}

func TestJobsAsksForTheRunsJobsAndParsesTheirSteps(t *testing.T) {
	stub := stubShell(t, `{"jobs": [
  {"name": "build", "status": "completed", "conclusion": "success", "started_at": "2026-11-10T12:00:00Z",
   "completed_at": "2026-11-10T12:01:32Z", "html_url": "https://github.com/acme/storefront/actions/runs/11/job/1",
   "steps": [
     {"name": "Check out", "status": "completed", "conclusion": "success", "started_at": "2026-11-10T12:00:00Z", "completed_at": "2026-11-10T12:00:05Z"},
     {"name": "Compile", "status": "in_progress", "conclusion": null, "started_at": "2026-11-10T12:00:05Z", "completed_at": null}]},
  {"name": "release", "status": "queued", "conclusion": null, "started_at": null, "completed_at": null,
   "html_url": "https://github.com/acme/storefront/actions/runs/11/job/2", "steps": []}
]}`)

	jobs, err := newProvider().Jobs(workflows.Run{Repository: storefront, ID: 11})

	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs = %+v, error = %v", jobs, err)
	}
	build := jobs[0]
	if build.Name != "build" || build.Status != workflows.RunStatusSucceeded || build.URL != "https://github.com/acme/storefront/actions/runs/11/job/1" ||
		build.CompletedAt.Sub(build.StartedAt) != 92*time.Second {
		t.Fatalf("build = %+v", build)
	}
	if len(build.Steps) != 2 || build.Steps[0].Name != "Check out" || build.Steps[0].Status != workflows.RunStatusSucceeded ||
		build.Steps[1].Status != workflows.RunStatusRunning || !build.Steps[1].CompletedAt.IsZero() {
		t.Fatalf("steps = %+v", build.Steps)
	}
	if jobs[1].Status != workflows.RunStatusQueued || !jobs[1].StartedAt.IsZero() || len(jobs[1].Steps) != 0 {
		t.Fatalf("release = %+v", jobs[1])
	}
	stub.assertCommands(t, apiCommand("repos/acme/storefront/actions/runs/11/jobs?per_page=100"))
}

func TestJobsReturnsTheErrorOfGh(t *testing.T) {
	stubShellWith(t, func([]string) ([]byte, error) { return nil, errorFromGh })

	jobs, err := newProvider().Jobs(workflows.Run{Repository: storefront, ID: 11})

	if err != errorFromGh || jobs != nil {
		t.Fatalf("jobs = %v, error = %v", jobs, err)
	}
}
