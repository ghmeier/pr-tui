package main

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	defaultRefreshInterval = 3 * time.Minute
	defaultEditorName      = "VS Code"
	defaultProvider        = "github"
	defaultCloneRoot       = "~/git"
)

var (
	defaultEditorCommand = []string{"code"}
	knownPlugins         = []string{"pull_requests"}
	knownProviders       = []string{"github"}
)

// repositoryPartPattern is deliberately narrow: the owner and name are written into a
// GraphQL query, so anything beyond what GitHub allows in them is rejected here.
var repositoryPartPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type repository struct {
	owner string
	name  string
}

func (r repository) fullName() string {
	return r.owner + "/" + r.name
}

type editorConfiguration struct {
	name string
	// command is the executable followed by its arguments.
	command []string
}

type pullRequestsConfiguration struct {
	provider     string
	repositories []repository
	reviewTeam   string
	cloneRoot    string
}

// configuration is the validated form of the configuration file, with defaults applied.
type configuration struct {
	plugins         []string
	refreshInterval time.Duration
	editor          editorConfiguration
	pullRequests    pullRequestsConfiguration
}

// fileConfiguration mirrors the file as written, before validation.
type fileConfiguration struct {
	Plugins         []string `toml:"plugins"`
	RefreshInterval string   `toml:"refresh_interval"`
	Editor          struct {
		Name string `toml:"name"`
		// Command is a string or a list of strings, so it is checked by editorCommandFrom.
		Command any `toml:"command"`
	} `toml:"editor"`
	PullRequests struct {
		Provider     string   `toml:"provider"`
		Repositories []string `toml:"repositories"`
		ReviewTeam   string   `toml:"review_team"`
		CloneRoot    string   `toml:"clone_root"`
	} `toml:"pull_requests"`
}

const starterConfiguration = `# Configuration for prs, a board of open pull requests.

# Which parts of the board to show. Only "pull_requests" exists today.
plugins = ["pull_requests"]

# How often the board refreshes itself, as a Go duration such as "90s" or "5m", at least "1s".
refresh_interval = "3m"

# The editor the "v" key opens a pull request's worktree in.
[editor]
name = "VS Code"   # shown in the help line
command = "code"
# The command may also be a list: the executable followed by its arguments. A single string is
# always one executable, even if it contains spaces.
# command = ["code", "--reuse-window"]

[pull_requests]
# Where pull requests come from. Only "github" is supported today.
provider = "github"

# Required. Each entry is "owner/name"; the owners may differ.
repositories = ["your-organization/your-repository"]

# Optional. When set, the "e" key requests a review from this team and the board marks
# pull requests that are waiting on it.
# review_team = "engineering"

# Local clones are expected at <clone_root>/<repository name>.
clone_root = "~/git"
`

// configurationPath picks the configuration file: the --config flag, then $PRS_CONFIG, then the
// XDG configuration directory, then ~/.config.
func configurationPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if fromEnvironment := os.Getenv("PRS_CONFIG"); fromEnvironment != "" {
		return fromEnvironment, nil
	}
	if directory := os.Getenv("XDG_CONFIG_HOME"); directory != "" {
		return filepath.Join(directory, "prs", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "prs", "config.toml"), nil
}

// expandHome replaces a leading ~ with the home directory, since the shell does not do it
// for paths read from a file.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}

func parseRepository(text string) (repository, bool) {
	owner, name, found := strings.Cut(text, "/")
	if !found || !repositoryPartPattern.MatchString(owner) || !repositoryPartPattern.MatchString(name) {
		return repository{}, false
	}
	return repository{owner: owner, name: name}, true
}

// loadConfiguration reads and validates the file. Every error names the file, and the
// offending key where there is one.
func loadConfiguration(path string) (configuration, error) {
	var raw fileConfiguration
	metadata, err := toml.DecodeFile(path, &raw)
	if errors.Is(err, fs.ErrNotExist) {
		return configuration{}, fmt.Errorf("no configuration file at %s; run prs --init to create one", path)
	}
	if err != nil {
		return configuration{}, fmt.Errorf("%s: %w", path, err)
	}
	// Unknown keys are reported first, because a misspelt key would otherwise show up as a
	// confusing "missing" error for the key that was meant.
	for _, unknownKey := range metadata.Undecoded() {
		// The contents of a malformed command are reported by validation, which names the key.
		if slices.Equal(unknownKey[:min(len(unknownKey), 2)], []string{"editor", "command"}) {
			continue
		}
		return configuration{}, fmt.Errorf("%s: unknown key %q; check its spelling", path, unknownKey.String())
	}
	result, err := validateConfiguration(raw)
	if err != nil {
		return configuration{}, fmt.Errorf("%s: %w", path, err)
	}
	return result, nil
}

// editorCommandFrom accepts one executable as a string, or an executable followed by its
// arguments as a list. A string is never split on spaces, because paths contain them.
func editorCommandFrom(value any) ([]string, error) {
	var command []string
	switch typed := value.(type) {
	case nil:
		return defaultEditorCommand, nil
	case string:
		command = []string{typed}
	case []any:
		for _, element := range typed {
			text, isText := element.(string)
			if !isText {
				return nil, fmt.Errorf("[editor] command must contain only strings, found %v", element)
			}
			command = append(command, text)
		}
	default:
		return nil, fmt.Errorf("[editor] command must be a string or a list of strings, not %v", value)
	}
	if len(command) == 0 || command[0] == "" {
		return nil, errors.New("[editor] command needs an executable as its first element")
	}
	return command, nil
}

func validateConfiguration(raw fileConfiguration) (configuration, error) {
	editorCommand, err := editorCommandFrom(raw.Editor.Command)
	if err != nil {
		return configuration{}, err
	}
	result := configuration{
		plugins:         raw.Plugins,
		refreshInterval: defaultRefreshInterval,
		editor: editorConfiguration{
			name:    cmp.Or(raw.Editor.Name, defaultEditorName),
			command: editorCommand,
		},
		pullRequests: pullRequestsConfiguration{
			provider:   cmp.Or(raw.PullRequests.Provider, defaultProvider),
			reviewTeam: raw.PullRequests.ReviewTeam,
		},
	}
	if raw.Plugins == nil {
		result.plugins = knownPlugins
	}
	for _, plugin := range result.plugins {
		if !slices.Contains(knownPlugins, plugin) {
			return configuration{}, fmt.Errorf(
				"plugins contains unknown plugin %q; known plugins: %s", plugin, strings.Join(knownPlugins, ", "),
			)
		}
	}
	if raw.RefreshInterval != "" {
		interval, err := time.ParseDuration(raw.RefreshInterval)
		if err != nil {
			return configuration{}, fmt.Errorf(`refresh_interval %q is not a duration such as "90s" or "5m"`, raw.RefreshInterval)
		}
		if interval < time.Second {
			return configuration{}, fmt.Errorf("refresh_interval %q must be at least one second", raw.RefreshInterval)
		}
		result.refreshInterval = interval
	}
	if !slices.Contains(knownProviders, result.pullRequests.provider) {
		return configuration{}, fmt.Errorf(
			"[pull_requests] provider %q is not supported; supported providers: %s",
			result.pullRequests.provider, strings.Join(knownProviders, ", "),
		)
	}
	if len(raw.PullRequests.Repositories) == 0 {
		return configuration{}, errors.New(`[pull_requests] repositories is required; list each one as "owner/name"`)
	}
	for _, text := range raw.PullRequests.Repositories {
		parsed, valid := parseRepository(text)
		if !valid {
			return configuration{}, fmt.Errorf(`[pull_requests] repositories entry %q must be written as "owner/name"`, text)
		}
		if slices.Contains(result.pullRequests.repositories, parsed) {
			return configuration{}, fmt.Errorf("[pull_requests] repositories lists %q more than once", text)
		}
		result.pullRequests.repositories = append(result.pullRequests.repositories, parsed)
	}
	cloneRoot, err := expandHome(cmp.Or(raw.PullRequests.CloneRoot, defaultCloneRoot))
	if err != nil {
		return configuration{}, fmt.Errorf("[pull_requests] clone_root: %w", err)
	}
	result.pullRequests.cloneRoot = cloneRoot
	return result, nil
}

// writeStarterConfiguration never overwrites: the file is created exclusively, so an
// existing configuration is safe even if two runs race.
func writeStarterConfiguration(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s already exists; not overwriting it", path)
	}
	if err != nil {
		return err
	}
	_, err = file.WriteString(starterConfiguration)
	return cmp.Or(err, file.Close())
}
