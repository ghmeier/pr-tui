package github

import (
	"gopkg.in/yaml.v3"

	"github.com/ghmeier/pr-tui/plugins/workflows"
)

// parseInputs reads the inputs a workflow asks for on a manual run, in the order the file
// declares them. A workflow that names the event without a table, or that cannot be read,
// has none.
func parseInputs(text string) []workflows.WorkflowInput {
	on := onNode(text)
	inputs := valueOfKey(valueOfKey(&on, "workflow_dispatch"), "inputs")
	if inputs == nil || inputs.Kind != yaml.MappingNode {
		return nil
	}
	var parsed []workflows.WorkflowInput
	for index := 0; index+1 < len(inputs.Content); index += 2 {
		definition := inputs.Content[index+1]
		input := workflows.WorkflowInput{
			Name:        inputs.Content[index].Value,
			Description: scalarOfKey(definition, "description"),
			Required:    scalarOfKey(definition, "required") == "true",
			Default:     scalarOfKey(definition, "default"),
			Kind:        inputKindOf(scalarOfKey(definition, "type")),
		}
		if options := valueOfKey(definition, "options"); options != nil && options.Kind == yaml.SequenceNode {
			for _, option := range options.Content {
				input.Options = append(input.Options, option.Value)
			}
		}
		parsed = append(parsed, input)
	}
	return parsed
}

func inputKindOf(declared string) workflows.InputKind {
	switch declared {
	case "choice":
		return workflows.InputKindChoice
	case "boolean":
		return workflows.InputKindBoolean
	case "number":
		return workflows.InputKindNumber
	case "environment":
		return workflows.InputKindEnvironment
	}
	return workflows.InputKindText
}

// valueOfKey is what a table maps the key to, or nil when the node is not a table or lacks it.
func valueOfKey(table *yaml.Node, key string) *yaml.Node {
	if table == nil || table.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(table.Content); index += 2 {
		if table.Content[index].Value == key {
			return table.Content[index+1]
		}
	}
	return nil
}

// scalarOfKey is the text of a string, boolean, or number the table maps the key to, as it is
// written in the file. A missing key and an empty value are both empty.
func scalarOfKey(table *yaml.Node, key string) string {
	value := valueOfKey(table, key)
	if value == nil || value.Kind != yaml.ScalarNode || value.Tag == "!!null" {
		return ""
	}
	return value.Value
}
