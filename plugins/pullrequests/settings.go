package pullrequests

import (
	"cmp"
	"fmt"
	"time"

	"github.com/ghmeier/pr-tui/internal/config"
)

const (
	defaultProviderName = "github"
	defaultCloneRoot    = "~/git"
)

// Settings is what the board needs besides its provider.
type Settings struct {
	// ProviderName selects the provider; the provider reads its own table of the file.
	ProviderName    string
	CloneRoot       string
	RefreshInterval time.Duration
	Editor          config.Editor
}

// LoadSettings reads the [pull_requests] table, and takes the refresh interval and editor
// from the top level of the file.
func LoadSettings(file *config.File) (Settings, error) {
	var section struct {
		Provider  string `toml:"provider"`
		CloneRoot string `toml:"clone_root"`
		// These two were settings of this table before each provider got a table of its own.
		// They are decoded only so that they can be refused with a pointer to the new table.
		Repositories any `toml:"repositories"`
		ReviewTeam   any `toml:"review_team"`
	}
	if err := file.Section("pull_requests", &section); err != nil {
		return Settings{}, err
	}
	providerName := cmp.Or(section.Provider, defaultProviderName)
	if section.Repositories != nil {
		return Settings{}, movedKeyError("repositories", providerName)
	}
	if section.ReviewTeam != nil {
		return Settings{}, movedKeyError("review_team", providerName)
	}
	cloneRoot, err := config.ExpandHome(cmp.Or(section.CloneRoot, defaultCloneRoot))
	if err != nil {
		return Settings{}, fmt.Errorf("[pull_requests] clone_root: %w", err)
	}
	return Settings{
		ProviderName:    providerName,
		CloneRoot:       cloneRoot,
		RefreshInterval: file.RefreshInterval,
		Editor:          file.Editor,
	}, nil
}

func movedKeyError(key, providerName string) error {
	return fmt.Errorf("[pull_requests] %s now belongs under [%s], the table named after the provider", key, providerName)
}
