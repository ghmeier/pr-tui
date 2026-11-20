package workflows

import (
	"slices"
	"time"

	"github.com/ghmeier/pr-tui/internal/ui"
)

type Repository struct {
	Owner string
	Name  string
}

func (repository Repository) FullName() string {
	return repository.Owner + "/" + repository.Name
}

// RunStatus is where a run is in its life, or how it ended.
type RunStatus int

const (
	RunStatusQueued RunStatus = iota
	RunStatusRunning
	RunStatusSucceeded
	RunStatusFailed
	RunStatusCancelled
	RunStatusSkipped
)

// QueueState is where a pull request stands in a merge queue.
type QueueState int

const (
	QueueStateQueued QueueState = iota
	QueueStateAwaitingChecks
	QueueStateMergeable
	QueueStateUnmergeable
	QueueStateLocked
)

// QueueEntry is one pull request waiting in a repository's merge queue.
type QueueEntry struct {
	Repository Repository
	// Position counts from the front of the queue, which is the next to merge.
	Position int
	State    QueueState
	// Checks is the combined state of the entry's checks.
	Checks     RunStatus
	Number     int
	Title      string
	URL        string
	Author     string
	EnqueuedAt time.Time
	// EstimatedTimeToMerge is zero when the forge gives no estimate.
	EstimatedTimeToMerge time.Duration
}

// Trigger is what started a run, or what starts a workflow.
type Trigger int

const (
	TriggerOther Trigger = iota
	TriggerPush
	TriggerPullRequest
	TriggerSchedule
	TriggerManual
	TriggerMergeQueue
)

// Run is one execution of a workflow.
type Run struct {
	Repository Repository
	ID         int64
	URL        string
	// Workflow is the workflow's name, and WorkflowFile the base name of its file, such as
	// deploy.yaml.
	Workflow     string
	WorkflowFile string
	Title        string
	Branch       string
	Trigger      Trigger
	Status       RunStatus
	Actor        string
	CreatedAt    time.Time
	StartedAt    time.Time
	UpdatedAt    time.Time
}

// Workflow is a definition that starts runs.
type Workflow struct {
	Repository Repository
	Name       string
	// File is the base name of the workflow's file.
	File     string
	URL      string
	Triggers []Trigger
	// Schedules are the cron expressions as written in the workflow.
	Schedules []string
	// Inputs are what a manual run asks for, in the order the workflow declares them.
	Inputs []WorkflowInput
	// DefaultBranch is the branch a manual run starts on unless told otherwise.
	DefaultBranch string
}

// InputKind is how a workflow input is entered.
type InputKind int

const (
	InputKindText InputKind = iota
	InputKindChoice
	InputKindBoolean
	InputKindNumber
	InputKindEnvironment
)

// WorkflowInput is a value that a manual run of a workflow asks for.
type WorkflowInput struct {
	Name        string
	Description string
	Required    bool
	// Default is the value as written in the workflow, and empty when it has none.
	Default string
	Kind    InputKind
	// Options are the values a choice allows, in the order the workflow lists them.
	Options []string
}

// startableByHand says whether the workflow accepts manual runs.
func (workflow Workflow) startableByHand() bool {
	return slices.Contains(workflow.Triggers, TriggerManual)
}

type Job struct {
	Name        string
	Status      RunStatus
	StartedAt   time.Time
	CompletedAt time.Time
	URL         string
	Steps       []Step
}

type Step struct {
	Name        string
	Status      RunStatus
	StartedAt   time.Time
	CompletedAt time.Time
}

// RunQuery asks for the most recent runs of one repository. Empty Statuses and Triggers mean
// any.
type RunQuery struct {
	Repository Repository
	Statuses   []RunStatus
	Triggers   []Trigger
	// WorkflowFile, when set, limits the query to the runs of the workflow with this file base
	// name, and Limit then counts that workflow's runs.
	WorkflowFile string
	Limit        int
}

// finished says whether the run has stopped, which is when its last update is its end.
func (status RunStatus) finished() bool {
	return status != RunStatusQueued && status != RunStatusRunning
}

// name is the word the configuration file and the screens use for the status.
func (status RunStatus) name() string {
	switch status {
	case RunStatusQueued:
		return "queued"
	case RunStatusRunning:
		return "running"
	case RunStatusSucceeded:
		return "succeeded"
	case RunStatusFailed:
		return "failed"
	case RunStatusCancelled:
		return "cancelled"
	}
	return "skipped"
}

func (status RunStatus) mark() string {
	switch status {
	case RunStatusQueued:
		return "⏳"
	case RunStatusRunning:
		return "🟡"
	case RunStatusSucceeded:
		return "🟢"
	case RunStatusFailed:
		return "🔴"
	case RunStatusCancelled:
		return "⚪"
	}
	return "⏭️"
}

// word is how a queue state reads in a cell.
func (state QueueState) word() string {
	switch state {
	case QueueStateAwaitingChecks:
		return "awaiting checks"
	case QueueStateMergeable:
		return "mergeable"
	case QueueStateUnmergeable:
		return "unmergeable"
	case QueueStateLocked:
		return "locked"
	}
	return "queued"
}

// word is how a trigger reads in a cell.
func (trigger Trigger) word() string {
	switch trigger {
	case TriggerPush:
		return "push"
	case TriggerPullRequest:
		return "pull request"
	case TriggerSchedule:
		return "schedule"
	case TriggerManual:
		return "manual"
	case TriggerMergeQueue:
		return "merge queue"
	}
	return "other"
}

// startedAt falls back to the creation time for a run that has not started yet.
func (run Run) startedAt() time.Time {
	if run.StartedAt.IsZero() {
		return run.CreatedAt
	}
	return run.StartedAt
}

// duration is how long the run took, or has been going for. A run that has not started has
// none.
func (run Run) duration(now time.Time) time.Duration {
	return elapsed(run.StartedAt, run.UpdatedAt, run.Status.finished(), now)
}

func (job Job) duration(now time.Time) time.Duration {
	return elapsed(job.StartedAt, job.CompletedAt, job.CompletedAt.After(job.StartedAt), now)
}

func (step Step) duration(now time.Time) time.Duration {
	return elapsed(step.StartedAt, step.CompletedAt, step.CompletedAt.After(step.StartedAt), now)
}

// elapsed measures from the start to the end of something that has ended, or to now for
// something still going. Something that never started has no duration.
func elapsed(start, end time.Time, ended bool, now time.Time) time.Duration {
	switch {
	case start.IsZero():
		return 0
	case ended:
		return max(end.Sub(start), 0)
	}
	return max(now.Sub(start), 0)
}

// ageText is how long ago the time was, such as "4m ago"; it is empty for an unknown time.
func ageText(since time.Time) string {
	if since.IsZero() {
		return ""
	}
	return ui.AgeText(since) + " ago"
}
