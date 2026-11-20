package workflows

import (
	"slices"
	"strings"
	"sync"

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
			{Key: "x", Label: "run", Run: func(selection plugin.Selection) plugin.Effect {
				workflow := workflowOf(selection.Row)
				if !workflow.startableByHand() {
					return plugin.Effect{Notice: workflow.Name + " cannot be started by hand"}
				}
				return plugin.Effect{Screen: newFormScreen(provider, workflow, selection)}
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

// fetchLastRuns makes one fetch for each repository, so that a slow repository leaves only its
// own cells loading. A fetch asks for the repository's recent runs once, which answers for most
// workflows, and asks for the latest run of only the workflows that had none among them.
func fetchLastRuns(provider Provider, rows []plugin.Row) []plugin.Fetch {
	var repositories []Repository
	rowsByRepository := map[Repository][]plugin.Row{}
	for _, row := range rows {
		repository := row.(WorkflowRow).Workflow.Repository
		if _, seen := rowsByRepository[repository]; !seen {
			repositories = append(repositories, repository)
		}
		rowsByRepository[repository] = append(rowsByRepository[repository], row)
	}
	fetches := make([]plugin.Fetch, len(repositories))
	for index, repository := range repositories {
		fetches[index] = func() (map[string]any, error) {
			return lastRunsOf(provider, repository, rowsByRepository[repository])
		}
	}
	return fetches
}

// lastRunsOf loads the last run of the rows' workflows, which all belong to the repository. A
// workflow whose own lookup fails is left out, so that it alone stays unloaded.
func lastRunsOf(provider Provider, repository Repository, rows []plugin.Row) (map[string]any, error) {
	recent, err := provider.LatestRuns(repository)
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	var mutex sync.Mutex
	var group sync.WaitGroup
	for _, row := range rows {
		workflow := row.(WorkflowRow).Workflow
		if run, found := recent[workflow.File]; found {
			values[row.Key()] = lastRun{run: run, exists: true}
			continue
		}
		group.Add(1)
		go func() {
			defer group.Done()
			run, exists, err := provider.LatestRun(workflow)
			if err != nil {
				return
			}
			mutex.Lock()
			defer mutex.Unlock()
			values[row.Key()] = lastRun{run: run, exists: exists}
		}()
	}
	group.Wait()
	return values, nil
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
