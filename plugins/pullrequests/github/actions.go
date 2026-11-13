package github

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

func (github *provider) Diff(pr pullrequests.PullRequest) (string, error) {
	output, err := shell.Run("gh", "pr", "diff", pr.URL)
	return string(output), err
}

// ChangedFiles lists the files the pull request touches that still exist on its branch.
func (github *provider) ChangedFiles(pr pullrequests.PullRequest) ([]string, error) {
	output, err := shell.Run("gh", "api", apiEndpoint(pr)+"/files?per_page=100")
	if err != nil {
		return nil, err
	}
	var files []struct {
		Filename string
		Status   string
	}
	if err := json.Unmarshal(output, &files); err != nil {
		return nil, err
	}
	var names []string
	for _, file := range files {
		if file.Status != "removed" {
			names = append(names, file.Filename)
		}
	}
	return names, nil
}

func (github *provider) Description(pr pullrequests.PullRequest) (string, error) {
	output, err := shell.Run("gh", "api", apiEndpoint(pr))
	if err != nil {
		return "", err
	}
	var response struct{ Body string }
	if err := json.Unmarshal(output, &response); err != nil {
		return "", err
	}
	return strings.ReplaceAll(response.Body, "\r\n", "\n"), nil
}

// sendJSON sends the payload as a JSON file so that gh passes it through untouched.
func sendJSON(method, endpoint string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp("", "prs-request-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(encoded); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = shell.Run("gh", "api", "--method", method, endpoint, "--input", file.Name())
	return err
}

func (github *provider) SaveDescription(pr pullrequests.PullRequest, body string) error {
	return sendJSON("PATCH", apiEndpoint(pr), map[string]string{"body": body})
}

func sideFrom(text string) pullrequests.CommentSide {
	if text == "LEFT" {
		return pullrequests.CommentSideOld
	}
	return pullrequests.CommentSideNew
}

func sideText(side pullrequests.CommentSide) string {
	if side == pullrequests.CommentSideOld {
		return "LEFT"
	}
	return "RIGHT"
}

func (github *provider) Comments(pr pullrequests.PullRequest) ([]pullrequests.ReviewComment, error) {
	output, err := shell.Run("gh", "api", apiEndpoint(pr)+"/comments?per_page=100")
	if err != nil {
		return nil, err
	}
	var response []struct {
		ID        int64
		InReplyTo int64 `json:"in_reply_to_id"`
		Path      string
		Line      int
		Side      string
		User      struct{ Login string }
		Body      string
		CreatedAt time.Time `json:"created_at"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}
	var comments []pullrequests.ReviewComment
	for _, comment := range response {
		comments = append(comments, pullrequests.ReviewComment{
			ID:        comment.ID,
			InReplyTo: comment.InReplyTo,
			Path:      comment.Path,
			Line:      comment.Line,
			Side:      sideFrom(comment.Side),
			Author:    comment.User.Login,
			Body:      comment.Body,
			CreatedAt: comment.CreatedAt,
		})
	}
	return comments, nil
}

func (github *provider) PostComment(pr pullrequests.PullRequest, target pullrequests.CommentTarget, body string) error {
	if target.ReplyTo != 0 {
		return sendJSON("POST", apiEndpoint(pr)+"/comments", map[string]any{"body": body, "in_reply_to": target.ReplyTo})
	}
	return sendJSON("POST", apiEndpoint(pr)+"/comments", map[string]any{
		"body":      body,
		"commit_id": pr.HeadCommit,
		"path":      target.Path,
		"line":      target.Line,
		"side":      sideText(target.Side),
	})
}

func (github *provider) SetDraft(pr pullrequests.PullRequest, draft bool) error {
	command := []string{"gh", "pr", "ready", pr.URL}
	if draft {
		command = append(command, "--undo")
	}
	_, err := shell.Run(command...)
	return err
}

func (github *provider) SetAutoMerge(pr pullrequests.PullRequest, enabled bool) error {
	command := []string{"gh", "pr", "merge", pr.URL, "--disable-auto"}
	if enabled {
		command = []string{"gh", "pr", "merge", pr.URL, "--auto", "--squash"}
	}
	_, err := shell.Run(command...)
	return err
}

func (github *provider) RequestTeamReview(pr pullrequests.PullRequest) error {
	endpoint := fmt.Sprintf("repos/%s/pulls/%d/requested_reviewers", pr.Repository.FullName(), pr.Number)
	_, err := shell.Run("gh", "api", "--method", "POST", endpoint, "-f", "team_reviewers[]="+github.reviewTeam)
	return err
}
