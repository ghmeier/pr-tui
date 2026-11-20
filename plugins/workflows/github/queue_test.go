package github

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/plugins/workflows"
)

const queuedPullRequests = `{"data": {"repository": {"mergeQueue": {"entries": {"nodes": [
  {"position": 1, "state": "MERGEABLE", "enqueuedAt": "2026-11-10T12:00:00Z", "estimatedTimeToMerge": 480,
   "pullRequest": {"number": 501, "title": "Cache the cart", "url": "https://github.com/acme/storefront/pull/501", "author": {"login": "first"}},
   "headCommit": {"statusCheckRollup": {"state": "SUCCESS"}}},
  {"position": 2, "state": "AWAITING_CHECKS", "enqueuedAt": "2026-11-10T12:05:00Z", "estimatedTimeToMerge": null,
   "pullRequest": {"number": 502, "title": "Resize the banner", "url": "https://github.com/acme/storefront/pull/502", "author": null},
   "headCommit": {"statusCheckRollup": {"state": "PENDING"}}},
  {"position": 3, "state": "LOCKED", "enqueuedAt": "2026-11-10T12:06:00Z", "estimatedTimeToMerge": null,
   "pullRequest": {"number": 503, "title": "Drop the old route", "url": "https://github.com/acme/storefront/pull/503", "author": {"login": "third"}},
   "headCommit": null}
]}}}}}`

func TestMergeQueueAsksForTheQueueWithTheOwnerAndNameAsVariables(t *testing.T) {
	stub := stubShell(t, queuedPullRequests)

	_, err := newProvider().MergeQueue(storefront)

	if err != nil {
		t.Fatal(err)
	}
	stub.assertCommands(t, []string{"gh", "api", "graphql", "-f", "query=" + mergeQueueQuery, "-f", "owner=acme", "-f", "name=storefront"})
	if strings.Contains(mergeQueueQuery, "acme") {
		t.Fatalf("query = %s", mergeQueueQuery)
	}
}

func TestMergeQueueParsesEveryFieldTheBoardUses(t *testing.T) {
	stubShell(t, queuedPullRequests)

	entries, err := newProvider().MergeQueue(storefront)

	if err != nil || len(entries) != 3 {
		t.Fatalf("entries = %+v, error = %v", entries, err)
	}
	expected := workflows.QueueEntry{
		Repository: storefront, Position: 1, State: workflows.QueueStateMergeable, Checks: workflows.RunStatusSucceeded,
		Number: 501, Title: "Cache the cart", URL: "https://github.com/acme/storefront/pull/501", Author: "first",
		EnqueuedAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC), EstimatedTimeToMerge: 8 * time.Minute,
	}
	if !reflect.DeepEqual(entries[0], expected) {
		t.Fatalf("entry = %+v, expected %+v", entries[0], expected)
	}
}

func TestMergeQueueNamesAnUnknownAuthorGhostAndLeavesAMissingEstimateZero(t *testing.T) {
	stubShell(t, queuedPullRequests)

	entries, _ := newProvider().MergeQueue(storefront)

	if entries[1].Author != "ghost" || entries[1].EstimatedTimeToMerge != 0 {
		t.Fatalf("entry = %+v", entries[1])
	}
	if entries[0].Author != "first" || entries[0].EstimatedTimeToMerge == 0 {
		t.Fatalf("entry = %+v", entries[0])
	}
}

func TestMergeQueueStatesMapToTheNeutralStates(t *testing.T) {
	for text, expected := range map[string]workflows.QueueState{
		"QUEUED":          workflows.QueueStateQueued,
		"AWAITING_CHECKS": workflows.QueueStateAwaitingChecks,
		"MERGEABLE":       workflows.QueueStateMergeable,
		"UNMERGEABLE":     workflows.QueueStateUnmergeable,
		"LOCKED":          workflows.QueueStateLocked,
	} {
		if state := queueStateFrom(text); state != expected {
			t.Errorf("%s = %v, expected %v", text, state, expected)
		}
	}
}

func TestMergeQueueCheckStatesMapLikeThePullRequestCheckStates(t *testing.T) {
	for text, expected := range map[string]workflows.RunStatus{
		"SUCCESS":  workflows.RunStatusSucceeded,
		"FAILURE":  workflows.RunStatusFailed,
		"ERROR":    workflows.RunStatusFailed,
		"PENDING":  workflows.RunStatusRunning,
		"EXPECTED": workflows.RunStatusRunning,
	} {
		if status := checksStatusFrom(text); status != expected {
			t.Errorf("%s = %v, expected %v", text, status, expected)
		}
	}
}

func TestMergeQueueChecksAreQueuedWhenTheCommitHasNoRollup(t *testing.T) {
	stubShell(t, queuedPullRequests)

	entries, _ := newProvider().MergeQueue(storefront)

	if entries[2].Checks != workflows.RunStatusQueued {
		t.Fatalf("checks = %v", entries[2].Checks)
	}
	if entries[0].Checks == workflows.RunStatusQueued {
		t.Fatalf("checks = %v", entries[0].Checks)
	}
}

func TestMergeQueueOfAnEmptyQueueIsEmpty(t *testing.T) {
	stubShell(t, `{"data": {"repository": {"mergeQueue": {"entries": {"nodes": []}}}}}`)

	entries, err := newProvider().MergeQueue(storefront)

	if err != nil || len(entries) != 0 {
		t.Fatalf("entries = %+v, error = %v", entries, err)
	}
}

func TestMergeQueueOfARepositoryWithoutOneIsEmptyAndNotAnError(t *testing.T) {
	stubShell(t, `{"data": {"repository": {"mergeQueue": null}}}`)

	entries, err := newProvider().MergeQueue(storefront)

	if err != nil || len(entries) != 0 {
		t.Fatalf("entries = %+v, error = %v", entries, err)
	}
}

func TestMergeQueueReturnsTheErrorOfGh(t *testing.T) {
	stubShellWith(t, func([]string) ([]byte, error) { return nil, errorFromGh })

	_, err := newProvider().MergeQueue(storefront)

	if err != errorFromGh {
		t.Fatalf("error = %v", err)
	}
}
