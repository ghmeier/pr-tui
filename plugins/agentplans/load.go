package agentplans

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/shell"
)

// listing is what "apl ls --json" prints.
type listing struct {
	Files []struct {
		File string `json:"file"`
		Type string `json:"type"`
		Meta struct {
			Title   string   `json:"title"`
			Status  string   `json:"status"`
			Tags    []string `json:"tags"`
			Created string   `json:"created"`
			Updated string   `json:"updated"`
		} `json:"meta"`
		Links []struct {
			Branch string `json:"branch"`
		} `json:"links"`
	} `json:"files"`
}

// load runs on a background goroutine. apl acts on its working directory, so it is started in
// each checkout.
func (settings settings) load() ([]plugin.Row, error) {
	var rows []Row
	for _, checkout := range settings.checkouts {
		found, err := checkout.rows()
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("apl is not installed, so there are no plans to list")
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", checkout.name, err)
		}
		rows = append(rows, found...)
	}
	slices.SortFunc(rows, func(left, right Row) int {
		return cmp.Or(
			cmp.Compare(right.Doc.Updated, left.Doc.Updated),
			cmp.Compare(left.Repository, right.Repository),
			cmp.Compare(left.Doc.File, right.Doc.File),
		)
	})
	loaded := make([]plugin.Row, len(rows))
	for index, row := range rows {
		loaded[index] = row
	}
	return loaded, nil
}

func (checkout checkout) rows() ([]Row, error) {
	output, err := shell.RunIn(checkout.path, "apl", "ls", "--json")
	if err != nil {
		return nil, err
	}
	var parsed listing
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("cannot read the output of apl ls: %w", err)
	}
	rows := make([]Row, len(parsed.Files))
	for index, file := range parsed.Files {
		doc := Doc{
			File: file.File, Type: file.Type, Title: file.Meta.Title, Status: file.Meta.Status,
			Tags: file.Meta.Tags, Created: file.Meta.Created, Updated: file.Meta.Updated,
		}
		for _, link := range file.Links {
			doc.Branches = append(doc.Branches, link.Branch)
		}
		path := filepath.Join(checkout.path, ".apl", file.File)
		// A document that cannot be read still has a row, only without pull requests.
		if body, err := os.ReadFile(path); err == nil {
			doc.PullRequestNumbers = pullRequestNumbers(string(body))
		}
		rows[index] = Row{Repository: checkout.name, Path: path, Doc: doc}
	}
	return rows, nil
}

var (
	// A "#" starts a pull request number unless it belongs to a word, an HTML entity such as
	// &#123;, or a path.
	hashReference = regexp.MustCompile(`(?:^|[^\w&/])#(\d+)\b`)
	pullAddress   = regexp.MustCompile(`/pull/(\d+)\b`)
)

// pullRequestNumbers finds the pull requests a text mentions, each once, largest first.
func pullRequestNumbers(text string) []int {
	var numbers []int
	for _, expression := range []*regexp.Regexp{hashReference, pullAddress} {
		for _, match := range expression.FindAllStringSubmatch(text, -1) {
			number, err := strconv.Atoi(match[1])
			if err == nil && !slices.Contains(numbers, number) {
				numbers = append(numbers, number)
			}
		}
	}
	slices.SortFunc(numbers, func(left, right int) int { return right - left })
	return numbers
}
