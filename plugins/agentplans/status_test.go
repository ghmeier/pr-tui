package agentplans

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

func activePlan() doc {
	return doc{file: "plan/active.md", kind: "plan", title: "Active plan", status: "active", updated: "2026-11-12"}
}

// callsTo are the calls of one apl subcommand.
func (apl *fakeApl) callsTo(subcommand string) []call {
	apl.mutex.Lock()
	defer apl.mutex.Unlock()
	var matching []call
	for _, received := range apl.calls {
		if received.arguments[1] == subcommand {
			matching = append(matching, received)
		}
	}
	return matching
}

func (apl *fakeApl) setCalls() []call { return apl.callsTo("set") }

func (apl *fakeApl) loadCount() int { return len(apl.callsTo("ls")) }

// statusText is the line under the board's table, where choices and notices are shown.
func statusText(model *plugin.Model) string {
	lines := screenLines(model)
	return lines[len(lines)-2]
}

func TestRowStatusesComeFromAplTypesForTheDocumentsType(t *testing.T) {
	apl := installFakeApl(t, false)
	path := apl.newCheckout(t, "storefront",
		doc{file: "plan/p.md", kind: "plan", updated: "2026-11-12"},
		doc{file: "handoff/h.md", kind: "handoff", updated: "2026-11-11"})

	rows, err := checkout{name: "storefront", path: path}.rows()

	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows[0].Statuses, []string{"draft", "active", "completed", "archived"}) ||
		!reflect.DeepEqual(rows[1].Statuses, []string{"open", "picked-up", "closed"}) {
		t.Fatalf("statuses = %v and %v", rows[0].Statuses, rows[1].Statuses)
	}
	expected := call{directory: path, arguments: []string{"apl", "types", "--json"}}
	if len(apl.calls) != 2 || !reflect.DeepEqual(apl.calls[1], expected) {
		t.Fatalf("calls = %v, expected apl types to run once after apl ls in %s", apl.calls, path)
	}
}

func TestRowStatusesAreEmptyForATypeThatAplDoesNotList(t *testing.T) {
	apl := installFakeApl(t, false)
	apl.types = []byte(`{"types":[{"name":"handoff","statuses":["open"]}]}`)
	path := apl.newCheckout(t, "storefront", doc{file: "plan/p.md", kind: "plan", updated: "2026-11-12"})

	rows, err := checkout{name: "storefront", path: path}.rows()

	if err != nil || len(rows[0].Statuses) != 0 {
		t.Fatalf("statuses = %v, error = %v", rows[0].Statuses, err)
	}
}

func TestLoadFailsWhenAplTypesCannotBeRead(t *testing.T) {
	apl := installFakeApl(t, false)
	apl.types = []byte("not json")
	path := apl.newCheckout(t, "storefront", activePlan())

	err := hostFor(t, path).PrintOnce(&strings.Builder{}, 120)

	if err == nil || !strings.Contains(err.Error(), "storefront") || !strings.Contains(err.Error(), "apl types") {
		t.Fatalf("error = %v", err)
	}
}

func TestStatusKeyAsksForTheStatusesOfTheDocumentsType(t *testing.T) {
	apl := installFakeApl(t, false)
	model := startedModel(t, apl.newCheckout(t, "storefront", activePlan()))

	plugintest.Press(t, model, "s")

	expected := "Status of plan/active.md  1 draft  2 active  3 completed  4 archived  esc cancel"
	if status := statusText(model); status != expected {
		t.Fatalf("status line = %q, expected %q", status, expected)
	}
}

func TestPickingAnotherStatusRunsAplSetInTheDocumentsCheckoutAndRefreshes(t *testing.T) {
	apl := installFakeApl(t, false)
	storefront := apl.newCheckout(t, "storefront", activePlan())
	warehouse := apl.newCheckout(t, "warehouse", doc{file: "plan/other.md", kind: "plan", title: "Warehouse plan", status: "draft", updated: "2026-11-01"})
	model := startedModel(t, storefront, warehouse)
	loadsBefore := apl.loadCount()

	plugintest.Press(t, model, "s", "3")

	expected := []call{{directory: storefront, arguments: []string{"apl", "set", "plan/active.md", "--status", "completed"}}}
	if !reflect.DeepEqual(apl.setCalls(), expected) {
		t.Fatalf("set calls = %v, expected %v", apl.setCalls(), expected)
	}
	if apl.loadCount() <= loadsBefore {
		t.Fatalf("the board was not reloaded")
	}
}

func TestPickingTheStatusInTheOtherCheckoutRunsInThatCheckout(t *testing.T) {
	apl := installFakeApl(t, false)
	storefront := apl.newCheckout(t, "storefront", activePlan())
	warehouse := apl.newCheckout(t, "warehouse", doc{file: "plan/other.md", kind: "plan", title: "Warehouse plan", status: "draft", updated: "2026-11-01"})
	model := startedModel(t, storefront, warehouse)

	plugintest.Press(t, model, "down", "s", "2")

	expected := []call{{directory: warehouse, arguments: []string{"apl", "set", "plan/other.md", "--status", "active"}}}
	if !reflect.DeepEqual(apl.setCalls(), expected) {
		t.Fatalf("set calls = %v, expected %v", apl.setCalls(), expected)
	}
}

func TestPickingTheCurrentStatusRunsNothingAndSaysSo(t *testing.T) {
	apl := installFakeApl(t, false)
	example := activePlan()
	model := startedModel(t, apl.newCheckout(t, "storefront", example))
	loadsBefore := apl.loadCount()

	plugintest.Press(t, model, "s", "2")

	if len(apl.setCalls()) != 0 || apl.loadCount() != loadsBefore {
		t.Fatalf("set calls = %v, reloads = %d", apl.setCalls(), apl.loadCount()-loadsBefore)
	}
	if status := statusText(model); !strings.Contains(status, example.file) || !strings.Contains(status, "already has the status "+example.status) {
		t.Fatalf("status line = %q", status)
	}
}

func TestStatusKeyOnATypeWithoutStatusesShowsANoticeAndRunsNothing(t *testing.T) {
	apl := installFakeApl(t, false)
	apl.types = []byte(`{"types":[{"name":"plan","statuses":[]}]}`)
	model := startedModel(t, apl.newCheckout(t, "storefront", activePlan()))

	plugintest.Press(t, model, "s", "1")

	if status := statusText(model); !strings.Contains(status, "plan docs have no statuses to choose from") {
		t.Fatalf("status line = %q", status)
	}
	if len(apl.setCalls()) != 0 {
		t.Fatalf("set calls = %v", apl.setCalls())
	}
}

func TestStatusChangeWithAnAplThatHasNoSetCommandSaysItNeedsANewerApl(t *testing.T) {
	apl := installFakeApl(t, false)
	apl.setError = errors.New("error: unknown command 'set'")
	model := startedModel(t, apl.newCheckout(t, "storefront", activePlan()))

	plugintest.Press(t, model, "s", "1")

	status := statusText(model)
	if !strings.Contains(status, "Failed: changing a status needs apl 0.9.0 or later") {
		t.Fatalf("status line = %q", status)
	}
}

func TestStatusChangeShowsAnyOtherAplFailureAsItIs(t *testing.T) {
	apl := installFakeApl(t, false)
	apl.setError = errors.New("Invalid status")
	model := startedModel(t, apl.newCheckout(t, "storefront", activePlan()))

	plugintest.Press(t, model, "s", "1")

	status := statusText(model)
	if !strings.Contains(status, "Failed: Invalid status") || strings.Contains(status, "0.9.0") {
		t.Fatalf("status line = %q", status)
	}
}

func TestStatusKeyOnTheDocumentScreenAsksTheSameChoiceAndRunsAplSet(t *testing.T) {
	apl := installFakeApl(t, false)
	checkoutPath := apl.newCheckout(t, "storefront", activePlan())
	model := startedModel(t, checkoutPath)
	plugintest.Press(t, model, "enter")

	plugintest.Press(t, model, "s")
	asked := screenLines(model)
	plugintest.Press(t, model, "1")

	if last := asked[len(asked)-1]; !strings.Contains(last, "Status of plan/active.md  1 draft  2 active") {
		t.Fatalf("last line = %q", last)
	}
	expected := []call{{directory: checkoutPath, arguments: []string{"apl", "set", "plan/active.md", "--status", "draft"}}}
	if !reflect.DeepEqual(apl.setCalls(), expected) {
		t.Fatalf("set calls = %v, expected %v", apl.setCalls(), expected)
	}
}

func TestDocumentScreenHelpListsTheStatusKeyBeforeBack(t *testing.T) {
	apl := installFakeApl(t, false)
	model := startedModel(t, apl.newCheckout(t, "storefront", activePlan()))
	plugintest.Press(t, model, "enter")

	help := screenLines(model)
	last := help[len(help)-1]

	if !strings.Contains(last, "s status") || strings.Index(last, "s status") > strings.Index(last, "esc back") {
		t.Fatalf("help line = %q", last)
	}
}

func TestStatusKeyIsListedInTheBoardsHelp(t *testing.T) {
	apl := installFakeApl(t, false)
	model := startedModel(t, apl.newCheckout(t, "storefront", activePlan()))

	lines := screenLines(model)

	if help := lines[len(lines)-1]; !strings.Contains(help, "s status") {
		t.Fatalf("help line = %q", help)
	}
}
