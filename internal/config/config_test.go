package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sectionsText is a pair of tables that belong to other packages.
const sectionsText = `
[pull_requests]
clone_root = "~/code"

[github]
repositories = ["acme/storefront"]
`

// exampleFile holds the values written by fullConfigurationText, so tests compare against
// fixture values rather than retyped literals.
type exampleFile struct {
	plugin          string
	refreshInterval time.Duration
	editor          Editor
}

func newExampleFile() exampleFile {
	return exampleFile{
		plugin:          "calendar",
		refreshInterval: 7 * time.Minute,
		editor:          Editor{Name: "Test Editor", Command: []string{"test-editor", "--reuse-window"}},
	}
}

// fullConfigurationText writes every key this package owns.
func fullConfigurationText(example exampleFile) string {
	var editorCommand []string
	for _, word := range example.editor.Command {
		editorCommand = append(editorCommand, fmt.Sprintf("%q", word))
	}
	return fmt.Sprintf(`
plugins = [%q]
refresh_interval = %q

[editor]
name = %q
command = [%s]
`, example.plugin, example.refreshInterval.String(), example.editor.Name, strings.Join(editorCommand, ", "))
}

// isolateEnvironment keeps the real environment out of path resolution and home
// expansion, and returns the fake home directory.
func isolateEnvironment(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PRS_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func writeConfigurationFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// loadText loads a file with the given text and returns its path too, so that error tests
// can check the message names it.
func loadText(t *testing.T, text string) (*File, string, error) {
	t.Helper()
	path := writeConfigurationFile(t, text)
	file, err := Load(path)
	return file, path, err
}

func mustLoadText(t *testing.T, text string) *File {
	t.Helper()
	file, _, err := loadText(t, text)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

// assertRejected checks that loading failed and the message names the file and the key.
func assertRejected(t *testing.T, text, key string) {
	t.Helper()
	_, path, err := loadText(t, text)
	if err == nil {
		t.Fatalf("expected an error naming %q", key)
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), key) {
		t.Fatalf("error %q should name the file %s and the key %q", err, path, key)
	}
}

func TestLoadReadsEveryKeyItOwns(t *testing.T) {
	example := newExampleFile()

	file := mustLoadText(t, fullConfigurationText(example))

	if !reflect.DeepEqual(file.Plugins, []string{example.plugin}) {
		t.Fatalf("plugins = %v, expected %v", file.Plugins, example.plugin)
	}
	if file.RefreshInterval != example.refreshInterval {
		t.Fatalf("refresh interval = %v, expected %v", file.RefreshInterval, example.refreshInterval)
	}
	if !reflect.DeepEqual(file.Editor, example.editor) {
		t.Fatalf("editor = %+v, expected %+v", file.Editor, example.editor)
	}
}

func TestLoadFillsDefaultsForAnEmptyFile(t *testing.T) {
	file := mustLoadText(t, "")

	if !reflect.DeepEqual(file.Plugins, defaultPlugins) || file.RefreshInterval != defaultRefreshInterval {
		t.Fatalf("plugins = %v, refresh interval = %v", file.Plugins, file.RefreshInterval)
	}
	expectedEditor := Editor{Name: defaultEditorName, Command: defaultEditorCommand}
	if !reflect.DeepEqual(file.Editor, expectedEditor) {
		t.Fatalf("editor = %+v, expected %+v", file.Editor, expectedEditor)
	}
}

func TestLoadRejectsADurationThatDoesNotParse(t *testing.T) {
	assertRejected(t, `refresh_interval = "soon"`, "refresh_interval")
}

func TestLoadRejectsADurationShorterThanOneSecond(t *testing.T) {
	for _, interval := range []string{"0s", "999ms"} {
		assertRejected(t, fmt.Sprintf("refresh_interval = %q", interval), "refresh_interval")
	}
}

func TestLoadAcceptsADurationOfExactlyOneSecond(t *testing.T) {
	file := mustLoadText(t, `refresh_interval = "1s"`)

	if file.RefreshInterval != time.Second {
		t.Fatalf("refresh interval = %v", file.RefreshInterval)
	}
}

func TestLoadReadsAStringEditorCommandAsOneExecutableEvenWithSpaces(t *testing.T) {
	executable := "/Applications/Some Editor.app/Contents/bin/edit"

	file := mustLoadText(t, fmt.Sprintf("[editor]\ncommand = %q\n", executable))

	if !reflect.DeepEqual(file.Editor.Command, []string{executable}) {
		t.Fatalf("editor command = %q", file.Editor.Command)
	}
}

func TestLoadReadsAListEditorCommandAsExecutableThenArguments(t *testing.T) {
	expected := []string{"code", "--reuse-window"}

	file := mustLoadText(t, `[editor]
command = ["code", "--reuse-window"]
`)

	if !reflect.DeepEqual(file.Editor.Command, expected) {
		t.Fatalf("editor command = %q, expected %q", file.Editor.Command, expected)
	}
}

func TestLoadRejectsEditorCommandsThatCannotRun(t *testing.T) {
	rejected := map[string]string{
		"an empty list":                    `[]`,
		"an empty string":                  `""`,
		"an empty first element":           `["", "--wait"]`,
		"a list with a non-string element": `["code", 3]`,
		"a number":                         `3`,
		"a table":                          `{ name = "code" }`,
	}

	for description, command := range rejected {
		t.Run(description, func(t *testing.T) {
			assertRejected(t, "[editor]\ncommand = "+command+"\n", "[editor] command")
		})
	}
}

func TestLoadLeavesJudgingPluginNamesToTheHost(t *testing.T) {
	file := mustLoadText(t, `plugins = ["calendar"]`)

	if !reflect.DeepEqual(file.Plugins, []string{"calendar"}) {
		t.Fatalf("plugins = %v", file.Plugins)
	}
}

func TestLoadRejectsAnUnknownTopLevelKeyAndNamesIt(t *testing.T) {
	assertRejected(t, `refresh_intervall = "3m"`, "refresh_intervall")
}

func TestLoadRejectsAnUnknownKeyInTheEditorTableAndNamesIt(t *testing.T) {
	assertRejected(t, "[editor]\nnaem = \"code\"\n", "editor.naem")
}

func TestLoadRejectsTextThatIsNotTOML(t *testing.T) {
	assertRejected(t, "this is not toml", "")
}

func TestLoadOfAMissingFileSaysToRunInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.toml")

	_, err := Load(path)

	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "prs --init") {
		t.Fatalf("error = %v, expected it to name %s and prs --init", err, path)
	}
}

func TestSectionDecodesTheNamedTable(t *testing.T) {
	file := mustLoadText(t, sectionsText)
	var target struct {
		Repositories []string `toml:"repositories"`
	}

	err := file.Section("github", &target)

	if expected := []string{"acme/storefront"}; err != nil || !reflect.DeepEqual(target.Repositories, expected) {
		t.Fatalf("repositories = %v, err = %v, expected %v", target.Repositories, err, expected)
	}
}

func TestSectionRejectsAnUnknownKeyNamingTheSectionAndTheKey(t *testing.T) {
	file := mustLoadText(t, "[github]\nreview_teem = \"engineering\"\n")
	var target struct {
		ReviewTeam string `toml:"review_team"`
	}

	err := file.Section("github", &target)

	if err == nil {
		t.Fatal("expected an error")
	}
	for _, expected := range []string{"[github]", "review_teem"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error %q should contain %q", err, expected)
		}
	}
}

func TestSectionRejectsAnUnknownKeyNestedInATable(t *testing.T) {
	file := mustLoadText(t, "[github.nested]\nmistake = 1\n")
	var target struct {
		Nested struct{} `toml:"nested"`
	}

	err := file.Section("github", &target)

	if err == nil || !strings.Contains(err.Error(), "nested.mistake") {
		t.Fatalf("error = %v, expected it to name nested.mistake", err)
	}
}

func TestSectionRejectsAValueOfTheWrongTypeNamingTheSection(t *testing.T) {
	file := mustLoadText(t, "[github]\nrepositories = \"not a list\"\n")
	var target struct {
		Repositories []string `toml:"repositories"`
	}

	err := file.Section("github", &target)

	if err == nil || !strings.Contains(err.Error(), "[github]") {
		t.Fatalf("error = %v, expected it to name [github]", err)
	}
}

func TestRejectUnknownSectionsAcceptsTheTablesItIsGiven(t *testing.T) {
	file := mustLoadText(t, "[editor]\nname = \"x\"\n"+sectionsText)

	err := file.RejectUnknownSections("pull_requests", "github")

	if err != nil {
		t.Fatal(err)
	}
}

func TestRejectUnknownSectionsNamesTheSectionNobodyReads(t *testing.T) {
	file := mustLoadText(t, "[editor]\nname = \"x\"\n[gitlab]\n"+sectionsText)

	err := file.RejectUnknownSections("pull_requests", "github")

	if err == nil || !strings.Contains(err.Error(), "[gitlab]") {
		t.Fatalf("error = %v, expected it to name [gitlab]", err)
	}
}

func TestSectionDecodesAMissingTableAsEmpty(t *testing.T) {
	file := mustLoadText(t, "")
	var target struct {
		Repositories []string `toml:"repositories"`
	}

	err := file.Section("github", &target)

	if err != nil || target.Repositories != nil {
		t.Fatalf("repositories = %v, err = %v", target.Repositories, err)
	}
}

func TestWriteStarterCreatesTheDirectoryAndAFileThatLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "prs", "config.toml")

	err := WriteStarter(path)

	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("the starter configuration should load unchanged: %v", err)
	}
}

func TestWriteStarterRefusesToOverwriteAnExistingFile(t *testing.T) {
	existing := "refresh_interval = \"1m\"\n"
	path := writeConfigurationFile(t, existing)

	err := WriteStarter(path)

	contents, _ := os.ReadFile(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v, expected a refusal naming %s", err, path)
	}
	if string(contents) != existing {
		t.Fatalf("the existing file was changed to %q", contents)
	}
}

func TestResolvePathPrefersTheFlagOverTheEnvironment(t *testing.T) {
	isolateEnvironment(t)
	t.Setenv("PRS_CONFIG", "/from/environment.toml")
	t.Setenv("XDG_CONFIG_HOME", "/from/xdg")

	path, err := ResolvePath("/from/flag.toml")

	if err != nil || path != "/from/flag.toml" {
		t.Fatalf("path = %q, err = %v", path, err)
	}
}

func TestResolvePathPrefersPRSConfigOverTheConfigDirectories(t *testing.T) {
	isolateEnvironment(t)
	t.Setenv("PRS_CONFIG", "/from/environment.toml")
	t.Setenv("XDG_CONFIG_HOME", "/from/xdg")

	path, err := ResolvePath("")

	if err != nil || path != "/from/environment.toml" {
		t.Fatalf("path = %q, err = %v", path, err)
	}
}

func TestResolvePathUsesXDGConfigHomeWhenSet(t *testing.T) {
	isolateEnvironment(t)
	t.Setenv("XDG_CONFIG_HOME", "/from/xdg")

	path, err := ResolvePath("")

	if expected := filepath.Join("/from/xdg", "prs", "config.toml"); err != nil || path != expected {
		t.Fatalf("path = %q, err = %v, expected %q", path, err, expected)
	}
}

func TestResolvePathFallsBackToTheConfigDirectoryInHome(t *testing.T) {
	home := isolateEnvironment(t)

	path, err := ResolvePath("")

	if expected := filepath.Join(home, ".config", "prs", "config.toml"); err != nil || path != expected {
		t.Fatalf("path = %q, err = %v, expected %q", path, err, expected)
	}
}

func TestExpandHomeReplacesALeadingTilde(t *testing.T) {
	home := isolateEnvironment(t)

	expanded, err := ExpandHome("~/code")

	if expected := filepath.Join(home, "code"); err != nil || expanded != expected {
		t.Fatalf("path = %q, err = %v, expected %q", expanded, err, expected)
	}
}

func TestExpandHomeLeavesAnAbsolutePathAlone(t *testing.T) {
	isolateEnvironment(t)
	absolute := t.TempDir()

	expanded, err := ExpandHome(absolute)

	if err != nil || expanded != absolute {
		t.Fatalf("path = %q, err = %v", expanded, err)
	}
}

func TestExpandHomeLeavesAnotherUsersTildeAlone(t *testing.T) {
	isolateEnvironment(t)

	expanded, err := ExpandHome("~other/code")

	if err != nil || expanded != "~other/code" {
		t.Fatalf("path = %q, err = %v", expanded, err)
	}
}
