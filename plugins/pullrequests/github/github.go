// Package github is the pull request provider for GitHub. It is the only place that runs gh
// or knows the shape of GitHub's GraphQL and REST responses.
package github

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

func init() {
	pullrequests.RegisterProvider("github", newProvider)
}

// repositoryPartPattern is deliberately narrow: the owner and name are written into a
// GraphQL query, so anything beyond what GitHub allows in them is rejected here.
var repositoryPartPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type provider struct {
	repositories []pullrequests.Repository
	reviewTeam   string
}

func parseRepository(text string) (pullrequests.Repository, bool) {
	owner, name, found := strings.Cut(text, "/")
	if !found || !repositoryPartPattern.MatchString(owner) || !repositoryPartPattern.MatchString(name) {
		return pullrequests.Repository{}, false
	}
	return pullrequests.Repository{Owner: owner, Name: name}, true
}

// newProvider reads the [github] table and checks it. Every error names the key.
func newProvider(decodeSection pullrequests.SectionDecoder) (pullrequests.Provider, error) {
	var section struct {
		Repositories []string `toml:"repositories"`
		ReviewTeam   string   `toml:"review_team"`
	}
	if err := decodeSection("github", &section); err != nil {
		return nil, err
	}
	if len(section.Repositories) == 0 {
		return nil, errors.New(`[github] repositories is required; list each one as "owner/name"`)
	}
	result := &provider{reviewTeam: section.ReviewTeam}
	for _, text := range section.Repositories {
		parsed, valid := parseRepository(text)
		if !valid {
			return nil, fmt.Errorf(`[github] repositories entry %q must be written as "owner/name"`, text)
		}
		if slices.Contains(result.repositories, parsed) {
			return nil, fmt.Errorf("[github] repositories lists %q more than once", text)
		}
		result.repositories = append(result.repositories, parsed)
	}
	return result, nil
}

func (github *provider) Repositories() []pullrequests.Repository {
	return github.repositories
}

func (github *provider) ReviewTeam() string {
	return github.reviewTeam
}

func apiEndpoint(pr pullrequests.PullRequest) string {
	return fmt.Sprintf("repos/%s/pulls/%d", pr.Repository.FullName(), pr.Number)
}
