package agentplans

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ghmeier/pr-tui/internal/config"
)

// checkout is one configured local repository.
type checkout struct {
	// name is the base name of the directory, which is how the repository is told apart in the
	// filter and matched to pull requests.
	name string
	path string
}

type settings struct {
	checkouts []checkout
	// editor opens a document's file. It is set once during setup and not changed after.
	editor config.Editor
}

// loadSettings reads the [agent_plans] table.
func loadSettings(decodeSection func(name string, target any) error) (settings, error) {
	var section struct {
		Repositories []string `toml:"repositories"`
	}
	if err := decodeSection(pluginName, &section); err != nil {
		return settings{}, err
	}
	if len(section.Repositories) == 0 {
		return settings{}, fmt.Errorf("[%s] repositories must list at least one local checkout", pluginName)
	}
	var loaded settings
	for _, configured := range section.Repositories {
		path, err := config.ExpandHome(configured)
		if err != nil {
			return settings{}, fmt.Errorf("[%s] repositories: cannot expand %q: %w", pluginName, configured, err)
		}
		if _, err := os.Stat(filepath.Join(path, ".apl")); err != nil {
			return settings{}, fmt.Errorf("[%s] repositories: %q has no .apl directory (%v)", pluginName, configured, err)
		}
		name := filepath.Base(path)
		for _, earlier := range loaded.checkouts {
			if earlier.name == name {
				return settings{}, fmt.Errorf("[%s] repositories: %q and %q are both named %q, and the name is how a repository is told apart", pluginName, earlier.path, path, name)
			}
		}
		loaded.checkouts = append(loaded.checkouts, checkout{name: name, path: path})
	}
	return loaded, nil
}
