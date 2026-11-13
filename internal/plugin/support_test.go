package plugin

import (
	"errors"
	"flag"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

var errorForTests = errors.New("cannot do that")

// fakeRow is a row of the fake boards. Its group is what the fake facets filter on.
type fakeRow struct {
	key   string
	name  string
	group string
}

func (row fakeRow) Key() string { return row.key }

func fruit(key, name string) fakeRow     { return fakeRow{key: key, name: name, group: "fruit"} }
func vegetable(key, name string) fakeRow { return fakeRow{key: key, name: name, group: "vegetable"} }

// groceries are listed with the fruit first, so a facet on the group changes what is shown.
func groceries() []Row {
	return []Row{fruit("apple-key", "apple"), vegetable("broccoli-key", "broccoli"), fruit("cherry-key", "cherry")}
}

// rowSource is where a fake board's rows come from. A test changes them between refreshes.
type rowSource struct {
	mutex sync.Mutex
	rows  []Row
	err   error
	loads int
}

func newRowSource(rows ...Row) *rowSource { return &rowSource{rows: rows} }

func (source *rowSource) set(rows ...Row) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	source.rows = rows
}

func (source *rowSource) fail(err error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	source.err = err
}

func (source *rowSource) loadCount() int {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	return source.loads
}

func (source *rowSource) load() ([]Row, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	source.loads++
	return source.rows, source.err
}

// doNothing is an action that has no effect.
func doNothing(Selection) Effect { return Effect{} }

func plainCell(text string) []ui.Segment {
	return []ui.Segment{{Text: text, Style: ui.PlainStyle}}
}

func nameColumn() Column {
	return Column{Header: "Name", Flexible: true, Cell: func(row Row, _ any) []ui.Segment {
		return plainCell(row.(fakeRow).name)
	}}
}

func groupColumn() Column {
	return Column{Header: "Group", Cell: func(row Row, _ any) []ui.Segment {
		return plainCell(row.(fakeRow).group)
	}}
}

// stockColumn shows "…" until a fetch has delivered a value, then the value.
func stockColumn(load func(rows []Row) []Fetch) Column {
	return Column{
		Header: "Stock",
		Cell: func(_ Row, data any) []ui.Segment {
			if data == nil {
				return plainCell("…")
			}
			return plainCell(data.(string))
		},
		Load: load,
	}
}

func groupFacet() Facet {
	return Facet{
		Title: "Groups",
		Options: []FacetOption{
			{Value: "fruit", Label: "fruit"},
			{Value: "vegetable", Label: "vegetable"},
		},
		Value: func(row Row) string { return row.(fakeRow).group },
	}
}

// sizeFacet is a single facet with a cycle key, over a property the rows do not have, so its
// option values are matched against a constant.
func sizeFacet(cycleKey string) Facet {
	return Facet{
		Title:    "Sizes",
		Options:  []FacetOption{{Value: "small", Label: "small"}, {Value: "large", Label: "large"}},
		Value:    func(row Row) string { return sizeOf(row.(fakeRow)) },
		Single:   true,
		CycleKey: cycleKey,
	}
}

// sizeOf calls a name of more than five letters large.
func sizeOf(row fakeRow) string {
	if len(row.name) > 5 {
		return "large"
	}
	return "small"
}

// groceryBoard is a board of the given rows with a name and a group column and a group facet.
func groceryBoard(source *rowSource) Board {
	return Board{
		Name:    "groceries",
		Title:   "Groceries",
		RowNoun: "groceries",
		Load:    source.load,
		Columns: []Column{nameColumn(), groupColumn()},
		Facets:  []Facet{groupFacet()},
	}
}

// fakePlugin registers what its setup function registers.
type fakePlugin struct {
	name  string
	flags func(flags *flag.FlagSet)
	setup func(host *Host) error
}

func (plugin fakePlugin) Name() string { return plugin.name }

func (plugin fakePlugin) Flags(flags *flag.FlagSet) {
	if plugin.flags != nil {
		plugin.flags(flags)
	}
}

func (plugin fakePlugin) Setup(host *Host) error {
	if plugin.setup == nil {
		return nil
	}
	return plugin.setup(host)
}

func boardPlugin(name string, board Board) fakePlugin {
	return fakePlugin{name: name, setup: func(host *Host) error {
		host.AddBoard(board)
		return nil
	}}
}

// buildHost loads the configuration text and sets up the plugins it enables.
func buildHost(t *testing.T, text string, plugins ...Plugin) (*Host, error) {
	t.Helper()
	file := configtest.Load(t, text)
	return NewHost("config.toml", file, plugins)
}

// modelFor builds the model of a host that enables every given plugin, and loads it.
func modelFor(t *testing.T, plugins ...Plugin) *Model {
	t.Helper()
	names := make([]string, len(plugins))
	for index, plugin := range plugins {
		names[index] = `"` + plugin.Name() + `"`
	}
	host, err := buildHost(t, "plugins = ["+strings.Join(names, ", ")+"]\n", plugins...)
	if err != nil {
		t.Fatal(err)
	}
	model := host.Model()
	plugintest.Start(t, model)
	return model
}

func modelForBoard(t *testing.T, board Board) *Model {
	t.Helper()
	return modelFor(t, boardPlugin("fake", board))
}

func screenText(model *Model) string {
	return ansi.Strip(model.View())
}

func lines(model *Model) []string {
	return strings.Split(screenText(model), "\n")
}

// tableLines are the lines of the view from the table's header on, leaving out the summary of
// the filters above it.
func tableLines(model *Model) []string {
	return lines(model)[1:]
}

func selectedName(t *testing.T, model *Model) string {
	t.Helper()
	row, found := model.current().selected()
	if !found {
		t.Fatalf("no row is selected")
	}
	return row.(fakeRow).name
}

func visibleNames(model *Model) []string {
	var names []string
	for _, row := range model.current().visible {
		names = append(names, row.(fakeRow).name)
	}
	return names
}

func namesOf(rows ...Row) []string {
	var names []string
	for _, row := range rows {
		names = append(names, row.(fakeRow).name)
	}
	return names
}

// fakeScreen records what it receives and closes on esc.
type fakeScreen struct {
	title    string
	received []tea.Msg
	width    int
	height   int
	// onKey lets a test make a key do more than record itself.
	onKey func(key string) (Screen, tea.Cmd, bool)
	// init is what the screen starts when it opens.
	init tea.Cmd
}

type screenMessage struct{ target string }

func (screen *fakeScreen) Init() tea.Cmd { return screen.init }

func (screen *fakeScreen) Update(message tea.Msg) (Screen, tea.Cmd) {
	if key, isKey := message.(tea.KeyMsg); isKey {
		if screen.onKey != nil {
			if next, command, handled := screen.onKey(key.String()); handled {
				return next, command
			}
		}
		if key.String() == "esc" {
			return nil, nil
		}
	}
	screen.received = append(screen.received, message)
	return screen, nil
}

func (screen *fakeScreen) View(width, height int) string {
	screen.width, screen.height = width, height
	return "screen " + screen.title
}

func (screen *fakeScreen) receivedKeys() []string {
	var keys []string
	for _, message := range screen.received {
		if key, isKey := message.(tea.KeyMsg); isKey {
			keys = append(keys, key.String())
		}
	}
	return keys
}
