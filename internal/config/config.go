// Package config reads the configuration file. It owns the top-level keys and the
// [editor] table; every other table is a section that some other package decodes for itself.
package config

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	defaultRefreshInterval = 3 * time.Minute
	defaultEditorName      = "VS Code"
)

var (
	defaultEditorCommand = []string{"code"}
	defaultPlugins       = []string{"pull_requests"}
)

type Editor struct {
	Name string
	// Command is the executable followed by its arguments.
	Command []string
}

// File is the validated form of the configuration file, with defaults applied.
type File struct {
	Plugins         []string
	RefreshInterval time.Duration
	Editor          Editor

	sections map[string]map[string]any
}

// rawFile mirrors the keys this package owns as written, before validation.
type rawFile struct {
	Plugins         []string `toml:"plugins"`
	RefreshInterval string   `toml:"refresh_interval"`
	Editor          struct {
		Name string `toml:"name"`
		// Command is a string or a list of strings, so it is checked by editorCommandFrom.
		Command any `toml:"command"`
	} `toml:"editor"`
}

// ResolvePath picks the configuration file: the --config flag, then $PRS_CONFIG, then the
// XDG configuration directory, then ~/.config.
func ResolvePath(flagValue string) (string, error) {
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

// ExpandHome replaces a leading ~ with the home directory, since the shell does not do it
// for paths read from a file.
func ExpandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}

// Load reads and validates the file. Every error names the file, and the offending key
// where there is one.
func Load(path string) (*File, error) {
	text, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no configuration file at %s; run prs --init to create one", path)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var raw rawFile
	metadata, err := toml.Decode(string(text), &raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var everything map[string]any
	if _, err := toml.Decode(string(text), &everything); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// Every table except [editor] belongs to some other package.
	sections := map[string]map[string]any{}
	for name, value := range everything {
		if table, isTable := value.(map[string]any); isTable && name != "editor" {
			sections[name] = table
		}
	}
	// Unknown keys are reported first, because a misspelt key would otherwise show up as a
	// confusing "missing" error for the key that was meant.
	for _, unknownKey := range metadata.Undecoded() {
		if _, isSection := sections[unknownKey[0]]; isSection {
			continue
		}
		// The contents of a malformed command are reported by validation, which names the key.
		if slices.Equal(unknownKey[:min(len(unknownKey), 2)], []string{"editor", "command"}) {
			continue
		}
		return nil, fmt.Errorf("%s: unknown key %q; check its spelling", path, unknownKey.String())
	}
	file, err := validate(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	file.sections = sections
	return file, nil
}

// RejectUnknownSections fails on any table that is neither [editor] nor one of the known
// sections, which are the tables that the caller's packages read. The error does not name
// the file; the caller does, once, for every error it reports.
func (file *File) RejectUnknownSections(known ...string) error {
	for _, name := range slices.Sorted(maps.Keys(file.sections)) {
		if !slices.Contains(known, name) {
			return fmt.Errorf("unknown section [%s]; known sections: editor, %s", name, strings.Join(known, ", "))
		}
	}
	return nil
}

// Section decodes the named table into target, rejecting any key the target does not
// declare. A table that is not in the file decodes as empty, leaving target untouched.
// Errors do not name the file.
func (file *File) Section(name string, target any) error {
	table, present := file.sections[name]
	if !present {
		return nil
	}
	// Decoding the table's own text, rather than the whole file, is what lets the metadata
	// say which keys of this section the target left unread.
	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(table); err != nil {
		return fmt.Errorf("[%s] %w", name, err)
	}
	metadata, err := toml.Decode(encoded.String(), target)
	if err != nil {
		return fmt.Errorf("[%s] %w", name, err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return fmt.Errorf("[%s] unknown key %q; check its spelling", name, undecoded[0].String())
	}
	return nil
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

func validate(raw rawFile) (*File, error) {
	editorCommand, err := editorCommandFrom(raw.Editor.Command)
	if err != nil {
		return nil, err
	}
	result := &File{
		Plugins:         raw.Plugins,
		RefreshInterval: defaultRefreshInterval,
		Editor: Editor{
			Name:    cmp.Or(raw.Editor.Name, defaultEditorName),
			Command: editorCommand,
		},
	}
	if raw.Plugins == nil {
		result.Plugins = defaultPlugins
	}
	if raw.RefreshInterval != "" {
		interval, err := time.ParseDuration(raw.RefreshInterval)
		if err != nil {
			return nil, fmt.Errorf(`refresh_interval %q is not a duration such as "90s" or "5m"`, raw.RefreshInterval)
		}
		if interval < time.Second {
			return nil, fmt.Errorf("refresh_interval %q must be at least one second", raw.RefreshInterval)
		}
		result.RefreshInterval = interval
	}
	return result, nil
}

// starterConfiguration is the commented file that --init writes. It shows every section, so
// it is the one place that has to change when a section gains a key.
const starterConfiguration = `# Configuration for prs, a board of open pull requests.

# Which parts of the board to show: "pull_requests", and optionally "agent_plans".
plugins = ["pull_requests"]
# plugins = ["pull_requests", "agent_plans"]

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
# Where pull requests come from. Only "github" is supported today. The settings for the
# provider live in the table named after it, below.
provider = "github"

# Local clones are expected at <clone_root>/<repository name>.
clone_root = "~/git"

[github]
# Required. Each entry is "owner/name"; the owners may differ.
repositories = ["your-organization/your-repository"]

# Optional. When set, the "e" key requests a review from this team and the board marks
# pull requests that are waiting on it.
# review_team = "engineering"

# Only read when "agent_plans" is in the plugins list above.
# [agent_plans]
# Required. Local checkouts whose apl documents (plans, research, handoffs) are listed. Each
# must contain a .apl directory, and no two may share a directory name.
# repositories = ["~/git/your-repository"]
`

// WriteStarter never overwrites: the file is created exclusively, so an existing
// configuration is safe even if two runs race.
func WriteStarter(path string) error {
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
