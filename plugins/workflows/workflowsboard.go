package workflows

import (
	"slices"
	"strings"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

const lastRunHeader = "Last run"

// WorkflowRow is a workflow as a row of a workflows section. Other plugins that add columns or
// actions to the board receive these.
type WorkflowRow struct {
	Workflow Workflow
}

// Key is the workflow's repository and file, which stay the same across refreshes.
func (row WorkflowRow) Key() string {
	return row.Workflow.Repository.FullName() + "/" + row.Workflow.File
}

// lastRun is what the Last run column loads for a workflow. A workflow that has never run is
// loaded, with exists false.
type lastRun struct {
	run    Run
	exists bool
}

func workflowsBoard(provider Provider, configured section) plugin.Board {
	workflowOf := func(row plugin.Row) Workflow { return row.(WorkflowRow).Workflow }
	return plugin.Board{
		Name:    boardName(configured.title),
		Title:   configured.title,
		Tab:     tabTitle,
		RowNoun: "workflows",
		Load:    func() ([]plugin.Row, error) { return loadWorkflows(provider, configured) },
		Columns: []plugin.Column{
			textColumn("Repo", 0, false, func(row plugin.Row) string { return workflowOf(row).Repository.Name }),
			textColumn("Workflow", 0, true, func(row plugin.Row) string { return workflowOf(row).Name }),
			textColumn("Schedule", 30, false, func(row plugin.Row) string { return strings.Join(workflowOf(row).Schedules, ", ") }),
			{Header: lastRunHeader, Cell: func(_ plugin.Row, data any) []ui.Segment { return lastRunSegments(data) },
				Load: func(rows []plugin.Row) []plugin.Fetch { return fetchLastRuns(provider, rows) }},
		},
		Actions: []plugin.Action{
			{Key: "enter", Label: "jobs", Run: func(selection plugin.Selection) plugin.Effect {
				latest, loaded := selection.Data(lastRunHeader).(lastRun)
				switch {
				case !loaded:
					return plugin.Effect{Notice: "The last run is still loading"}
				case !latest.exists:
					return plugin.Effect{Notice: "No runs yet"}
				}
				return plugin.Effect{Screen: newRunScreen(provider, latest.run, selection)}
			}},
			{Key: "o", Label: "open", Run: func(selection plugin.Selection) plugin.Effect {
				return openInBrowser(workflowOf(selection.Row).URL)
			}},
		},
		Facets: []plugin.Facet{{
			Title: "Repository", OptionsFromRows: true,
			Value: func(row plugin.Row) string { return workflowOf(row).Repository.FullName() },
		}},
	}
}

func loadWorkflows(provider Provider, configured section) ([]plugin.Row, error) {
	workflows, err := inEach(configured.repositories, provider.Workflows)
	if err != nil {
		return nil, err
	}
	workflows = slices.DeleteFunc(workflows, func(workflow Workflow) bool { return !configured.matchesWorkflow(workflow) })
	return rowsOf(workflows, func(workflow Workflow) plugin.Row { return WorkflowRow{Workflow: workflow} }), nil
}

// fetchLastRuns makes one fetch for each workflow, so that a slow lookup leaves only its own
// cell loading.
func fetchLastRuns(provider Provider, rows []plugin.Row) []plugin.Fetch {
	fetches := make([]plugin.Fetch, len(rows))
	for index, row := range rows {
		fetches[index] = func() (map[string]any, error) {
			run, exists, err := provider.LatestRun(row.(WorkflowRow).Workflow)
			if err != nil {
				return nil, err
			}
			return map[string]any{row.Key(): lastRun{run: run, exists: exists}}, nil
		}
	}
	return fetches
}

func lastRunSegments(data any) []ui.Segment {
	latest, loaded := data.(lastRun)
	switch {
	case !loaded:
		return []ui.Segment{{Text: "…", Style: ui.DimStyle}}
	case !latest.exists:
		return []ui.Segment{{Text: "never", Style: ui.DimStyle}}
	}
	return ui.PlainSegments(latest.run.Status.mark() + " " + ageText(latest.run.startedAt()))
}
