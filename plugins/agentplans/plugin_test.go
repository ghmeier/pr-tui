package agentplans

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/shell"
)

// doc is what a test says about one document: apl's listing of it and the text of its file.
type doc struct {
	file, kind, title, status, updated, body string
	// branches are the branches the document is linked to.
	branches []string
}

// fakeApl stands in for the apl program: each checkout has a canned listing, and the calls it
// received are recorded.
type fakeApl struct {
	mutex    sync.Mutex
	listings map[string][]byte
	calls    []call
}

type call struct {
	directory string
	arguments []string
}

// newCheckout makes a checkout directory holding the documents' files and teaches the fake apl
// to list them.
func (apl *fakeApl) newCheckout(t *testing.T, name string, docs ...doc) string {
	t.Helper()
	path := checkoutIn(t, t.TempDir(), name)
	var files []map[string]any
	for _, example := range docs {
		file := filepath.Join(path, ".apl", example.file)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(example.body), 0o644); err != nil {
			t.Fatal(err)
		}
		meta := map[string]any{"title": example.title, "tags": []string{}, "updated": example.updated}
		if example.status != "" {
			meta["status"] = example.status
		}
		links := []map[string]any{}
		for _, branch := range example.branches {
			links = append(links, map[string]any{"branch": branch})
		}
		files = append(files, map[string]any{"file": example.file, "type": example.kind, "meta": meta, "links": links})
	}
	listing, err := json.Marshal(map[string]any{"files": files})
	if err != nil {
		t.Fatal(err)
	}
	apl.listings[path] = listing
	return path
}

// installFakeApl replaces shell.RunIn for the test. With missing set, the program is not found.
func installFakeApl(t *testing.T, missing bool) *fakeApl {
	t.Helper()
	apl := &fakeApl{listings: map[string][]byte{}}
	original := shell.RunIn
	t.Cleanup(func() { shell.RunIn = original })
	shell.RunIn = func(directory string, arguments ...string) ([]byte, error) {
		apl.mutex.Lock()
		defer apl.mutex.Unlock()
		apl.calls = append(apl.calls, call{directory: directory, arguments: arguments})
		if missing {
			return nil, fmt.Errorf("start apl: %w", exec.ErrNotFound)
		}
		return apl.listings[directory], nil
	}
	return apl
}

func hostFor(t *testing.T, paths ...string) *plugin.Host {
	t.Helper()
	return hostWithConfiguration(t, `plugins = ["agent_plans"]`+"\n", []plugin.Plugin{&agentPlansPlugin{}}, paths)
}

// hostWithConfiguration builds a host from the given top-level configuration, followed by the
// editor and [agent_plans] tables for the checkouts at paths.
func hostWithConfiguration(t *testing.T, topLevel string, plugins []plugin.Plugin, paths []string) *plugin.Host {
	t.Helper()
	quoted := make([]string, len(paths))
	for index, path := range paths {
		quoted[index] = fmt.Sprintf("%q", path)
	}
	text := topLevel + "[editor]\nname = \"Test Editor\"\ncommand = [\"test-editor\", \"--wait\"]\n[agent_plans]\nrepositories = [" + strings.Join(quoted, ", ") + "]\n"
	host, err := plugin.NewHost("config.toml", configtest.Load(t, text), plugins)
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func startedModel(t *testing.T, paths ...string) *plugin.Model {
	t.Helper()
	model := hostFor(t, paths...).Model()
	plugintest.Resize(t, model, 160, 40)
	plugintest.Start(t, model)
	return model
}

func screenLines(model *plugin.Model) []string {
	return strings.Split(ansi.Strip(model.View()), "\n")
}

// lineWith returns the screen line that contains the text, or "".
func lineWith(model *plugin.Model, text string) string {
	for _, line := range screenLines(model) {
		if strings.Contains(line, text) {
			return line
		}
	}
	return ""
}

func lineIndex(model *plugin.Model, text string) int {
	for index, line := range screenLines(model) {
		if strings.Contains(line, text) {
			return index
		}
	}
	return -1
}

func TestRowsOfTwoCheckoutsAreMergedNewestFirst(t *testing.T) {
	apl := installFakeApl(t, false)
	storefront := apl.newCheckout(t, "storefront",
		doc{file: "plan/a.md", kind: "plan", title: "Storefront older plan", updated: "2026-11-01"},
		doc{file: "plan/b.md", kind: "plan", title: "Storefront newest plan", updated: "2026-11-12"})
	warehouse := apl.newCheckout(t, "warehouse",
		doc{file: "plan/c.md", kind: "plan", title: "Warehouse middle plan", updated: "2026-11-05"})

	model := startedModel(t, storefront, warehouse)

	newest, middle, oldest := lineIndex(model, "Storefront newest plan"), lineIndex(model, "Warehouse middle plan"), lineIndex(model, "Storefront older plan")
	if newest < 0 || !(newest < middle && middle < oldest) {
		t.Fatalf("positions newest %d, middle %d, oldest %d:\n%s", newest, middle, oldest, strings.Join(screenLines(model), "\n"))
	}
	if line := lineWith(model, "Warehouse middle plan"); !strings.Contains(line, "warehouse") {
		t.Fatalf("the row does not name its repository: %q", line)
	}
}

func TestPullRequestCellShowsTheNumbersOfTheBodyAndCountsThoseBeyondThree(t *testing.T) {
	apl := installFakeApl(t, false)
	checkout := apl.newCheckout(t, "storefront",
		doc{file: "plan/many.md", kind: "plan", title: "Many pull requests", updated: "2026-11-12", body: "Landed in #40, #50, #30, #20 and #10."},
		doc{file: "plan/few.md", kind: "plan", title: "Two pull requests", updated: "2026-11-11", body: "See #7 and https://example.test/acme/storefront/pull/9."},
		doc{file: "plan/none.md", kind: "plan", title: "No pull requests", updated: "2026-11-10", body: "Nothing yet."})

	model := startedModel(t, checkout)

	if line := lineWith(model, "Many pull requests"); !strings.HasSuffix(strings.TrimSpace(line), "#50 #40 #30 +2") {
		t.Fatalf("line = %q", line)
	}
	if line := lineWith(model, "Two pull requests"); !strings.HasSuffix(strings.TrimSpace(line), "#9 #7") {
		t.Fatalf("line = %q", line)
	}
	if line := lineWith(model, "No pull requests"); strings.Contains(line, "#") {
		t.Fatalf("line = %q", line)
	}
}

func TestADocumentWhoseFileCannotBeReadKeepsItsRowWithoutPullRequests(t *testing.T) {
	apl := installFakeApl(t, false)
	checkout := apl.newCheckout(t, "storefront", doc{file: "plan/gone.md", kind: "plan", title: "Deleted since listing", updated: "2026-11-12", body: "#5"})
	if err := os.Remove(filepath.Join(checkout, ".apl", "plan", "gone.md")); err != nil {
		t.Fatal(err)
	}

	model := startedModel(t, checkout)

	if line := lineWith(model, "Deleted since listing"); line == "" || strings.Contains(line, "#") {
		t.Fatalf("line = %q", line)
	}
}

func typeFacetModel(t *testing.T) *plugin.Model {
	t.Helper()
	apl := installFakeApl(t, false)
	checkout := apl.newCheckout(t, "storefront",
		doc{file: "plan/p.md", kind: "plan", title: "A plan", updated: "2026-11-12"},
		doc{file: "research/r.md", kind: "research", title: "A research note", updated: "2026-11-11"})
	return startedModel(t, checkout)
}

func TestTypeFacetStartsOnPlansOnly(t *testing.T) {
	model := typeFacetModel(t)

	if lineWith(model, "A plan") == "" || lineWith(model, "A research note") != "" {
		t.Fatalf("only the plan should be shown:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestTypeFacetShowsAnotherTypeOnceItIsSelected(t *testing.T) {
	model := typeFacetModel(t)

	// The repository facet has one option, so the type facet's options are numbers 2 and 3.
	plugintest.Press(t, model, "f", "3", "esc")

	if lineWith(model, "A research note") == "" || lineWith(model, "A plan") == "" {
		t.Fatalf("both types should be shown:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestStatusFacetListsDocumentsWithoutAStatusAsNone(t *testing.T) {
	apl := installFakeApl(t, false)
	checkout := apl.newCheckout(t, "storefront",
		doc{file: "plan/p.md", kind: "plan", title: "A plan", status: "active", updated: "2026-11-12"},
		doc{file: "plan/q.md", kind: "plan", title: "Another plan", updated: "2026-11-11"})

	model := startedModel(t, checkout)

	summary := lineWith(model, "Status:")
	if !strings.Contains(summary, "● active") || !strings.Contains(summary, "● none") {
		t.Fatalf("summary = %q", summary)
	}
	if lineWith(model, "A plan") == "" || lineWith(model, "Another plan") == "" {
		t.Fatalf("both plans should be shown:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestLoadFailsWithAnErrorSayingAplIsNotInstalledWhenTheProgramIsMissing(t *testing.T) {
	installFakeApl(t, true)
	checkout := checkoutIn(t, t.TempDir(), "storefront")
	var printed strings.Builder

	err := hostFor(t, checkout).PrintOnce(&printed, 120)

	if err == nil || !strings.Contains(err.Error(), "apl is not installed") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadNamesTheCheckoutWhoseListingFails(t *testing.T) {
	apl := installFakeApl(t, false)
	broken := checkoutIn(t, t.TempDir(), "storefront")
	apl.listings[broken] = []byte("not json")

	err := hostFor(t, broken).PrintOnce(&strings.Builder{}, 120)

	if err == nil || !strings.Contains(err.Error(), "storefront") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRunsAplListInEachCheckoutsDirectory(t *testing.T) {
	apl := installFakeApl(t, false)
	first := apl.newCheckout(t, "storefront")
	second := apl.newCheckout(t, "warehouse")

	startedModel(t, first, second)

	expected := []call{
		{directory: first, arguments: []string{"apl", "ls", "--json"}},
		{directory: second, arguments: []string{"apl", "ls", "--json"}},
	}
	if !reflect.DeepEqual(apl.calls, expected) {
		t.Fatalf("calls = %v, expected %v", apl.calls, expected)
	}
}
