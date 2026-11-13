package pullrequests

import (
	"fmt"
	"slices"
	"strings"
)

// Provider is where pull requests come from. Every method that takes a pull request or a
// repository is given the values the provider itself returned, and may block on the network.
type Provider interface {
	Repositories() []Repository
	// ReviewTeam is "" when no team is configured.
	ReviewTeam() string
	PullRequests() (pullRequests []PullRequest, viewer string, err error)
	// LineCounts and CIStates are separate from PullRequests because they are slow to compute
	// and the board should not wait for them. Both are keyed by pull request URL.
	LineCounts(repository Repository) (map[string]LineCount, error)
	CIStates(repository Repository) (map[string]CIState, error)
	// Diff is the pull request's changes as unified diff text.
	Diff(pullRequest PullRequest) (string, error)
	// ChangedFiles lists the files the pull request touches that still exist on its branch.
	ChangedFiles(pullRequest PullRequest) ([]string, error)
	Description(pullRequest PullRequest) (string, error)
	SaveDescription(pullRequest PullRequest, body string) error
	Comments(pullRequest PullRequest) ([]ReviewComment, error)
	PostComment(pullRequest PullRequest, target CommentTarget, body string) error
	SetDraft(pullRequest PullRequest, draft bool) error
	SetAutoMerge(pullRequest PullRequest, enabled bool) error
	RequestTeamReview(pullRequest PullRequest) error
}

// SectionDecoder decodes one table of the configuration file into target, rejecting unknown
// keys. A provider uses it to read its own settings without this package knowing them.
type SectionDecoder func(name string, target any) error

type ProviderFactory func(decodeSection SectionDecoder) (Provider, error)

var providerFactories = map[string]ProviderFactory{}

// RegisterProvider makes a provider selectable by the name used in the configuration file.
// Providers call it from init, so a program gets the providers it imports.
func RegisterProvider(name string, factory ProviderFactory) {
	if _, taken := providerFactories[name]; taken {
		panic(fmt.Sprintf("pull request provider %q is registered twice", name))
	}
	providerFactories[name] = factory
}

func providerNames() []string {
	names := make([]string, 0, len(providerFactories))
	for name := range providerFactories {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func NewProvider(name string, decodeSection SectionDecoder) (Provider, error) {
	factory, known := providerFactories[name]
	if !known {
		return nil, fmt.Errorf(
			"[pull_requests] provider %q is not supported; supported providers: %s", name, strings.Join(providerNames(), ", "),
		)
	}
	return factory(decodeSection)
}
