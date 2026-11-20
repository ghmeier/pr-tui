package github

import (
	"reflect"
	"testing"

	"github.com/ghmeier/pr-tui/plugins/workflows"
)

const dispatchWithEveryKind = `name: Run Command
on:
  workflow_dispatch:
    inputs:
      environment:
        description: Where to run
        type: choice
        required: true
        default: staging
        options:
          - staging
          - production
      size:
        type: choice
        options: [S, M, L]
      command:
        description: What to run
        type: string
      verbose:
        type: boolean
        default: true
      replicas:
        type: number
        default: 3
      target:
        type: environment
      untyped:
        description: Has no type
`

func TestParseInputsKeepsDeclarationOrderAndReadsEveryKind(t *testing.T) {
	inputs := parseInputs(dispatchWithEveryKind)

	expected := []workflows.WorkflowInput{
		{Name: "environment", Description: "Where to run", Required: true, Default: "staging", Kind: workflows.InputKindChoice, Options: []string{"staging", "production"}},
		{Name: "size", Kind: workflows.InputKindChoice, Options: []string{"S", "M", "L"}},
		{Name: "command", Description: "What to run", Kind: workflows.InputKindText},
		{Name: "verbose", Default: "true", Kind: workflows.InputKindBoolean},
		{Name: "replicas", Default: "3", Kind: workflows.InputKindNumber},
		{Name: "target", Kind: workflows.InputKindEnvironment},
		{Name: "untyped", Description: "Has no type", Kind: workflows.InputKindText},
	}
	if !reflect.DeepEqual(inputs, expected) {
		t.Fatalf("inputs = %+v", inputs)
	}
}

func TestParseInputsKeepsTheTextOfStringBooleanAndNumberDefaults(t *testing.T) {
	inputs := parseInputs("on:\n  workflow_dispatch:\n    inputs:\n      a: {default: 'text'}\n      b: {default: false}\n      c: {default: 2.50}\n      d: {default: }\n")

	var defaults []string
	for _, input := range inputs {
		defaults = append(defaults, input.Default)
	}
	if !reflect.DeepEqual(defaults, []string{"text", "false", "2.50", ""}) {
		t.Fatalf("defaults = %q", defaults)
	}
}

func TestParseInputsOfAWorkflowWithoutInputsIsEmpty(t *testing.T) {
	cases := map[string]string{
		"a string":            "on: workflow_dispatch\n",
		"a list":              "on: [push, workflow_dispatch]\n",
		"a table without any": "on:\n  workflow_dispatch:\n",
		"no manual trigger":   "on:\n  push:\n",
		"no on key":           "name: Nothing\n",
		"unreadable":          "on: [unclosed\n",
	}

	for name, text := range cases {
		if inputs := parseInputs(text); len(inputs) != 0 {
			t.Errorf("%s: inputs = %+v", name, inputs)
		}
	}
}
