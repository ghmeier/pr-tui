package github

import (
	"testing"

	"github.com/ghmeier/pr-tui/plugins/workflows"
)

var deployWorkflow = workflows.Workflow{
	Repository: storefront, Name: "Deploy CI", File: "deploy.yaml",
	Inputs: []workflows.WorkflowInput{
		{Name: "environment", Required: true, Kind: workflows.InputKindChoice, Options: []string{"staging", "production"}},
		{Name: "size", Kind: workflows.InputKindChoice, Options: []string{"S", "M"}},
		{Name: "command", Kind: workflows.InputKindText},
	},
}

func TestStartWorkflowPassesTheBranchAndTheInputsInDeclaredOrder(t *testing.T) {
	stub := stubShell(t, "")

	err := newProvider().StartWorkflow(deployWorkflow, "release", map[string]string{"command": "make all", "size": "M", "environment": "staging"})

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{
		"gh", "workflow", "run", "deploy.yaml", "-R", "acme/storefront", "--ref", "release",
		"-f", "environment=staging", "-f", "size=M", "-f", "command=make all",
	})
}

func TestStartWorkflowSkipsEmptyInputsUnlessTheyAreRequired(t *testing.T) {
	stub := stubShell(t, "")

	newProvider().StartWorkflow(deployWorkflow, "main", map[string]string{"size": "", "command": "ls"})

	stub.assertCommands(t, []string{
		"gh", "workflow", "run", "deploy.yaml", "-R", "acme/storefront", "--ref", "main",
		"-f", "environment=", "-f", "command=ls",
	})
}

func TestStartWorkflowReturnsTheFailureOfGh(t *testing.T) {
	stubShellWith(t, func([]string) ([]byte, error) { return nil, errorFromGh })

	err := newProvider().StartWorkflow(deployWorkflow, "main", nil)

	if err != errorFromGh {
		t.Fatalf("error = %v", err)
	}
}

func TestRerunRunsAllJobsOfTheRunByItsID(t *testing.T) {
	stub := stubShell(t, "")

	err := newProvider().Rerun(workflows.Run{Repository: storefront, ID: 4417}, false)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{"gh", "run", "rerun", "4417", "-R", "acme/storefront"})
}

func TestRerunRunsOnlyTheFailedJobsWhenAsked(t *testing.T) {
	stub := stubShell(t, "")

	newProvider().Rerun(workflows.Run{Repository: storefront, ID: 4417}, true)

	stub.assertCommands(t, []string{"gh", "run", "rerun", "4417", "-R", "acme/storefront", "--failed"})
}
