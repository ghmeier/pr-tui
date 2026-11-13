// Package plugin is the host that boards are built on. A plugin registers contributions, such
// as a board or a column on another plugin's board, and the host owns everything generic
// about showing them: the table, cursor, filter menu, refresh, status line, confirmation and
// tabs.
package plugin

import (
	"flag"
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// Plugin is compiled in and registers itself from init.
type Plugin interface {
	// Name is also the name of the plugin's configuration table.
	Name() string
	// Flags registers the plugin's command-line flags. It is called for every registered
	// plugin, enabled or not, because flags are parsed before the configuration is read.
	Flags(flags *flag.FlagSet)
	// Setup reads the plugin's configuration and registers its contributions.
	Setup(host *Host) error
}

var registered []Plugin

// Register makes a plugin available to be enabled in the configuration file. Plugins call it
// from init, so a program gets the plugins it imports.
func Register(plugin Plugin) {
	for _, existing := range registered {
		if existing.Name() == plugin.Name() {
			panic(fmt.Sprintf("plugin %q is registered twice", plugin.Name()))
		}
	}
	registered = append(registered, plugin)
}

// Registered returns the plugins passed to Register, in registration order.
func Registered() []Plugin {
	return slices.Clone(registered)
}

// RegisterFlags lets every plugin declare its command-line flags.
func RegisterFlags(plugins []Plugin, flags *flag.FlagSet) {
	for _, plugin := range plugins {
		plugin.Flags(flags)
	}
}

// Row is one line of a board.
type Row interface {
	// Key is the row's identity across refreshes, such as a URL. The cursor and the loaded
	// column data follow it.
	Key() string
}

// Column is one column of a board's table.
type Column struct {
	Header string
	// Flexible marks the column that shrinks when the table is wider than the terminal. At
	// most one column of a board is flexible.
	Flexible bool
	// Cell draws the column for a row. The data is nil until a fetch from Load has delivered
	// a value for the row. It runs on the update loop, so it must return quickly.
	Cell func(row Row, data any) []ui.Segment
	// Load is optional. It returns independent fetches that run in the background after each
	// refresh, each delivering values keyed by Row.Key. Delivered values survive later
	// refreshes. Load itself runs on the update loop and must return quickly without
	// blocking; the work belongs in the fetches it returns.
	Load func(rows []Row) []Fetch
}

// Fetch is one background lookup of column data. A fetch that fails leaves its cells unloaded.
// Fetches run on background goroutines, several at once.
type Fetch func() (map[string]any, error)

// Selection is the row an action was invoked on, and the way for a screen or a background
// command to hand an effect back to the board that row belongs to.
type Selection struct {
	Row Row
	// Data is the loaded value of the column with the given header for this row, or nil. It
	// reads a copy made when the selection was built, so it is safe to call from any
	// goroutine, including an Effect.Command.
	Data func(columnHeader string) any

	boardIndex int
}

// Run has the host carry out an effect for this selection's board, from a screen or a
// background command. The effect applies to the board the selection came from, whichever
// board is in view when it arrives.
func (selection Selection) Run(effect Effect) tea.Cmd {
	return func() tea.Msg {
		return effectMessage{board: selection.boardIndex, effect: effect}
	}
}

// Action is a key that does something to the selected row.
type Action struct {
	Key string
	// Label is shown in the help line. An empty label hides the action there.
	Label string
	// Run describes what the key does. It runs on the update loop, so it must return quickly
	// without blocking; slow work belongs in the returned Effect's Command.
	Run func(selection Selection) Effect
}

// Effect describes what an action does, so that the host can show notices, ask, and refresh.
type Effect struct {
	// Notice is shown in the status line while Command runs.
	Notice string
	// Confirm, when set, is a question the host asks before doing anything else: y or enter
	// accepts it, n or esc declines it.
	Confirm string
	// Command runs on a background goroutine. A failure shows "Failed: " and its first line.
	// State that it shares with a function that runs on the update loop needs a lock.
	Command func() error
	// Refresh reloads the board after Command succeeds.
	Refresh bool
	// Screen, when set, opens a full-screen view.
	Screen Screen
}

// Screen is a full-screen view above the board. While any screen is open it receives every key
// except ctrl+c, and every message the host does not recognise, because screens run their own
// background commands. A screen must ignore messages that are not its own.
type Screen interface {
	Init() tea.Cmd
	// Update returns a nil Screen to close itself.
	Update(message tea.Msg) (Screen, tea.Cmd)
	View(width, height int) string
}

// FacetOption is one choice of a facet.
type FacetOption struct {
	Value string
	Label string
}

// Facet is a way of narrowing the rows of a board, shown in the filter menu.
type Facet struct {
	Title   string
	Options []FacetOption
	// Value is the option value a row has for this facet. It runs on the update loop, so it
	// must return quickly.
	Value func(row Row) string
	// Single means exactly one option is selected at a time.
	Single bool
	// Selected is the initial selection. Empty means every option, or the first one for a
	// single facet.
	Selected []string
	// CycleKey is an optional key that, while the filter menu is open, selects the next
	// option of a single facet.
	CycleKey string
}

// Board is a table of rows with its own columns, actions and facets.
type Board struct {
	// Name is what other plugins pass to AddColumn and AddAction.
	Name string
	// Title is the label of the board's tab.
	Title string
	// RowNoun says what the rows are, such as "open pull requests", for the status line.
	RowNoun string
	// Load fetches the rows. It runs on a background goroutine. State that it shares with a
	// function that runs on the update loop, such as Cell or Action.Run, needs a lock.
	Load    func() ([]Row, error)
	Columns []Column
	Actions []Action
	Facets  []Facet
}
