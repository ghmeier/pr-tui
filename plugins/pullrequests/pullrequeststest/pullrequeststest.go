// Package pullrequeststest has a small pull request provider for the tests of plugins that
// add columns or actions to the pull request board. The pull request plugin's own tests use a
// larger fake that records the changes it is asked to make.
package pullrequeststest

import (
	"fmt"
	"testing"

	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

// Provider serves one repository with a fixed list of open pull requests. It has no line
// counts or CI states. A method that is not listed here panics, which tells a test that it
// reached for something this provider does not fake.
type Provider struct {
	pullrequests.Provider

	Source pullrequests.Repository
	Open   []pullrequests.PullRequest
}

func (provider *Provider) Repositories() []pullrequests.Repository {
	return []pullrequests.Repository{provider.Source}
}

func (provider *Provider) ReviewTeam() string { return "" }

func (provider *Provider) PullRequests() ([]pullrequests.PullRequest, string, error) {
	return provider.Open, "viewer", nil
}

func (provider *Provider) LineCounts(pullrequests.Repository) (map[string]pullrequests.LineCount, error) {
	return nil, nil
}

func (provider *Provider) CIStates(pullrequests.Repository) (map[string]pullrequests.CIState, error) {
	return nil, nil
}

// Register makes the provider selectable and returns the name to put in the configuration
// file as `provider = "<name>"`. Registrations cannot be removed, so every call uses a name
// that no other call has.
func Register(t testing.TB, provider pullrequests.Provider) string {
	t.Helper()
	name := fmt.Sprintf("fake-%p", provider)
	pullrequests.RegisterProvider(name, func(pullrequests.SectionDecoder) (pullrequests.Provider, error) {
		return provider, nil
	})
	return name
}
