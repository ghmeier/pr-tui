package github

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ghmeier/pr-tui/plugins/workflows"
)

const workflowsEndpoint = "repos/acme/storefront/actions/workflows?per_page=100"

const listedWorkflows = `{"workflows": [
  {"name": "Deploy CI", "path": ".github/workflows/deploy.yaml", "html_url": "https://github.com/acme/storefront/actions/workflows/deploy.yaml", "state": "active"},
  {"name": "Checks", "path": ".github/workflows/checks.yml", "html_url": "https://github.com/acme/storefront/actions/workflows/checks.yml", "state": "active"},
  {"name": "Nightly", "path": ".github/workflows/nightly.yaml", "html_url": "https://github.com/acme/storefront/actions/workflows/nightly.yaml", "state": "active"},
  {"name": "Retired", "path": ".github/workflows/retired.yaml", "html_url": "https://github.com/acme/storefront/actions/workflows/retired.yaml", "state": "disabled_manually"}
]}`

// workflowFiles is what the GraphQL query answers: the text of each file of the directory, and
// a file that the workflows endpoint does not list.
const workflowFiles = `{"data": {"repository": {"object": {"entries": [
  {"name": "actions.lock", "object": {"text": "not: [a workflow"}},
  {"name": "checks.yml", "object": {"text": "name: Checks\non: pull_request\n"}},
  {"name": "deploy.yaml", "object": {"text": "name: Deploy CI\non:\n  - push\n  - workflow_dispatch\n"}},
  {"name": "nightly.yaml", "object": {"text": "name: Nightly\non:\n  schedule:\n    - cron: '0 6 * * *'\n    - cron: '30 18 * * 1-5'\n  workflow_dispatch:\n"}},
  {"name": "retired.yaml", "object": {"text": "on: push\n"}}
]}}}}`

// answerWorkflowCalls answers the two calls that listing a repository's workflows makes.
func answerWorkflowCalls(arguments []string) ([]byte, error) {
	if arguments[2] == "graphql" {
		return []byte(workflowFiles), nil
	}
	return []byte(listedWorkflows), nil
}

func TestWorkflowsListsTheEndpointAndThenFetchesTheFilesWithTheOwnerAndNameAsVariables(t *testing.T) {
	stub := stubShellWith(t, answerWorkflowCalls)

	_, err := newProvider().Workflows(storefront)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t,
		apiCommand(workflowsEndpoint),
		[]string{"gh", "api", "graphql", "-f", "query=" + workflowFilesQuery, "-f", "owner=acme", "-f", "name=storefront"},
	)
	if strings.Contains(workflowFilesQuery, "acme") || !strings.Contains(workflowFilesQuery, "HEAD:.github/workflows") {
		t.Fatalf("query = %s", workflowFilesQuery)
	}
}

func TestWorkflowsJoinsEachActiveWorkflowWithTheTriggersOfItsFile(t *testing.T) {
	stubShellWith(t, answerWorkflowCalls)

	found, err := newProvider().Workflows(storefront)

	expected := []workflows.Workflow{
		{
			Repository: storefront, Name: "Deploy CI", File: "deploy.yaml", URL: "https://github.com/acme/storefront/actions/workflows/deploy.yaml",
			Triggers: []workflows.Trigger{workflows.TriggerPush, workflows.TriggerManual},
		},
		{
			Repository: storefront, Name: "Checks", File: "checks.yml", URL: "https://github.com/acme/storefront/actions/workflows/checks.yml",
			Triggers: []workflows.Trigger{workflows.TriggerPullRequest},
		},
		{
			Repository: storefront, Name: "Nightly", File: "nightly.yaml", URL: "https://github.com/acme/storefront/actions/workflows/nightly.yaml",
			Triggers:  []workflows.Trigger{workflows.TriggerSchedule, workflows.TriggerManual},
			Schedules: []string{"0 6 * * *", "30 18 * * 1-5"},
		},
	}
	if err != nil || !reflect.DeepEqual(found, expected) {
		t.Fatalf("workflows = %+v, error = %v", found, err)
	}
}

func TestWorkflowsSkipsFilesTheEndpointDoesNotListAndWorkflowsThatAreNotActive(t *testing.T) {
	stubShellWith(t, answerWorkflowCalls)

	found, _ := newProvider().Workflows(storefront)

	for _, workflow := range found {
		if workflow.File == "actions.lock" || workflow.File == "retired.yaml" {
			t.Errorf("listed %s", workflow.File)
		}
	}
}

func TestTheOnKeyMayBeAStringAListOrATable(t *testing.T) {
	cases := map[string]struct {
		text      string
		triggers  []workflows.Trigger
		schedules []string
	}{
		"a string":                {"on: push\n", []workflows.Trigger{workflows.TriggerPush}, nil},
		"a list":                  {"on: [push, pull_request]\n", []workflows.Trigger{workflows.TriggerPush, workflows.TriggerPullRequest}, nil},
		"a table":                 {"on:\n  push:\n    branches: [main]\n  merge_group:\n", []workflows.Trigger{workflows.TriggerPush, workflows.TriggerMergeQueue}, nil},
		"a table with a schedule": {"on:\n  schedule:\n    - cron: '5 4 * * *'\n", []workflows.Trigger{workflows.TriggerSchedule}, []string{"5 4 * * *"}},
	}

	for name, example := range cases {
		triggers, schedules := parseTriggers(example.text)

		if !reflect.DeepEqual(triggers, example.triggers) || !reflect.DeepEqual(schedules, example.schedules) {
			t.Errorf("%s: triggers = %v, schedules = %v", name, triggers, schedules)
		}
	}
}

func TestTheSameTriggerListedByTwoEventsIsKeptOnce(t *testing.T) {
	triggers, _ := parseTriggers("on: [pull_request, pull_request_target, release, issues]\n")

	expected := []workflows.Trigger{workflows.TriggerPullRequest, workflows.TriggerOther}
	if !reflect.DeepEqual(triggers, expected) {
		t.Fatalf("triggers = %v, expected %v", triggers, expected)
	}
}

func TestAFileWithoutAnOnKeyOrThatIsNotYAMLHasNoTriggers(t *testing.T) {
	for _, text := range []string{"", "name: Nothing\n", "on: [unclosed\n"} {
		triggers, schedules := parseTriggers(text)

		if len(triggers) != 0 || len(schedules) != 0 {
			t.Errorf("%q: triggers = %v, schedules = %v", text, triggers, schedules)
		}
	}
}

func TestWorkflowsKeepsAWorkflowWhoseFileIsMissingFromTheDirectoryWithoutTriggers(t *testing.T) {
	stubShellWith(t, func(arguments []string) ([]byte, error) {
		if arguments[2] == "graphql" {
			return []byte(`{"data": {"repository": {"object": null}}}`), nil
		}
		return []byte(listedWorkflows), nil
	})

	found, err := newProvider().Workflows(storefront)

	if err != nil || len(found) != 3 || len(found[0].Triggers) != 0 {
		t.Fatalf("workflows = %+v, error = %v", found, err)
	}
}

func TestWorkflowsOfARepositoryWithoutAnyAreNotFollowedByAFileRequest(t *testing.T) {
	stub := stubShell(t, `{"workflows": []}`)

	found, err := newProvider().Workflows(storefront)

	if err != nil || len(found) != 0 {
		t.Fatalf("workflows = %+v, error = %v", found, err)
	}
	stub.assertCommands(t, apiCommand(workflowsEndpoint))
}

func TestWorkflowsAreFetchedOncePerRepositoryForTheLifeOfTheProvider(t *testing.T) {
	stub := stubShellWith(t, answerWorkflowCalls)
	provider := newProvider()

	first, _ := provider.Workflows(storefront)
	second, _ := provider.Workflows(storefront)

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("first = %+v, second = %+v", first, second)
	}
	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	if len(stub.commands) != 2 {
		t.Fatalf("commands = %v", stub.commands)
	}
}

func TestWorkflowsOfDifferentRepositoriesAreCachedSeparately(t *testing.T) {
	stub := stubShellWith(t, answerWorkflowCalls)
	provider := newProvider()

	provider.Workflows(storefront)
	provider.Workflows(billing)

	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	if len(stub.commands) != 4 || stub.commands[2][2] != "repos/acme/billing/actions/workflows?per_page=100" {
		t.Fatalf("commands = %v", stub.commands)
	}
}

func TestAFailedListingIsNotCached(t *testing.T) {
	failing := true
	stubShellWith(t, func(arguments []string) ([]byte, error) {
		if failing {
			return nil, errorFromGh
		}
		return answerWorkflowCalls(arguments)
	})
	provider := newProvider()

	_, failure := provider.Workflows(storefront)
	failing = false
	found, err := provider.Workflows(storefront)

	if failure != errorFromGh || err != nil || len(found) != 3 {
		t.Fatalf("failure = %v, workflows = %+v, error = %v", failure, found, err)
	}
}

func TestWorkflowsCanBeLoadedForSeveralRepositoriesAtTheSameTime(t *testing.T) {
	stubShellWith(t, answerWorkflowCalls)
	provider := newProvider()

	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			provider.Workflows(storefront)
			provider.Workflows(billing)
		}()
	}
	group.Wait()
}
