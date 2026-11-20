package workflows

import (
	"fmt"
	"slices"
	"strings"
)

// Provider is where workflow runs come from. Every method that takes a repository, a workflow
// or a run is given the values the provider itself returned, and may block on the network.
type Provider interface {
	// Runs returns the repository's most recent runs that match the query, newest first.
	Runs(query RunQuery) ([]Run, error)
	// Workflows lists the repository's workflows with their triggers and schedules.
	Workflows(repository Repository) ([]Workflow, error)
	// LatestRun returns the workflow's most recent run, and false when it has never run.
	LatestRun(workflow Workflow) (Run, bool, error)
	Jobs(run Run) ([]Job, error)
}

// ProviderFactory makes a provider. A provider reads nothing from the configuration file: the
// repositories are the plugin's setting, and the login of the tool it calls is its credential.
type ProviderFactory func() (Provider, error)

var providerFactories = map[string]ProviderFactory{}

// RegisterProvider makes a provider selectable by the name used in the configuration file.
// Providers call it from init, so a program gets the providers it imports.
func RegisterProvider(name string, factory ProviderFactory) {
	if _, taken := providerFactories[name]; taken {
		panic(fmt.Sprintf("workflow provider %q is registered twice", name))
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

func newProvider(name string) (Provider, error) {
	factory, known := providerFactories[name]
	if !known {
		return nil, fmt.Errorf(
			"[%s] provider %q is not supported; supported providers: %s", pluginName, name, strings.Join(providerNames(), ", "),
		)
	}
	return factory()
}
