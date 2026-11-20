package workflows

import (
	"slices"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/shell"
)

// RunRow is a workflow run as a row of a runs section. Other plugins that add columns or
// actions to the board receive these.
type RunRow struct {
	Run Run
}

// Key is the run's URL, which stays the same across refreshes.
func (row RunRow) Key() string {
	return row.Run.URL
}

func runsBoard(provider Provider, configured section) plugin.Board {
	runOf := func(row plugin.Row) Run { return row.(RunRow).Run }
	return plugin.Board{
		Name:    boardName(configured.title),
		Title:   configured.title,
		Tab:     tabTitle,
		RowNoun: "runs",
		Load:    func() ([]plugin.Row, error) { return loadRuns(provider, configured) },
		Columns: []plugin.Column{
			textColumn("Status", 0, false, func(row plugin.Row) string { return runOf(row).Status.mark() }),
			textColumn("Repo", 0, false, func(row plugin.Row) string { return runOf(row).Repository.Name }),
			textColumn("Workflow", 24, false, func(row plugin.Row) string { return runOf(row).Workflow }),
			textColumn("Title", 0, true, func(row plugin.Row) string { return runOf(row).Title }),
			textColumn("Started", 0, false, func(row plugin.Row) string { return ageText(runOf(row).startedAt()) }),
			textColumn("Duration", 0, false, func(row plugin.Row) string { return runDuration(runOf(row)) }),
			textColumn("Branch", 30, false, func(row plugin.Row) string { return runOf(row).Branch }),
			textColumn("Trigger", 0, false, func(row plugin.Row) string { return runOf(row).Trigger.word() }),
		},
		Actions: []plugin.Action{
			{Key: "enter", Label: "jobs", Run: func(selection plugin.Selection) plugin.Effect {
				return plugin.Effect{Screen: newRunScreen(provider, runOf(selection.Row), selection)}
			}},
			{Key: "x", Label: "re-run", Run: func(selection plugin.Selection) plugin.Effect {
				return rerunEffect(provider, runOf(selection.Row))
			}},
			{Key: "o", Label: "open", Run: func(selection plugin.Selection) plugin.Effect {
				return openInBrowser(runOf(selection.Row).URL)
			}},
		},
		Facets: []plugin.Facet{
			{
				Title: "Repository", OptionsFromRows: true,
				Value: func(row plugin.Row) string { return runOf(row).Repository.FullName() },
			},
			{
				Title: "Workflow", OptionsFromRows: true,
				Value: func(row plugin.Row) string { return runOf(row).Workflow },
			},
		},
	}
}

// loadRuns asks for the recent runs and lists them newest first. A section that names workflows
// asks for each matching workflow's runs separately, so that busy workflows do not use up the
// limit of quiet ones; otherwise each repository is asked once.
func loadRuns(provider Provider, configured section) ([]plugin.Row, error) {
	runs, err := inEach(configured.repositories, func(repository Repository) ([]Run, error) {
		query := RunQuery{Repository: repository, Statuses: configured.statuses, Triggers: configured.triggers, Limit: configured.limit}
		if len(configured.workflows) == 0 {
			return provider.Runs(query)
		}
		return runsOfNamedWorkflows(provider, configured, query)
	})
	if err != nil {
		return nil, err
	}
	runs = slices.DeleteFunc(runs, func(run Run) bool { return !configured.matchesRun(run) })
	slices.SortStableFunc(runs, func(left, right Run) int { return right.CreatedAt.Compare(left.CreatedAt) })
	return rowsOf(runs, func(run Run) plugin.Row { return RunRow{Run: run} }), nil
}

// runsOfNamedWorkflows resolves the configured names to the repository's workflow files. A name
// that matches no workflow of the repository is skipped for it.
func runsOfNamedWorkflows(provider Provider, configured section, query RunQuery) ([]Run, error) {
	workflows, err := provider.Workflows(query.Repository)
	if err != nil {
		return nil, err
	}
	workflows = slices.DeleteFunc(workflows, func(workflow Workflow) bool {
		return !matchesWorkflow(configured.workflows, workflow.Name, workflow.File)
	})
	return inEach(workflows, func(workflow Workflow) ([]Run, error) {
		perWorkflow := query
		perWorkflow.WorkflowFile = workflow.File
		return provider.Runs(perWorkflow)
	})
}

const (
	rerunFailedJobs = "re-run failed jobs"
	rerunAllJobs    = "re-run all jobs"
)

// rerunEffect asks which jobs of a finished run to run again, and then confirms. Running only
// the failed jobs is offered only for a run that failed.
func rerunEffect(provider Provider, run Run) plugin.Effect {
	if !run.Status.finished() {
		return plugin.Effect{Notice: run.Workflow + " is still running"}
	}
	options := []string{rerunAllJobs}
	if run.Status == RunStatusFailed {
		options = []string{rerunFailedJobs, rerunAllJobs}
	}
	return plugin.Effect{Choice: &plugin.Choice{
		Question: "Re-run " + run.Workflow + " in " + run.Repository.Name,
		Options:  options,
		Pick: func(option string) plugin.Effect {
			failedOnly := option == rerunFailedJobs
			which := "all jobs"
			if failedOnly {
				which = "the failed jobs"
			}
			return plugin.Effect{
				Confirm: "Re-run " + which + " of " + run.Workflow + " " + run.Title + "?",
				Notice:  "Re-running " + run.Workflow + "…",
				Command: func() error { return provider.Rerun(run, failedOnly) },
				Refresh: true,
			}
		},
	}}
}

// openInBrowser is the effect of the o key, on the board and on the screens.
func openInBrowser(url string) plugin.Effect {
	return plugin.Effect{Command: func() error {
		_, err := shell.Run("open", url)
		return err
	}}
}
