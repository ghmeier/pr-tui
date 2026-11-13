package plugin

import (
	"flag"
	"reflect"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config"
	"github.com/ghmeier/pr-tui/internal/config/configtest"
)

// assertRejectedNamingTheFileOnce checks that building failed, that the message names the
// configuration file exactly once, and that it contains every one of the parts.
func assertRejectedNamingTheFileOnce(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error containing %q", parts)
	}
	if strings.Count(err.Error(), "config.toml") != 1 {
		t.Fatalf("error %q should name the file exactly once", err)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("error %q should contain %q", err, part)
		}
	}
}

// recordingPlugin notes that it was set up, in a shared list, so that tests can check which
// plugins ran and in which order.
func recordingPlugin(name string, order *[]string) fakePlugin {
	return fakePlugin{name: name, setup: func(*Host) error {
		*order = append(*order, name)
		return nil
	}}
}

func TestNewHostSetsUpOnlyTheEnabledPluginsInTheOrderTheFileListsThem(t *testing.T) {
	var order []string
	first, second, unlisted := recordingPlugin("first", &order), recordingPlugin("second", &order), recordingPlugin("unlisted", &order)

	_, err := buildHost(t, `plugins = ["second", "first"]`, first, second, unlisted)

	if err != nil || !reflect.DeepEqual(order, []string{second.name, first.name}) {
		t.Fatalf("set up %v, err = %v", order, err)
	}
}

func TestNewHostRejectsAnUnknownPluginAndListsTheRegisteredOnes(t *testing.T) {
	var order []string
	first, second := recordingPlugin("first", &order), recordingPlugin("second", &order)

	_, err := buildHost(t, `plugins = ["missing"]`, first, second)

	assertRejectedNamingTheFileOnce(t, err, `"missing"`, first.name, second.name)
}

func TestNewHostRejectsAPluginListedTwice(t *testing.T) {
	var order []string
	only := recordingPlugin("only", &order)

	_, err := buildHost(t, `plugins = ["only", "only"]`, only)

	assertRejectedNamingTheFileOnce(t, err, `"only"`)
	if len(order) != 1 {
		t.Fatalf("the plugin was set up %d times", len(order))
	}
}

func TestRegisterFlagsAsksEveryPluginIncludingThoseThatAreNotEnabled(t *testing.T) {
	var registered []string
	flagPlugin := func(name string) fakePlugin {
		return fakePlugin{name: name, flags: func(flags *flag.FlagSet) {
			flags.Bool(name, false, "")
			registered = append(registered, name)
		}}
	}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)

	RegisterFlags([]Plugin{flagPlugin("enabled"), flagPlugin("disabled")}, flags)

	if !reflect.DeepEqual(registered, []string{"enabled", "disabled"}) || flags.Lookup("disabled") == nil {
		t.Fatalf("flags registered by %v", registered)
	}
}

func TestRegisterRefusesAnotherPluginUnderTheSameName(t *testing.T) {
	original := registered
	t.Cleanup(func() { registered = original })
	registered = nil
	Register(fakePlugin{name: "twice"})

	defer func() {
		if recover() == nil {
			t.Fatal("registering a name twice should panic")
		}
	}()
	Register(fakePlugin{name: "twice"})
}

func TestRegisteredReturnsThePluginsInRegistrationOrder(t *testing.T) {
	original := registered
	t.Cleanup(func() { registered = original })
	registered = nil
	Register(fakePlugin{name: "first"})
	Register(fakePlugin{name: "second"})

	names := []string{}
	for _, plugin := range Registered() {
		names = append(names, plugin.Name())
	}

	if !reflect.DeepEqual(names, []string{"first", "second"}) {
		t.Fatalf("registered = %v", names)
	}
}

func TestSectionDecodesThePluginsTableAndRejectsAKeyItDoesNotDeclare(t *testing.T) {
	type settings struct {
		Fruit string `toml:"fruit"`
	}
	read := func(text string) (settings, error) {
		var result settings
		plugin := fakePlugin{name: "fake", setup: func(host *Host) error { return host.Section("fake", &result) }}
		_, err := buildHost(t, "plugins = [\"fake\"]\n"+text, plugin)
		return result, err
	}

	decoded, decodeErr := read("[fake]\nfruit = \"apple\"\n")
	_, misspelt := read("[fake]\nfruti = \"apple\"\n")

	if decodeErr != nil || decoded.Fruit != "apple" {
		t.Fatalf("decoded = %+v, err = %v", decoded, decodeErr)
	}
	assertRejectedNamingTheFileOnce(t, misspelt, "[fake]", "fruti")
}

func TestNewHostAcceptsTheEditorTableAndTheTablesOfRegisteredPlugins(t *testing.T) {
	enabled := fakePlugin{name: "enabled"}
	disabled := fakePlugin{name: "disabled"}

	_, err := buildHost(t, "plugins = [\"enabled\"]\n[editor]\nname = \"Zed\"\n[enabled]\n[disabled]\n", enabled, disabled)

	if err != nil {
		t.Fatal(err)
	}
}

func TestNewHostAcceptsATableThatSomePluginAskedForThroughSection(t *testing.T) {
	asking := fakePlugin{name: "asking", setup: func(host *Host) error {
		var ignored struct{}
		return host.Section("backend", &ignored)
	}}

	_, err := buildHost(t, "plugins = [\"asking\"]\n[backend]\n", asking)

	if err != nil {
		t.Fatal(err)
	}
}

func TestNewHostRejectsATableNoPluginReads(t *testing.T) {
	asking := fakePlugin{name: "asking"}

	_, err := buildHost(t, "plugins = [\"asking\"]\n[stranger]\n", asking)

	assertRejectedNamingTheFileOnce(t, err, "[stranger]", "editor", asking.name)
}

func TestNewHostNamesTheFileOnceForAFailureOfAPluginsSetup(t *testing.T) {
	failing := fakePlugin{name: "failing", setup: func(*Host) error { return errorForTests }}

	_, err := buildHost(t, `plugins = ["failing"]`, failing)

	assertRejectedNamingTheFileOnce(t, err, errorForTests.Error())
}

func TestNewHostDoesNotNameTheFileForAMistakeOnTheCommandLine(t *testing.T) {
	failing := fakePlugin{name: "failing", setup: func(*Host) error { return UsageError(errorForTests) }}

	_, err := buildHost(t, `plugins = ["failing"]`, failing)

	if err == nil || err.Error() != errorForTests.Error() {
		t.Fatalf("error = %v, expected the plugin's message alone", err)
	}
}

func TestHostReportsTheEditorOfTheFile(t *testing.T) {
	file := configtest.Load(t, "[editor]\nname = \"Zed\"\ncommand = \"zed\"\n")
	var editor config.Editor
	reading := fakePlugin{name: "pull_requests", setup: func(host *Host) error {
		editor = host.Editor()
		return nil
	}}

	_, err := NewHost("config.toml", file, []Plugin{reading})

	if err != nil || !reflect.DeepEqual(editor, file.Editor) {
		t.Fatalf("editor = %+v, err = %v", editor, err)
	}
}

func TestAddColumnReachesABoardOfAPluginSetUpLater(t *testing.T) {
	stock := stockColumn(nil)
	adder := fakePlugin{name: "adder", setup: func(host *Host) error {
		host.AddColumn("groceries", stock)
		return nil
	}}
	owner := boardPlugin("owner", groceryBoard(newRowSource(groceries()...)))

	model := modelFor(t, adder, owner)

	headers := []string{}
	for _, column := range model.current().spec.Columns {
		headers = append(headers, column.Header)
	}
	if expected := []string{"Name", "Group", stock.Header}; !reflect.DeepEqual(headers, expected) {
		t.Fatalf("headers = %v, expected %v", headers, expected)
	}
}

func TestAddColumnAppendsAfterTheBoardsOwnColumnsInPluginOrder(t *testing.T) {
	first, second := stockColumn(nil), stockColumn(nil)
	first.Header, second.Header = "First", "Second"
	addColumn := func(name string, column Column) fakePlugin {
		return fakePlugin{name: name, setup: func(host *Host) error {
			host.AddColumn("groceries", column)
			return nil
		}}
	}

	model := modelFor(t, boardPlugin("owner", groceryBoard(newRowSource(groceries()...))), addColumn("one", first), addColumn("two", second))

	headers := []string{}
	for _, column := range model.current().spec.Columns {
		headers = append(headers, column.Header)
	}
	if expected := []string{"Name", "Group", first.Header, second.Header}; !reflect.DeepEqual(headers, expected) {
		t.Fatalf("headers = %v, expected %v", headers, expected)
	}
}

func TestAddColumnToABoardThatDoesNotExistNamesThePluginAndTheBoard(t *testing.T) {
	adder := fakePlugin{name: "adder", setup: func(host *Host) error {
		host.AddColumn("missing-board", stockColumn(nil))
		return nil
	}}

	_, err := buildHost(t, `plugins = ["adder"]`, adder)

	assertRejectedNamingTheFileOnce(t, err, `"adder"`, `"missing-board"`)
}

func TestAddActionToABoardThatDoesNotExistNamesThePluginAndTheBoard(t *testing.T) {
	adder := fakePlugin{name: "adder", setup: func(host *Host) error {
		host.AddAction("missing-board", Action{Key: "x"})
		return nil
	}}

	_, err := buildHost(t, `plugins = ["adder"]`, adder)

	assertRejectedNamingTheFileOnce(t, err, `"adder"`, `"missing-board"`)
}

func TestAddActionToABoardOfAPluginThatIsNotEnabledIsAnError(t *testing.T) {
	adder := fakePlugin{name: "adder", setup: func(host *Host) error {
		host.AddAction("groceries", Action{Key: "x"})
		return nil
	}}
	owner := boardPlugin("owner", groceryBoard(newRowSource()))

	_, err := buildHost(t, `plugins = ["adder"]`, adder, owner)

	assertRejectedNamingTheFileOnce(t, err, `"adder"`, `"groceries"`)
}

func TestTwoActionsOnOneBoardCannotShareAKey(t *testing.T) {
	board := groceryBoard(newRowSource())
	board.Actions = []Action{{Key: "x"}}
	adder := fakePlugin{name: "adder", setup: func(host *Host) error {
		host.AddAction("groceries", Action{Key: "x"})
		return nil
	}}

	_, err := buildHost(t, `plugins = ["owner", "adder"]`, boardPlugin("owner", board), adder)

	assertRejectedNamingTheFileOnce(t, err, `"x"`)
}

func TestActionsOnDifferentBoardsMayShareAKey(t *testing.T) {
	first, second := groceryBoard(newRowSource()), groceryBoard(newRowSource())
	second.Name = "other"
	first.Actions, second.Actions = []Action{{Key: "x"}}, []Action{{Key: "x"}}

	_, err := buildHost(t, `plugins = ["one", "two"]`, boardPlugin("one", first), boardPlugin("two", second))

	if err != nil {
		t.Fatal(err)
	}
}

func TestAnActionCannotClaimAKeyTheHostUses(t *testing.T) {
	for _, key := range reservedKeys {
		board := groceryBoard(newRowSource())
		board.Actions = []Action{{Key: key}}

		_, err := buildHost(t, `plugins = ["owner"]`, boardPlugin("owner", board))

		assertRejectedNamingTheFileOnce(t, err, `"`+key+`"`)
	}
}

func TestABoardMayHaveAtMostOneFlexibleColumn(t *testing.T) {
	board := groceryBoard(newRowSource())
	other := groupColumn()
	other.Flexible = true
	board.Columns = append(board.Columns, other)

	_, err := buildHost(t, `plugins = ["owner"]`, boardPlugin("owner", board))

	assertRejectedNamingTheFileOnce(t, err, "flexible")
}

func TestAFacetCannotCycleOnAKeyTheFilterMenuUses(t *testing.T) {
	for _, key := range []string{"f", "j", "1", " "} {
		board := groceryBoard(newRowSource())
		board.Facets = []Facet{sizeFacet(key)}

		_, err := buildHost(t, `plugins = ["owner"]`, boardPlugin("owner", board))

		assertRejectedNamingTheFileOnce(t, err, "filter menu")
	}
}

func TestTwoBoardsCannotShareAName(t *testing.T) {
	first, second := boardPlugin("one", groceryBoard(newRowSource())), boardPlugin("two", groceryBoard(newRowSource()))

	_, err := buildHost(t, `plugins = ["one", "two"]`, first, second)

	assertRejectedNamingTheFileOnce(t, err, `"groceries"`, first.name, second.name)
}
