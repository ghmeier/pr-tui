package github

import (
	"encoding/json"
	"time"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/workflows"
)

// mergeQueueQuery fetches the queue of the default branch. The owner and name are variables, so
// they are never written into the query text.
const mergeQueueQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    mergeQueue {
      entries(first: 100) {
        nodes {
          position state enqueuedAt estimatedTimeToMerge
          pullRequest { number title url author { login } }
          headCommit { statusCheckRollup { state } }
        }
      }
    }
  }
}`

// MergeQueue lists the queue of the repository's default branch, front first. A repository
// without a merge queue answers null, which is an empty queue.
func (*provider) MergeQueue(repository workflows.Repository) ([]workflows.QueueEntry, error) {
	output, err := shell.Run("gh", "api", "graphql",
		"-f", "query="+mergeQueueQuery, "-f", "owner="+repository.Owner, "-f", "name="+repository.Name)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			Repository struct {
				MergeQueue *struct {
					Entries struct {
						Nodes []struct {
							Position             int       `json:"position"`
							State                string    `json:"state"`
							EnqueuedAt           time.Time `json:"enqueuedAt"`
							EstimatedTimeToMerge *int64    `json:"estimatedTimeToMerge"`
							PullRequest          struct {
								Number int    `json:"number"`
								Title  string `json:"title"`
								URL    string `json:"url"`
								Author *struct {
									Login string `json:"login"`
								} `json:"author"`
							} `json:"pullRequest"`
							HeadCommit *struct {
								StatusCheckRollup *struct {
									State string `json:"state"`
								} `json:"statusCheckRollup"`
							} `json:"headCommit"`
						} `json:"nodes"`
					} `json:"entries"`
				} `json:"mergeQueue"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}
	queue := response.Data.Repository.MergeQueue
	if queue == nil {
		return nil, nil
	}
	entries := make([]workflows.QueueEntry, 0, len(queue.Entries.Nodes))
	for _, node := range queue.Entries.Nodes {
		entry := workflows.QueueEntry{
			Repository: repository, Position: node.Position, State: queueStateFrom(node.State),
			Checks: workflows.RunStatusQueued, Number: node.PullRequest.Number, Title: node.PullRequest.Title,
			URL: node.PullRequest.URL, Author: "ghost", EnqueuedAt: node.EnqueuedAt,
		}
		if node.PullRequest.Author != nil {
			entry.Author = node.PullRequest.Author.Login
		}
		if node.EstimatedTimeToMerge != nil {
			entry.EstimatedTimeToMerge = time.Duration(*node.EstimatedTimeToMerge) * time.Second
		}
		if node.HeadCommit != nil && node.HeadCommit.StatusCheckRollup != nil {
			entry.Checks = checksStatusFrom(node.HeadCommit.StatusCheckRollup.State)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func queueStateFrom(text string) workflows.QueueState {
	switch text {
	case "AWAITING_CHECKS":
		return workflows.QueueStateAwaitingChecks
	case "MERGEABLE":
		return workflows.QueueStateMergeable
	case "UNMERGEABLE":
		return workflows.QueueStateUnmergeable
	case "LOCKED":
		return workflows.QueueStateLocked
	}
	return workflows.QueueStateQueued
}

func checksStatusFrom(text string) workflows.RunStatus {
	switch text {
	case "SUCCESS":
		return workflows.RunStatusSucceeded
	case "FAILURE", "ERROR":
		return workflows.RunStatusFailed
	case "PENDING", "EXPECTED":
		return workflows.RunStatusRunning
	}
	return workflows.RunStatusQueued
}
