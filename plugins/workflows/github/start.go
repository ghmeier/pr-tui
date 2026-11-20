package github

import (
	"strconv"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/workflows"
)

// StartWorkflow passes the inputs in the order the workflow declares them. An input left empty
// is not passed unless it is required, so that the workflow applies its own default.
func (*provider) StartWorkflow(workflow workflows.Workflow, branch string, values map[string]string) error {
	arguments := []string{"gh", "workflow", "run", workflow.File, "-R", workflow.Repository.FullName(), "--ref", branch}
	for _, input := range workflow.Inputs {
		value := values[input.Name]
		if value == "" && !input.Required {
			continue
		}
		arguments = append(arguments, "-f", input.Name+"="+value)
	}
	_, err := shell.Run(arguments...)
	return err
}

func (*provider) Rerun(run workflows.Run, failedOnly bool) error {
	arguments := []string{"gh", "run", "rerun", strconv.FormatInt(run.ID, 10), "-R", run.Repository.FullName()}
	if failedOnly {
		arguments = append(arguments, "--failed")
	}
	_, err := shell.Run(arguments...)
	return err
}
