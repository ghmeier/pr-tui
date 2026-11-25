// Package plugin is the host that boards are built on. A plugin registers contributions, such
// as a board or a column on another plugin's board, and the host owns everything generic
// about showing them: the table, cursor, filter menu, refresh, status line, confirmation and
// tabs.
package plugin

import (
	"flag"
	"fmt"
	"slices"
	"time"

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
	// MaxWidth is the widest the column is drawn; zero means no limit.
	MaxWidth int
	// Cell draws the column for a row. The data is nil until a fetch from Load has delivered
	// a value for the row. It runs on the update loop, so it must return quickly.
	Cell func(row Row, data any) []ui.Segment
	// Load is optional. It returns independent fetches that run in the background after each
	// refresh, each delivering values keyed by Row.Key. Delivered values survive later
	// refreshes. Load itself runs on the update loop and must return quickly without
	// blocking; the work belongs in the fetches it returns.
	Load func(rows []Row) []Fetch
	// Within, when set, is the header of another column on the same board. This column is then
	// not drawn as a column of its own: its cell is drawn at the end of that column's cell, after
	// a space, and it loads and delivers data like any other column, so Selection.Data still
	// finds it by its own Header.
	Within string
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
	// Value is what a screen offered the action for, or nil when the action ran from its key.
	Value any

	boardIndex int
	current    func() (Row, bool)
	// latest is the selection for the same row built from the board's rows and data as they are
	// now, which differ from this selection's copy once more has loaded. It is false when the
	// board no longer has the row.
	latest func() (Selection, bool)
	// offerable are the board's actions that take a kind of value. They never change after setup,
	// so a selection shares them safely with any goroutine.
	offerable []Action
}

// Current is the board's version of this selection's row as it is now, found by the row's key
// among all the rows the board has loaded. Row, in contrast, stays as it was when the selection
// was built. The second result is false when the board no longer has the row. It reads the
// board's rows, so call it only on the update loop, such as from a screen's Update or View.
func (selection Selection) Current() (Row, bool) {
	if selection.current == nil {
		return nil, false
	}
	return selection.current()
}

// Run has the host carry out an effect for this selection's board, from a screen or a
// background command. The effect applies to the board the selection came from, whichever
// board is in view when it arrives.
func (selection Selection) Run(effect Effect) tea.Cmd {
	return func() tea.Msg {
		return effectMessage{board: selection.boardIndex, effect: effect}
	}
}

// RunAction has the host run the board's action with the given key on the row as the board has
// it now, found by this selection's row key among all the rows the board has loaded and not
// only the visible ones. When that row is gone it runs on this selection's own row. It is
// exactly as if the key had been pressed on the board, including the question the action asks.
// A key the board has no action for, or whose action does not apply to the row, does nothing.
func (selection Selection) RunAction(key string) tea.Cmd {
	return func() tea.Msg {
		return actionMessage{board: selection.boardIndex, rowKey: selection.Row.Key(), fallback: selection.Row, key: key}
	}
}

// Latest is this selection rebuilt from the board's rows and data as they are now, or this
// selection when the board no longer has the row. A screen uses it to decide what to show from
// what has loaded since the screen opened. It reads the board, so call it only on the update loop.
func (selection Selection) Latest() Selection {
	if selection.latest != nil {
		if latest, found := selection.latest(); found {
			return latest
		}
	}
	return selection
}

// Offers lists the labels of the board's actions that take the kind of value and apply to the
// row as the board has it now, in the order they were added. A screen with a value of that kind
// shows them as choices. The row's data may have loaded since the screen opened, so Applies sees
// the board's current data rather than this selection's copy, as RunOffer does. It runs the
// actions' Applies functions, so call it only on the update loop.
func (selection Selection) Offers(kind string) []string {
	if selection.latest != nil {
		latest, found := selection.latest()
		if !found {
			return nil
		}
		selection = latest
	}
	var labels []string
	for _, action := range selection.offerable {
		if action.Takes == kind && action.offeredFor(selection) {
			labels = append(labels, action.Label)
		}
	}
	return labels
}

// RunOffer has the host run the board's action with that label for the value, on the row as the
// board has it now. The action reads the value from Selection.Value, and its effect is applied
// like any other. When the board no longer has the row, or has no such action or the action no
// longer applies, the board shows a notice saying so and nothing runs.
func (selection Selection) RunOffer(label string, value any) tea.Cmd {
	return func() tea.Msg {
		return offerMessage{board: selection.boardIndex, rowKey: selection.Row.Key(), label: label, value: value}
	}
}

// offeredFor is whether a screen may offer the action for the selection.
func (action Action) offeredFor(selection Selection) bool {
	return action.Applies == nil || action.Applies(selection)
}

// Action is something that can be done to the selected row, either with a key on the board or,
// when it takes a kind of value, as an offer from a screen.
type Action struct {
	// Key is the key that runs the action. An action that takes a value has none.
	Key string
	// Takes, when set, is the kind of value the action works on, such as "comment". Such an
	// action has no key: a screen offers it, by its label, for a value of that kind, and its Run
	// reads the value from Selection.Value. It is not shown in the help line.
	Takes string
	// Applies, when set, says whether the action applies to the selection: a screen offers an
	// action that takes a value only if it does, and a keyed action that does not apply is hidden
	// from the help line and its key does nothing, as when LabelFor returns "". It runs on the
	// update loop, so it must return quickly.
	Applies func(selection Selection) bool
	// Label is shown in the help line. An empty label hides the action there. An action that
	// takes a value needs one, because it is what a screen offers.
	Label string
	// LabelFor, when set, replaces Label for the selected row. An empty label means the action
	// does not apply to that row: it is hidden from the help line and its key does nothing. It
	// runs on the update loop, so it must return quickly.
	LabelFor func(row Row) string
	// Run describes what the key does. It runs on the update loop, so it must return quickly
	// without blocking; slow work belongs in the returned Effect's Command.
	Run func(selection Selection) Effect
}

// LabelOf is the action's label for the selection's row, and whether the action applies to it.
// A selection without a row, which an empty board has, applies only to an action with neither
// LabelFor nor Applies.
func (action Action) LabelOf(selection Selection) (label string, applies bool) {
	if selection.Row == nil {
		return action.Label, action.LabelFor == nil && action.Applies == nil
	}
	if action.Applies != nil && !action.Applies(selection) {
		return "", false
	}
	if action.LabelFor == nil {
		return action.Label, true
	}
	label = action.LabelFor(selection.Row)
	return label, label != ""
}

// Choice asks for one of a few options in the status line, numbered from 1. A number key picks
// one and esc cancels. Options past the ninth are not shown, unless Menu is set.
type Choice struct {
	Question string
	Options  []string
	// Current is marked among the options, or empty.
	Current string
	// Cancel, when set, returns what esc does. Without it esc does nothing.
	Cancel func() Effect
	// Menu shows the options in a list above the board, or at the bottom of a screen, instead of
	// in the status line. It suits a choice that can have more options than number keys: the list
	// scrolls, typing narrows it to the options that contain the typed text, ignoring case, the
	// arrows move the cursor, enter picks the option under it, backspace removes a typed
	// character, and esc cancels. Every key but ctrl+c goes to the menu, q included.
	Menu bool
	// Pick returns what choosing the option does. It runs on the update loop, so it must return
	// quickly; slow work belongs in the returned Effect's Command.
	Pick func(option string) Effect
}

// Effect describes what an action does, so that the host can show notices, ask, and refresh.
type Effect struct {
	// Notice is shown in the status line while Command runs.
	Notice string
	// Then, when set, is a command the host starts along with the effect. The message it returns
	// reaches the open screens like any message the host does not recognise, so a screen can ask
	// a question and get the work of the answer done by its own command. It is dropped when
	// Choice or Edit is set, and with Confirm it starts only after the answer is yes.
	Then tea.Cmd
	// Choice, when set, is asked before anything else, and the host does nothing else with this
	// effect. The effect that Pick returns for the chosen option is applied like any other, so
	// it may ask a question of its own.
	Choice *Choice
	// Confirm, when set, is a question the host asks before doing anything else: y or enter
	// accepts it, n or esc declines it.
	Confirm string
	// Command runs on a background goroutine. A failure shows "Failed: " and its first line.
	// State that it shares with a function that runs on the update loop needs a lock.
	Command func() error
	// Refresh reloads the whole board after Command succeeds, or at once when there is no
	// Command. It is an alternative to RefreshRow, which is cheaper when the command changed one
	// row.
	Refresh bool
	// RefreshRow reloads only this row, with the board's Reload, after Command succeeds, or at
	// once when there is no Command. A board without a Reload is reloaded whole, as Refresh does.
	// It is an alternative to Refresh.
	RefreshRow Row
	// Copy, when set, is put on the clipboard on a background goroutine. The status line then
	// shows "Copied " and the text when it is one line, or "Copied N lines" when it has several,
	// unless the effect has a Notice of its own. A failure shows "Failed: " and its first line.
	Copy string
	// Screen, when set, opens a full-screen view.
	Screen Screen
	// Edit, when set, opens the user's editor, and the host does nothing else with this effect:
	// Command, Copy, Refresh, RefreshRow and Screen are ignored. Choice and Confirm are still asked
	// first, and Notice stays on the status line while the editor is open.
	Edit *Edit
}

// Edit asks for text in the user's editor: $VISUAL, else $EDITOR, else vi.
type Edit struct {
	// Text is what the editor opens with.
	Text string
	// Done receives the saved text with surrounding white space trimmed. It runs on the update
	// loop and returns what to do next, such as selection.Run(effect) or a screen's own
	// command. It is not called when the editor fails; the board shows "Editor failed: " and the
	// first line of the error instead.
	Done func(text string) tea.Cmd
}

// Screen is a full-screen view above the board. While any screen is open it receives every key
// except ctrl+c, a CommandFinished whenever a command started by an effect finishes, and every
// message the host does not recognise, because screens run their own background commands. A
// screen must ignore messages that are not its own.
type Screen interface {
	Init() tea.Cmd
	// Update returns a nil Screen to close itself.
	Update(message tea.Msg) (Screen, tea.Cmd)
	View(width, height int) string
}

// CommandFinished is delivered to every open screen when a command that an effect started has
// finished, after the host has shown its outcome. A screen can use it to show what the command
// changed, because nothing else tells it. Err is the command's failure, or nil.
type CommandFinished struct{ Err error }

// FacetOption is one choice of a facet.
type FacetOption struct {
	Value string
	Label string
}

// Facet is a way of narrowing the rows of a board, shown in the filter menu.
type Facet struct {
	Title   string
	Options []FacetOption
	// OptionsFromRows adds an option for every other value that the loaded rows have, after
	// the fixed Options, sorted and labelled by value. The options are rebuilt after each load,
	// and one the rows no longer have disappears from the menu but keeps its selection in case
	// it returns. A new option is selected unless Selected names the initial selection. It
	// calls Value for every row on the update loop, so Value must return quickly. It cannot be
	// combined with Single.
	OptionsFromRows bool
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

// displayTitle is the title that tells a board apart from every other, which is the title of
// its tab followed by its own when it shares a tab.
func (board Board) displayTitle() string {
	if board.Tab == "" {
		return board.Title
	}
	return board.Tab + " · " + board.Title
}

// Board is a table of rows with its own columns, actions and facets.
type Board struct {
	// Name is what other plugins pass to AddColumn and AddAction.
	Name string
	// Title labels the board: it is the title of the board's tab, or of its pane when the board
	// shares a tab.
	Title string
	// Tab, when set, groups this board with the other boards of the same Tab into one tab with
	// that title, drawn side by side in registration order. The tab sits where its first board
	// was registered. Boards without a Tab get a tab of their own, titled by Title.
	Tab string
	// RefreshInterval, when set, is how often this board reloads instead of the configured
	// refresh_interval, unless Busy says there is nothing in progress. A board whose tab is not
	// shown never reloads faster than the configured refresh_interval.
	RefreshInterval time.Duration
	// Busy is optional. When it is set and reports false for the loaded rows, the board
	// refreshes at the configured refresh_interval instead of its RefreshInterval, so a board
	// that only needs to be quick while something is in progress does not stay quick. It runs on
	// the update loop and must return quickly.
	Busy func(rows []Row) bool
	// RowNoun says what the rows are, such as "open pull requests", for the status line.
	RowNoun string
	// Load fetches the rows. It runs on a background goroutine. State that it shares with a
	// function that runs on the update loop, such as Cell or Action.Run, needs a lock.
	Load func() ([]Row, error)
	// Reload is optional. It returns the current version of one row, or nil when the row no
	// longer belongs on the board. It runs on a background goroutine, and it is what an
	// Effect.RefreshRow uses instead of Load.
	Reload  func(row Row) (Row, error)
	Columns []Column
	Actions []Action
	Facets  []Facet
}
