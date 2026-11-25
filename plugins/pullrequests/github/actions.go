package github

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sort"
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

func (github *provider) Description(pr pullrequests.PullRequest) (pullrequests.Description, error) {
	output, err := shell.Run("gh", "api", apiEndpoint(pr))
	if err != nil {
		return pullrequests.Description{}, err
	}
	var response struct{ Title, Body string }
	if err := json.Unmarshal(output, &response); err != nil {
		return pullrequests.Description{}, err
	}
	return pullrequests.Description{Title: response.Title, Body: strings.ReplaceAll(response.Body, "\r\n", "\n")}, nil
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

func (github *provider) SaveDescription(pr pullrequests.PullRequest, description pullrequests.Description) error {
	return sendJSON("PATCH", apiEndpoint(pr), map[string]string{"title": description.Title, "body": description.Body})
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

// listEach fetches every page of a list endpoint. It asks for --jq '.[]' because the pages of a
// plain --paginate are several JSON arrays back to back, where this prints one value for each element.
func listEach[Element any](endpoint string) ([]Element, error) {
	output, err := shell.Run("gh", "api", "--paginate", endpoint, "--jq", ".[]")
	if err != nil {
		return nil, err
	}
	var elements []Element
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var element Element
		err := decoder.Decode(&element)
		if err == io.EOF {
			return elements, nil
		}
		if err != nil {
			return nil, err
		}
		elements = append(elements, element)
	}
}

func (github *provider) Comments(pr pullrequests.PullRequest) ([]pullrequests.ReviewComment, error) {
	response, err := listEach[struct {
		ID          int64
		InReplyTo   int64 `json:"in_reply_to_id"`
		Path        string
		Line        int
		Side        string
		SubjectType string `json:"subject_type"`
		User        struct{ Login string }
		Body        string
		CreatedAt   time.Time `json:"created_at"`
	}](apiEndpoint(pr) + "/comments?per_page=100")
	if err != nil {
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
			WholeFile: comment.SubjectType == "file",
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
	fields := map[string]any{"body": body, "commit_id": pr.HeadCommit, "path": target.Path}
	if target.WholeFile {
		fields["subject_type"] = "file"
	} else {
		fields["line"] = target.Line
		fields["side"] = sideText(target.Side)
	}
	return sendJSON("POST", apiEndpoint(pr)+"/comments", fields)
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

func (github *provider) Merge(pr pullrequests.PullRequest) error {
	_, err := shell.Run("gh", "pr", "merge", pr.URL, "--squash")
	return err
}

func reviewEventText(event pullrequests.ReviewEvent) string {
	switch event {
	case pullrequests.ReviewEventRequestChanges:
		return "REQUEST_CHANGES"
	case pullrequests.ReviewEventComment:
		return "COMMENT"
	}
	return "APPROVE"
}

func (github *provider) SubmitReview(pr pullrequests.PullRequest, review pullrequests.Review) error {
	// The commit is left out so that GitHub reviews the latest one, whose line numbers are the
	// ones the diff screen shows.
	payload := map[string]any{"event": reviewEventText(review.Event)}
	if review.Body != "" {
		payload["body"] = review.Body
	}
	if len(review.Comments) > 0 {
		comments := make([]map[string]any, len(review.Comments))
		for index, comment := range review.Comments {
			comments[index] = map[string]any{
				"path": comment.Path, "line": comment.Line, "side": sideText(comment.Side), "body": comment.Body,
			}
		}
		payload["comments"] = comments
	}
	return sendJSON("POST", apiEndpoint(pr)+"/reviews", payload)
}

// RequestReview asks the team, whose slug is the reviewer's login, or the person.
func (github *provider) RequestReview(pr pullrequests.PullRequest, reviewer pullrequests.Reviewer) error {
	field := "reviewers[]="
	if reviewer.Team {
		field = "team_reviewers[]="
	}
	_, err := shell.Run("gh", "api", "--method", "POST", apiEndpoint(pr)+"/requested_reviewers", "-f", field+reviewer.Login)
	return err
}

// TeamMembers lists the review team's members in the repository's organization, sorted, or
// nothing when no team is configured.
func (github *provider) TeamMembers(repository pullrequests.Repository) ([]string, error) {
	if github.reviewTeam == "" {
		return nil, nil
	}
	output, err := shell.Run("gh", "api", "--paginate", "orgs/"+repository.Owner+"/teams/"+github.reviewTeam+"/members", "--jq", ".[].login")
	if err != nil {
		return nil, err
	}
	logins := strings.Fields(string(output))
	sort.Strings(logins)
	return logins, nil
}
