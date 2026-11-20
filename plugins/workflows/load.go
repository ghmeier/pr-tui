package workflows

import (
	"slices"
	"sync"
)

// inEachRepository runs the lookup for every repository at the same time, so that a slow
// repository does not hold up the others. The results are in the order of the repositories. The
// first error, in that order, is the one returned.
func inEachRepository[Result any](repositories []Repository, lookup func(Repository) ([]Result, error)) ([]Result, error) {
	results := make([][]Result, len(repositories))
	failures := make([]error, len(repositories))
	var group sync.WaitGroup
	for index, repository := range repositories {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index], failures[index] = lookup(repository)
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
