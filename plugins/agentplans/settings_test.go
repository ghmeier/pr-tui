package agentplans

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
)

// checkoutIn makes a directory named name with an .apl directory inside the given parent.
func checkoutIn(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(path, ".apl"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadSettingsFor(t *testing.T, repositories ...string) (settings, error) {
	t.Helper()
	quoted := make([]string, len(repositories))
	for index, repository := range repositories {
		quoted[index] = fmt.Sprintf("%q", repository)
	}
	text := "[agent_plans]\nrepositories = [" + strings.Join(quoted, ", ") + "]\n"
	return loadSettings(configtest.Load(t, text).Section)
}

func assertRejected(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error naming %v", fragments)
	}
	for _, fragment := range append([]string{"[agent_plans] repositories"}, fragments...) {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not mention %q", err, fragment)
		}
	}
}

func TestSettingsReadEachCheckoutWithItsBaseNameAsItsName(t *testing.T) {
	parent := t.TempDir()
	first, second := checkoutIn(t, parent, "storefront"), checkoutIn(t, parent, "warehouse")

	loaded, err := loadSettingsFor(t, first, second)

	expected := []checkout{{name: "storefront", path: first}, {name: "warehouse", path: second}}
	if err != nil || fmt.Sprint(loaded.checkouts) != fmt.Sprint(expected) {
		t.Fatalf("checkouts = %v, error = %v", loaded.checkouts, err)
	}
}

func TestSettingsExpandALeadingTildeInACheckoutPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := checkoutIn(t, home, "storefront")

	loaded, err := loadSettingsFor(t, "~/storefront")

	if err != nil || len(loaded.checkouts) != 1 || loaded.checkouts[0].path != path {
		t.Fatalf("checkouts = %v, error = %v", loaded.checkouts, err)
	}
}

func TestSettingsRejectAMissingTable(t *testing.T) {
	_, err := loadSettings(configtest.Load(t, "").Section)

	assertRejected(t, err, "at least one")
}

func TestSettingsRejectAnEmptyList(t *testing.T) {
	_, err := loadSettingsFor(t)

	assertRejected(t, err, "at least one")
}

func TestSettingsRejectAPathThatDoesNotExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	_, err := loadSettingsFor(t, missing)

	assertRejected(t, err, missing)
}

func TestSettingsRejectADirectoryWithoutAnAplDirectory(t *testing.T) {
	bare := t.TempDir()

	_, err := loadSettingsFor(t, bare)

	assertRejected(t, err, bare, ".apl")
}

func TestSettingsAcceptAnAplDirectoryThatIsASymbolicLink(t *testing.T) {
	parent := t.TempDir()
	main := checkoutIn(t, parent, "storefront")
	worktree := filepath.Join(parent, "storefront.feature")
	if err := os.Mkdir(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(main, ".apl"), filepath.Join(worktree, ".apl")); err != nil {
		t.Fatal(err)
	}

	_, err := loadSettingsFor(t, worktree)

	if err != nil {
		t.Fatal(err)
	}
}

func TestSettingsRejectTwoCheckoutsWithTheSameBaseName(t *testing.T) {
	first, second := checkoutIn(t, t.TempDir(), "storefront"), checkoutIn(t, t.TempDir(), "storefront")

	_, err := loadSettingsFor(t, first, second)

	assertRejected(t, err, first, second, `"storefront"`)
}

func TestSettingsRejectAnUnknownKey(t *testing.T) {
	_, err := loadSettings(configtest.Load(t, "[agent_plans]\nrepos = []\n").Section)

	if err == nil || !strings.Contains(err.Error(), "repos") {
		t.Fatalf("error = %v", err)
	}
}
