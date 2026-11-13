package github

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

const pullRequestFields = `
fragment pullRequestFields on Repository {
  pullRequests(states: OPEN, first: 100, orderBy: {field: UPDATED_AT, direction: DESC}) {
    nodes {
      number
      title
      url
      isDraft
      headRefOid
      autoMergeRequest { enabledAt }
      isInMergeQueue
      updatedAt
      reviewDecision
      author { login __typename }
      reviewRequests(first: 20) {
        nodes {
          requestedReviewer {
            __typename
            ... on User { login }
            ... on Team { slug }
          }
        }
      }
    }
  }
}
`

const lineCountsQuery = `
query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, first: 100) { nodes { url additions deletions } }
  }
}
`

const ciStatesQuery = `
query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, first: 100) {
      nodes { url commits(last: 1) { nodes { commit { statusCheckRollup { state } } } } }
    }
  }
}
`

type pullRequestNode struct {
	Number           int
	Title            string
	URL              string `json:"url"`
	IsDraft          bool
	HeadRefOid       string
	AutoMergeRequest *struct{ EnabledAt string }
	IsInMergeQueue   bool
	UpdatedAt        time.Time
	ReviewDecision   string
	Author           *struct {
		Login    string
		Typename string `json:"__typename"`
	}
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *struct {
				Typename string `json:"__typename"`
				Login    string
				Slug     string
			}
		}
	}
}

func reviewDecisionFrom(text string) pullrequests.ReviewDecision {
	switch text {
	case "APPROVED":
		return pullrequests.ReviewDecisionApproved
	case "CHANGES_REQUESTED":
		return pullrequests.ReviewDecisionChangesRequested
	}
	return pullrequests.ReviewDecisionNone
}

func ciStateFrom(text string) pullrequests.CIState {
	switch text {
	case "SUCCESS":
		return pullrequests.CIStatePassing
	case "FAILURE", "ERROR":
		return pullrequests.CIStateFailing
	case "PENDING", "EXPECTED":
		return pullrequests.CIStateRunning
	}
	return pullrequests.CIStateNone
}

func (node pullRequestNode) pullRequest(source pullrequests.Repository) pullrequests.PullRequest {
	result := pullrequests.PullRequest{
		Repository:     source,
		Number:         node.Number,
		Title:          node.Title,
		URL:            node.URL,
		Author:         "ghost",
		IsDraft:        node.IsDraft,
		HeadCommit:     node.HeadRefOid,
		AutoMerge:      node.AutoMergeRequest != nil,
		InMergeQueue:   node.IsInMergeQueue,
		ReviewDecision: reviewDecisionFrom(node.ReviewDecision),
		UpdatedAt:      node.UpdatedAt,
	}
	if node.Author != nil {
		result.Author = node.Author.Login
		result.IsBot = node.Author.Typename == "Bot"
	}
	for _, request := range node.ReviewRequests.Nodes {
		reviewer := request.RequestedReviewer
		if reviewer == nil {
			continue
		}
		if reviewer.Typename == "Team" {
			result.RequestedTeams = append(result.RequestedTeams, reviewer.Slug)
		} else if reviewer.Login != "" {
			result.RequestedUsers = append(result.RequestedUsers, reviewer.Login)
		}
	}
	return result
}

// PullRequests asks for every repository in one query. The owner is written into each
// alias, because the repositories may belong to different owners.
func (github *provider) PullRequests() (pullRequests []pullrequests.PullRequest, viewer string, err error) {
	var selections strings.Builder
	for index, source := range github.repositories {
		fmt.Fprintf(&selections, "repository%d: repository(owner: %q, name: %q) { ...pullRequestFields }\n", index, source.Owner, source.Name)
	}
	query := "query { viewer { login }\n" + selections.String() + "}\n" + pullRequestFields

	output, err := shell.Run("gh", "api", "graphql", "-f", "query="+query)
	if err != nil {
		return nil, "", err
	}
	var response struct {
		Data map[string]json.RawMessage
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, "", err
	}
	var viewerNode struct{ Login string }
	if err := json.Unmarshal(response.Data["viewer"], &viewerNode); err != nil {
		return nil, "", err
	}
	for index, source := range github.repositories {
		var repositoryNode struct {
			PullRequests struct{ Nodes []pullRequestNode }
		}
		if err := json.Unmarshal(response.Data[fmt.Sprintf("repository%d", index)], &repositoryNode); err != nil {
			return nil, "", err
		}
		for _, node := range repositoryNode.PullRequests.Nodes {
			pullRequests = append(pullRequests, node.pullRequest(source))
		}
	}
	sort.SliceStable(pullRequests, func(i, j int) bool {
		return pullRequests[i].UpdatedAt.After(pullRequests[j].UpdatedAt)
	})
	return pullRequests, viewerNode.Login, nil
}

// LineCounts and CIStates are separate from PullRequests because GitHub computes both
// slowly, and the board should not wait for them.
func (github *provider) LineCounts(source pullrequests.Repository) (map[string]pullrequests.LineCount, error) {
	output, err := shell.Run(
		"gh", "api", "graphql", "-f", "query="+lineCountsQuery, "-f", "owner="+source.Owner, "-f", "name="+source.Name,
	)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			Repository struct {
				PullRequests struct {
					Nodes []struct {
						URL       string `json:"url"`
						Additions int
						Deletions int
					}
				}
			}
		}
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}
	counts := map[string]pullrequests.LineCount{}
	for _, node := range response.Data.Repository.PullRequests.Nodes {
		counts[node.URL] = pullrequests.LineCount{Additions: node.Additions, Deletions: node.Deletions}
	}
	return counts, nil
}

// CIStates maps each open pull request's URL to its CI state, with none for no checks.
func (github *provider) CIStates(source pullrequests.Repository) (map[string]pullrequests.CIState, error) {
	output, err := shell.Run(
		"gh", "api", "graphql", "-f", "query="+ciStatesQuery, "-f", "owner="+source.Owner, "-f", "name="+source.Name,
	)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			Repository struct {
				PullRequests struct {
					Nodes []struct {
						URL     string `json:"url"`
						Commits struct {
							Nodes []struct {
								Commit struct {
									StatusCheckRollup *struct{ State string }
								}
							}
						}
					}
				}
			}
		}
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}
	states := map[string]pullrequests.CIState{}
	for _, node := range response.Data.Repository.PullRequests.Nodes {
		states[node.URL] = pullrequests.CIStateNone
		if len(node.Commits.Nodes) > 0 && node.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
			states[node.URL] = ciStateFrom(node.Commits.Nodes[0].Commit.StatusCheckRollup.State)
		}
	}
	return states, nil
}
