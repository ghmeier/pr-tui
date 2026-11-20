package github

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/workflows"
)

// apiRun is the part of GitHub's workflow run that the board uses.
type apiRun struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	DisplayTitle string    `json:"display_title"`
	HeadBranch   string    `json:"head_branch"`
	Event        string    `json:"event"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	CreatedAt    time.Time `json:"created_at"`
	RunStartedAt time.Time `json:"run_started_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	URL          string    `json:"html_url"`
	Actor        struct {
		Login string `json:"login"`
	} `json:"actor"`
}

func (run apiRun) neutral(repository workflows.Repository) workflows.Run {
	return workflows.Run{
		Repository: repository, ID: run.ID, URL: run.URL, Workflow: run.Name, WorkflowFile: baseName(run.Path),
		Title: run.DisplayTitle, Branch: run.HeadBranch, Trigger: triggerOfEvent(run.Event),
		Status: statusOf(run.Status, run.Conclusion), Actor: run.Actor.Login,
		CreatedAt: run.CreatedAt, StartedAt: run.RunStartedAt, UpdatedAt: run.UpdatedAt,
	}
}

func parseRuns(repository workflows.Repository, output []byte) ([]workflows.Run, error) {
	var response struct {
		WorkflowRuns []apiRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}
	runs := make([]workflows.Run, len(response.WorkflowRuns))
	for index, run := range response.WorkflowRuns {
		runs[index] = run.neutral(repository)
	}
	return runs, nil
}

func statusOf(status, conclusion string) workflows.RunStatus {
	switch status {
	case "in_progress":
		return workflows.RunStatusRunning
	case "completed":
		switch conclusion {
		case "success":
			return workflows.RunStatusSucceeded
		case "cancelled":
			return workflows.RunStatusCancelled
		case "skipped", "neutral":
			return workflows.RunStatusSkipped
		}
		return workflows.RunStatusFailed
	}
	return workflows.RunStatusQueued
}

func triggerOfEvent(event string) workflows.Trigger {
	switch event {
	case "push":
		return workflows.TriggerPush
	case "pull_request", "pull_request_target":
		return workflows.TriggerPullRequest
	case "schedule":
		return workflows.TriggerSchedule
	case "workflow_dispatch":
		return workflows.TriggerManual
	case "merge_group":
		return workflows.TriggerMergeQueue
	}
	return workflows.TriggerOther
}

// filterOf is the value of the status parameter that selects exactly the runs of one status,
// or "" when no single value does. Some statuses are several of GitHub's, which the
// parameter cannot ask for together.
func filterOf(status workflows.RunStatus) string {
	switch status {
	case workflows.RunStatusRunning:
		return "in_progress"
	case workflows.RunStatusSucceeded:
		return "success"
	case workflows.RunStatusCancelled:
		return "cancelled"
	}
	return ""
}

// eventFilterOf is the value of the event parameter that selects exactly the runs of one
// trigger, or "" when no single value does.
func eventFilterOf(trigger workflows.Trigger) string {
	switch trigger {
	case workflows.TriggerPush:
		return "push"
	case workflows.TriggerSchedule:
		return "schedule"
	case workflows.TriggerManual:
		return "workflow_dispatch"
	case workflows.TriggerMergeQueue:
		return "merge_group"
	}
	return ""
}

// Runs asks GitHub to narrow the runs by status or by event only when one value does it
// exactly, and narrows by the rest here.
func (github *provider) Runs(query workflows.RunQuery) ([]workflows.Run, error) {
	endpoint := fmt.Sprintf("repos/%s/actions/runs?per_page=%d", query.Repository.FullName(), query.Limit)
	if len(query.Statuses) == 1 && filterOf(query.Statuses[0]) != "" {
		endpoint += "&status=" + filterOf(query.Statuses[0])
	}
	if len(query.Triggers) == 1 && eventFilterOf(query.Triggers[0]) != "" {
		endpoint += "&event=" + eventFilterOf(query.Triggers[0])
	}
	output, err := shell.Run("gh", "api", endpoint)
	if err != nil {
		return nil, err
	}
	runs, err := parseRuns(query.Repository, output)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(runs, func(run workflows.Run) bool {
		return (len(query.Statuses) > 0 && !slices.Contains(query.Statuses, run.Status)) ||
			(len(query.Triggers) > 0 && !slices.Contains(query.Triggers, run.Trigger))
	}), nil
}

func (github *provider) LatestRun(workflow workflows.Workflow) (workflows.Run, bool, error) {
	endpoint := fmt.Sprintf("repos/%s/actions/workflows/%s/runs?per_page=1", workflow.Repository.FullName(), workflow.File)
	output, err := shell.Run("gh", "api", endpoint)
	if err != nil {
		return workflows.Run{}, false, err
	}
	runs, err := parseRuns(workflow.Repository, output)
	if err != nil || len(runs) == 0 {
		return workflows.Run{}, false, err
	}
	return runs[0], true, nil
}

func (github *provider) Jobs(run workflows.Run) ([]workflows.Job, error) {
	endpoint := fmt.Sprintf("repos/%s/actions/runs/%d/jobs?per_page=100", run.Repository.FullName(), run.ID)
	output, err := shell.Run("gh", "api", endpoint)
	if err != nil {
		return nil, err
	}
	var response struct {
		Jobs []struct {
			Name        string    `json:"name"`
			Status      string    `json:"status"`
			Conclusion  string    `json:"conclusion"`
			StartedAt   time.Time `json:"started_at"`
			CompletedAt time.Time `json:"completed_at"`
			URL         string    `json:"html_url"`
			Steps       []struct {
				Name        string    `json:"name"`
				Status      string    `json:"status"`
				Conclusion  string    `json:"conclusion"`
				StartedAt   time.Time `json:"started_at"`
				CompletedAt time.Time `json:"completed_at"`
			} `json:"steps"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}
	jobs := make([]workflows.Job, len(response.Jobs))
	for index, job := range response.Jobs {
		jobs[index] = workflows.Job{
			Name: job.Name, Status: statusOf(job.Status, job.Conclusion), StartedAt: job.StartedAt, CompletedAt: job.CompletedAt, URL: job.URL,
		}
		for _, step := range job.Steps {
			jobs[index].Steps = append(jobs[index].Steps, workflows.Step{
				Name: step.Name, Status: statusOf(step.Status, step.Conclusion), StartedAt: step.StartedAt, CompletedAt: step.CompletedAt,
			})
		}
	}
	return jobs, nil
}
