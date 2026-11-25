package agentsessions

import (
	"cmp"
	"fmt"
)

const (
	defaultProviderName = "claude"
	defaultRelayModel   = "haiku"
)

type settings struct {
	providerName string
	relayModel   string
}

// loadSettings reads the [agent_sessions] table. The table is optional.
func loadSettings(decodeSection func(name string, target any) error) (settings, error) {
	var section struct {
		Provider   string  `toml:"provider"`
		RelayModel *string `toml:"relay_model"`
	}
	if err := decodeSection(pluginName, &section); err != nil {
		return settings{}, err
	}
	loaded := settings{providerName: cmp.Or(section.Provider, defaultProviderName), relayModel: defaultRelayModel}
	if section.RelayModel != nil {
		loaded.relayModel = *section.RelayModel
	}
	if loaded.relayModel == "" {
		return settings{}, fmt.Errorf("[%s] relay_model must name a model, such as %q", pluginName, defaultRelayModel)
	}
	return loaded, nil
}
