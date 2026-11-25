package agentsessions

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Provider is where sessions come from, and how a message reaches one. Both may block.
type Provider interface {
	// Sessions lists the live sessions with what each one has mentioned.
	Sessions() ([]Session, error)
	// Send delivers the message to the session, and fails when it cannot say it was delivered.
	Send(session Session, message string) error
}

// ProviderSettings are the plugin settings that a provider needs.
type ProviderSettings struct {
	// RelayModel is the model that relays a message, for a provider that needs one.
	RelayModel string
}

// ProviderFactory makes a provider. A provider reads nothing else from the configuration file:
// the login of the tool it calls is its credential.
type ProviderFactory func(settings ProviderSettings) (Provider, error)

var providerFactories = map[string]ProviderFactory{}

// RegisterProvider makes a provider selectable by the name used in the configuration file.
// Providers call it from init, so a program gets the providers it imports.
func RegisterProvider(name string, factory ProviderFactory) {
	if _, taken := providerFactories[name]; taken {
		panic(fmt.Sprintf("agent session provider %q is registered twice", name))
	}
	providerFactories[name] = factory
}

func providerNames() []string {
	return slices.Sorted(maps.Keys(providerFactories))
}

func newProvider(name string, settings ProviderSettings) (Provider, error) {
	factory, known := providerFactories[name]
	if !known {
		return nil, fmt.Errorf(
			"[%s] provider %q is not supported; supported providers: %s", pluginName, name, strings.Join(providerNames(), ", "),
		)
	}
	return factory(settings)
}
