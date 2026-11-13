package pullrequests

import (
	"cmp"
	"fmt"
	"time"

	"github.com/ghmeier/pr-tui/internal/config"
)

const (
	DefaultProviderName = "github"
	DefaultCloneRoot    = "~/git"
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
	}
	if err := file.Section("pull_requests", &section); err != nil {
		return Settings{}, err
	}
	cloneRoot, err := config.ExpandHome(cmp.Or(section.CloneRoot, DefaultCloneRoot))
	if err != nil {
		return Settings{}, fmt.Errorf("[pull_requests] clone_root: %w", err)
	}
	return Settings{
		ProviderName:    cmp.Or(section.Provider, DefaultProviderName),
		CloneRoot:       cloneRoot,
		RefreshInterval: file.RefreshInterval,
		Editor:          file.Editor,
	}, nil
}
