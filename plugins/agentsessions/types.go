package agentsessions

import "time"

// Status is what a session is doing, in terms every provider can report.
type Status string

const (
	StatusIdle    Status = "idle"
	StatusBusy    Status = "busy"
	StatusBlocked Status = "blocked"
	StatusUnknown Status = "unknown"
)

// PullRequestReference names a pull request that a session has mentioned.
type PullRequestReference struct {
	// Repository is "owner/name".
	Repository string
	Number     int
}

// Session is a live agent session.
type Session struct {
	// ID is stable for the life of the session.
	ID   string
	Name string
	// Repository is the base name of the checkout the session works in, the same for every
	// worktree of it, or empty when the session is not in a git repository.
	Repository string
	Branch     string
	Status     Status
	LastActive time.Time
	// PullRequests and Documents are what the session has mentioned, the latest mention first.
	PullRequests []PullRequestReference
	// Documents are paths relative to the .apl directory, such as "plan/2026-11-25-sign-in.md".
	Documents []string
}
