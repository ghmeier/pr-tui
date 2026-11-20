package workflows

import (
	"slices"
	"sync"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

// inEach runs the lookup for every item at the same time, so that a slow one does not hold up
// the others. The results are in the order of the items. The first error, in that order, is the
// one returned.
func inEach[Item, Result any](items []Item, lookup func(Item) ([]Result, error)) ([]Result, error) {
	results := make([][]Result, len(items))
	failures := make([]error, len(items))
	var group sync.WaitGroup
	for index, item := range items {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index], failures[index] = lookup(item)
		}()
	}
	group.Wait()
	var combined []Result
	for index, found := range results {
		if failures[index] != nil {
			return nil, failures[index]
		}
		combined = append(combined, found...)
	}
	return combined, nil
}

// rowsOf wraps each item as a row of a board.
func rowsOf[Item any](items []Item, wrap func(Item) plugin.Row) []plugin.Row {
	rows := make([]plugin.Row, len(items))
	for index, item := range items {
		rows[index] = wrap(item)
	}
	return rows
}

// matchesWorkflow says whether a workflow, given by its name and its file's base name, is
// among the names a section asks for. A section that names none wants every workflow.
func matchesWorkflow(wanted []string, name, file string) bool {
	return len(wanted) == 0 || slices.Contains(wanted, name) || slices.Contains(wanted, file)
}

func (configured section) matchesRun(run Run) bool {
	return matchesWorkflow(configured.workflows, run.Workflow, run.WorkflowFile) &&
		(len(configured.statuses) == 0 || slices.Contains(configured.statuses, run.Status)) &&
		(len(configured.triggers) == 0 || slices.Contains(configured.triggers, run.Trigger))
}

func (configured section) matchesWorkflow(workflow Workflow) bool {
	if !matchesWorkflow(configured.workflows, workflow.Name, workflow.File) {
		return false
	}
	if len(configured.triggers) == 0 {
		return true
	}
	return slices.ContainsFunc(workflow.Triggers, func(trigger Trigger) bool {
		return slices.Contains(configured.triggers, trigger)
	})
}
