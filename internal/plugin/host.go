package plugin

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ghmeier/pr-tui/internal/config"
)

// reservedKeys are the keys the host handles itself on a board, so no action may claim them.
var reservedKeys = []string{"up", "k", "down", "j", "pgup", "pgdown", "home", "end", "f", "r", "q", "tab", "shift+tab", "ctrl+c"}

// filterMenuKeys are the keys the filter menu handles itself, so no facet may use one as its
// cycle key.
var filterMenuKeys = []string{"up", "k", "down", "j", " ", "f", "esc", "enter", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

type usageError struct{ err error }

func (usage usageError) Error() string { return usage.err.Error() }
func (usage usageError) Unwrap() error { return usage.err }

// UsageError marks an error from Setup as a mistake in the command line rather than in the
// configuration file, so the host does not name the file in it.
func UsageError(err error) error {
	if err == nil {
		return nil
	}
	return usageError{err: err}
}

type pendingColumn struct {
	plugin string
	board  string
	column Column
}

type pendingAction struct {
	plugin string
	board  string
	action Action
}

// Host is what plugins register their contributions with while they are set up.
type Host struct {
	file    *config.File
	current string

	sections    []string
	boards      []Board
	boardOwners []string
	columns     []pendingColumn
	actions     []pendingAction
}

// NewHost sets up the plugins that the configuration file enables, in the order it lists them.
// The path is only used to name the file in errors, which it does exactly once.
func NewHost(path string, file *config.File, available []Plugin) (*Host, error) {
	host := &Host{file: file}
	if err := host.setUp(file, available); err != nil {
		var usage usageError
		if errors.As(err, &usage) {
			return nil, err
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return host, nil
}

func (host *Host) setUp(file *config.File, available []Plugin) error {
	names := make([]string, len(available))
	for index, plugin := range available {
		names[index] = plugin.Name()
	}
	var enabled []string
	for _, name := range file.Plugins {
		index := slices.Index(names, name)
		if index < 0 {
			return fmt.Errorf("plugins contains unknown plugin %q; registered plugins: %s", name, strings.Join(names, ", "))
		}
		if slices.Contains(enabled, name) {
			return fmt.Errorf("plugins lists %q more than once", name)
		}
		enabled = append(enabled, name)
		host.current = name
		if err := available[index].Setup(host); err != nil {
			return err
		}
	}
	host.current = ""

	if err := file.RejectUnknownSections(append(slices.Clone(names), host.sections...)...); err != nil {
		return err
	}
	return host.resolveContributions()
}

// resolveContributions attaches the columns and actions that plugins added to boards. It runs
// after every plugin is set up, so the order of the plugins does not matter.
func (host *Host) resolveContributions() error {
	for index, board := range host.boards {
		if board.Name == "" {
			return fmt.Errorf("plugin %q registers a board without a name", host.boardOwners[index])
		}
		if first, _ := host.boardIndex(board.Name); first != index {
			return fmt.Errorf("plugins %q and %q both register a board named %q", host.boardOwners[first], host.boardOwners[index], board.Name)
		}
	}
	for _, added := range host.columns {
		index, found := host.boardIndex(added.board)
		if !found {
			return fmt.Errorf("plugin %q adds a column to board %q, which no enabled plugin registers", added.plugin, added.board)
		}
		host.boards[index].Columns = append(host.boards[index].Columns, added.column)
	}
	for _, added := range host.actions {
		index, found := host.boardIndex(added.board)
		if !found {
			return fmt.Errorf("plugin %q adds an action to board %q, which no enabled plugin registers", added.plugin, added.board)
		}
		host.boards[index].Actions = append(host.boards[index].Actions, added.action)
	}
	for index, board := range host.boards {
		if err := validateBoard(board, host.boardOwners[index]); err != nil {
			return err
		}
	}
	return nil
}

func (host *Host) boardIndex(name string) (int, bool) {
	index := slices.IndexFunc(host.boards, func(board Board) bool { return board.Name == name })
	return index, index >= 0
}

func validateBoard(board Board, owner string) error {
	if board.Load == nil {
		return fmt.Errorf("plugin %q registers board %q without a Load function", owner, board.Name)
	}
	flexible := 0
	for _, column := range board.Columns {
		if column.Cell == nil {
			return fmt.Errorf("board %q has column %q without a Cell function", board.Name, column.Header)
		}
		if column.Flexible {
			flexible++
		}
	}
	if flexible > 1 {
		return fmt.Errorf("board %q has %d flexible columns; at most one may be flexible", board.Name, flexible)
	}
	var keys []string
	for _, action := range board.Actions {
		if action.Run == nil {
			return fmt.Errorf("board %q has an action on %q without a Run function", board.Name, action.Key)
		}
		if slices.Contains(reservedKeys, action.Key) {
			return fmt.Errorf("board %q has an action on %q, a key the board uses itself", board.Name, action.Key)
		}
		if slices.Contains(keys, action.Key) {
			return fmt.Errorf("board %q has two actions on the key %q", board.Name, action.Key)
		}
		keys = append(keys, action.Key)
	}
	for _, facet := range board.Facets {
		if facet.Value == nil {
			return fmt.Errorf("board %q has facet %q without a Value function", board.Name, facet.Title)
		}
		if facet.CycleKey != "" && slices.Contains(filterMenuKeys, facet.CycleKey) {
			return fmt.Errorf("board %q facet %q cycles on %q, a key the filter menu uses itself", board.Name, facet.Title, facet.CycleKey)
		}
	}
	return nil
}

// Section decodes a table of the configuration file into target, rejecting any key the target
// does not declare. Asking for a table also makes it a known table of the file.
func (host *Host) Section(name string, target any) error {
	if !slices.Contains(host.sections, name) {
		host.sections = append(host.sections, name)
	}
	return host.file.Section(name, target)
}

// Enabled says whether the configuration file's plugins list contains the named plugin. The
// answer does not depend on the order the plugins are set up in, so a plugin can use it to
// offer a contribution only when another plugin is there to receive it.
func (host *Host) Enabled(pluginName string) bool {
	return slices.Contains(host.file.Plugins, pluginName)
}

func (host *Host) Editor() config.Editor { return host.file.Editor }

// AddBoard registers a board, shown as a tab after the boards registered before it.
func (host *Host) AddBoard(board Board) {
	host.boards = append(host.boards, board)
	host.boardOwners = append(host.boardOwners, host.current)
}

// AddColumn appends a column after the board's own columns. The board may belong to another
// plugin, and may be registered after this call.
func (host *Host) AddColumn(boardName string, column Column) {
	host.columns = append(host.columns, pendingColumn{plugin: host.current, board: boardName, column: column})
}

// AddAction adds a key to the board. The board may belong to another plugin.
func (host *Host) AddAction(boardName string, action Action) {
	host.actions = append(host.actions, pendingAction{plugin: host.current, board: boardName, action: action})
}
