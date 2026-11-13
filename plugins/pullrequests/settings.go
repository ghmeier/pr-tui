package pullrequests

import (
	"cmp"
	"fmt"

	"github.com/ghmeier/pr-tui/internal/config"
)

const (
	defaultProviderName = "github"
	defaultCloneRoot    = "~/git"
)

// boardSettings is what the board needs besides its provider.
type boardSettings struct {
	// ProviderName selects the provider; the provider reads its own table of the file.
	ProviderName string
	CloneRoot    string
}

// loadSettings reads the [pull_requests] table.
func loadSettings(decodeSection SectionDecoder) (boardSettings, error) {
	var section struct {
		Provider  string `toml:"provider"`
		CloneRoot string `toml:"clone_root"`
		// These two were settings of this table before each provider got a table of its own.
		// They are decoded only so that they can be refused with a pointer to the new table.
		Repositories any `toml:"repositories"`
		ReviewTeam   any `toml:"review_team"`
	}
	if err := decodeSection(pluginName, &section); err != nil {
		return boardSettings{}, err
	}
	providerName := cmp.Or(section.Provider, defaultProviderName)
	if section.Repositories != nil {
		return boardSettings{}, movedKeyError("repositories", providerName)
	}
	if section.ReviewTeam != nil {
		return boardSettings{}, movedKeyError("review_team", providerName)
	}
	cloneRoot, err := config.ExpandHome(cmp.Or(section.CloneRoot, defaultCloneRoot))
	if err != nil {
		return boardSettings{}, fmt.Errorf("[%s] clone_root: %w", pluginName, err)
	}
	return boardSettings{ProviderName: providerName, CloneRoot: cloneRoot}, nil
}

func movedKeyError(key, providerName string) error {
	return fmt.Errorf("[%s] %s now belongs under [%s], the table named after the provider", pluginName, key, providerName)
}
