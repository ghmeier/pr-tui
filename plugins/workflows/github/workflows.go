package github

import (
	"encoding/json"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/workflows"
)

// workflowFilesQuery fetches the text of every file in the workflows directory in one call.
// The owner and name are variables, so they are never written into the query text.
const workflowFilesQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    defaultBranchRef { name }
    object(expression: "HEAD:.github/workflows") {
      ... on Tree { entries { name object { ... on Blob { text } } } }
    }
  }
}`

// Workflows lists the repository's active workflows. The answer is kept, because workflow
// files rarely change, and each caller gets its own copy, because callers filter what they get
// in place.
func (github *provider) Workflows(repository workflows.Repository) ([]workflows.Workflow, error) {
	github.mutex.Lock()
	known, cached := github.workflowsByRepository[repository]
	github.mutex.Unlock()
	if cached {
		return slices.Clone(known), nil
	}
	listed, err := fetchWorkflows(repository)
	if err != nil {
		return nil, err
	}
	github.mutex.Lock()
	github.workflowsByRepository[repository] = listed
	github.mutex.Unlock()
	return slices.Clone(listed), nil
}

func fetchWorkflows(repository workflows.Repository) ([]workflows.Workflow, error) {
	listing, err := shell.Run("gh", "api", fmt.Sprintf("repos/%s/actions/workflows?per_page=100", repository.FullName()))
	if err != nil {
		return nil, err
	}
	var listed struct {
		Workflows []struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			URL   string `json:"html_url"`
			State string `json:"state"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal(listing, &listed); err != nil {
		return nil, err
	}
	if len(listed.Workflows) == 0 {
		return nil, nil
	}
	texts, defaultBranch, err := fetchWorkflowTexts(repository)
	if err != nil {
		return nil, err
	}
	var result []workflows.Workflow
	for _, entry := range listed.Workflows {
		if entry.State != "active" {
			continue
		}
		file := baseName(entry.Path)
		triggers, schedules := parseTriggers(texts[file])
		result = append(result, workflows.Workflow{
			Repository: repository, Name: entry.Name, File: file, URL: entry.URL, Triggers: triggers, Schedules: schedules,
			Inputs: parseInputs(texts[file]), DefaultBranch: defaultBranch,
		})
	}
	return result, nil
}

// fetchWorkflowTexts returns the text of each file of the workflows directory by its name, and
// the repository's default branch. Files that are not text, and a repository with no such
// directory, are left out.
func fetchWorkflowTexts(repository workflows.Repository) (map[string]string, string, error) {
	output, err := shell.Run("gh", "api", "graphql",
		"-f", "query="+workflowFilesQuery, "-f", "owner="+repository.Owner, "-f", "name="+repository.Name)
	if err != nil {
		return nil, "", err
	}
	var response struct {
		Data struct {
			Repository struct {
				DefaultBranchRef struct {
					Name string `json:"name"`
				} `json:"defaultBranchRef"`
				Object struct {
					Entries []struct {
						Name   string `json:"name"`
						Object struct {
							Text string `json:"text"`
						} `json:"object"`
					} `json:"entries"`
				} `json:"object"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, "", err
	}
	texts := map[string]string{}
	for _, entry := range response.Data.Repository.Object.Entries {
		texts[entry.Name] = entry.Object.Text
	}
	return texts, response.Data.Repository.DefaultBranchRef.Name, nil
}

// parseTriggers reads what the "on" key of a workflow file says: one event, a list of events,
// or a table of events, one of which may be the schedule with its cron expressions. A file
// with no "on" key, or one that cannot be read, has none; the workflow still belongs on the board.
func parseTriggers(text string) (triggers []workflows.Trigger, schedules []string) {
	on := onNode(text)
	var events []string
	switch on.Kind {
	case yaml.ScalarNode:
		events = []string{on.Value}
	case yaml.SequenceNode:
		for _, event := range on.Content {
			events = append(events, event.Value)
		}
	case yaml.MappingNode:
		for index := 0; index+1 < len(on.Content); index += 2 {
			event, definition := on.Content[index].Value, on.Content[index+1]
			events = append(events, event)
			if event == "schedule" {
				schedules = cronExpressions(definition)
			}
		}
	}
	for _, event := range events {
		trigger := triggerOfEvent(event)
		if !slices.Contains(triggers, trigger) {
			triggers = append(triggers, trigger)
		}
	}
	return triggers, schedules
}

// onNode is the value of a workflow file's "on" key, or an empty node when the file has none or
// cannot be read.
func onNode(text string) yaml.Node {
	var file struct {
		On yaml.Node `yaml:"on"`
	}
	if err := yaml.Unmarshal([]byte(text), &file); err != nil {
		return yaml.Node{}
	}
	return file.On
}

func cronExpressions(schedule *yaml.Node) []string {
	var expressions []string
	for _, entry := range schedule.Content {
		for index := 0; index+1 < len(entry.Content); index += 2 {
			if entry.Content[index].Value == "cron" {
				expressions = append(expressions, entry.Content[index+1].Value)
			}
		}
	}
	return expressions
}
