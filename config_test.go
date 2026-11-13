package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const minimalConfigurationText = `
[pull_requests]
repositories = ["CoProcure/gazebo"]
`

// fullConfigurationText writes every key, using the fixture's own values.
func fullConfigurationText(settings configuration) string {
	var repositories []string
	for _, source := range settings.pullRequests.repositories {
		repositories = append(repositories, fmt.Sprintf("%q", source.fullName()))
	}
	return fmt.Sprintf(`
plugins = [%q]
refresh_interval = %q

[editor]
name = %q
command = %q

[pull_requests]
provider = %q
repositories = [%s]
review_team = %q
clone_root = %q
`,
		settings.plugins[0], settings.refreshInterval.String(), settings.editor.name, settings.editor.command,
		settings.pullRequests.provider, strings.Join(repositories, ", "), settings.pullRequests.reviewTeam,
		settings.pullRequests.cloneRoot,
	)
}

// isolateConfigurationEnvironment keeps the real environment out of path resolution and
// home expansion, and returns the fake home directory.
func isolateConfigurationEnvironment(t *testing.T) string {
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

// loadText loads a configuration file with the given text and returns its path too, so
// that error tests can check the message names it.
func loadText(t *testing.T, text string) (configuration, string, error) {
	t.Helper()
	path := writeConfigurationFile(t, text)
	settings, err := loadConfiguration(path)
	return settings, path, err
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

func TestLoadConfigurationReadsEveryKey(t *testing.T) {
	isolateConfigurationEnvironment(t)
	expected := testConfiguration()
	expected.pullRequests.reviewTeam = "platform"

	settings, _, err := loadText(t, fullConfigurationText(expected))

	if err != nil || !reflect.DeepEqual(settings, expected) {
		t.Fatalf("settings = %+v, err = %v, expected %+v", settings, err, expected)
	}
}

func TestLoadConfigurationFillsDefaultsForOmittedKeys(t *testing.T) {
	home := isolateConfigurationEnvironment(t)

	settings, _, err := loadText(t, minimalConfigurationText)

	if err != nil {
		t.Fatal(err)
	}
	expected := configuration{
		plugins:         knownPlugins,
		refreshInterval: defaultRefreshInterval,
		editor:          editorConfiguration{name: defaultEditorName, command: defaultEditorCommand},
		pullRequests: pullRequestsConfiguration{
			provider:     defaultProvider,
			repositories: []repository{{owner: "CoProcure", name: "gazebo"}},
			cloneRoot:    filepath.Join(home, "git"),
		},
	}
	if !reflect.DeepEqual(settings, expected) {
		t.Fatalf("settings = %+v, expected %+v", settings, expected)
	}
}

func TestLoadConfigurationExpandsALeadingTildeInCloneRoot(t *testing.T) {
	home := isolateConfigurationEnvironment(t)

	settings, _, err := loadText(t, minimalConfigurationText+`clone_root = "~/code"`)

	if err != nil || settings.pullRequests.cloneRoot != filepath.Join(home, "code") {
		t.Fatalf("clone root = %q, err = %v", settings.pullRequests.cloneRoot, err)
	}
}

func TestLoadConfigurationLeavesAnAbsoluteCloneRootAlone(t *testing.T) {
	isolateConfigurationEnvironment(t)
	absolute := t.TempDir()

	settings, _, err := loadText(t, minimalConfigurationText+fmt.Sprintf("clone_root = %q", absolute))

	if err != nil || settings.pullRequests.cloneRoot != absolute {
		t.Fatalf("clone root = %q, err = %v", settings.pullRequests.cloneRoot, err)
	}
}

func TestLoadConfigurationAcceptsRepositoriesFromDifferentOwners(t *testing.T) {
	isolateConfigurationEnvironment(t)
	expected := []repository{{owner: "FirstOwner", name: "one"}, {owner: "SecondOwner", name: "two"}}

	settings, _, err := loadText(t, `
[pull_requests]
repositories = ["FirstOwner/one", "SecondOwner/two"]
`)

	if err != nil || !reflect.DeepEqual(settings.pullRequests.repositories, expected) {
		t.Fatalf("repositories = %v, err = %v", settings.pullRequests.repositories, err)
	}
}

func TestLoadConfigurationRejectsMissingRepositories(t *testing.T) {
	isolateConfigurationEnvironment(t)

	assertRejected(t, `plugins = ["pull_requests"]`, "repositories")
}

func TestLoadConfigurationRejectsRepositoriesNotWrittenAsOwnerSlashName(t *testing.T) {
	isolateConfigurationEnvironment(t)
	malformed := []string{"gazebo", "CoProcure/", "/gazebo", "a/b/c"}

	for _, entry := range malformed {
		assertRejected(t, fmt.Sprintf("[pull_requests]\nrepositories = [%q]", entry), fmt.Sprintf("%q", entry))
	}
}

func TestLoadConfigurationRejectsRepositoryListedTwice(t *testing.T) {
	isolateConfigurationEnvironment(t)

	assertRejected(t, "[pull_requests]\nrepositories = [\"a/b\", \"a/b\"]", "more than once")
}

func TestLoadConfigurationRejectsADurationThatDoesNotParse(t *testing.T) {
	isolateConfigurationEnvironment(t)

	assertRejected(t, `refresh_interval = "soon"`+minimalConfigurationText, "refresh_interval")
}

func TestLoadConfigurationRejectsADurationOfZero(t *testing.T) {
	isolateConfigurationEnvironment(t)

	assertRejected(t, `refresh_interval = "0s"`+minimalConfigurationText, "refresh_interval")
}

func TestLoadConfigurationRejectsAnUnknownPlugin(t *testing.T) {
	isolateConfigurationEnvironment(t)

	assertRejected(t, `plugins = ["calendar"]`+minimalConfigurationText, `"calendar"`)
}

func TestLoadConfigurationRejectsAnUnknownProvider(t *testing.T) {
	isolateConfigurationEnvironment(t)

	assertRejected(t, minimalConfigurationText+`provider = "gitlab"`, `"gitlab"`)
}

func TestLoadConfigurationRejectsAnUnknownKeyAndNamesIt(t *testing.T) {
	isolateConfigurationEnvironment(t)

	assertRejected(t, minimalConfigurationText+`review_teem = "engineering"`, "pull_requests.review_teem")
}

func TestLoadConfigurationRejectsTextThatIsNotTOML(t *testing.T) {
	isolateConfigurationEnvironment(t)

	assertRejected(t, "this is not toml", "")
}

func TestLoadConfigurationOfAMissingFileSaysToRunInit(t *testing.T) {
	isolateConfigurationEnvironment(t)
	path := filepath.Join(t.TempDir(), "absent.toml")

	_, err := loadConfiguration(path)

	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "prs --init") {
		t.Fatalf("error = %v, expected it to name %s and prs --init", err, path)
	}
}

func TestWriteStarterConfigurationCreatesTheDirectoryAndAFileThatLoads(t *testing.T) {
	isolateConfigurationEnvironment(t)
	path := filepath.Join(t.TempDir(), "nested", "prs", "config.toml")

	err := writeStarterConfiguration(path)

	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfiguration(path); err != nil {
		t.Fatalf("the starter configuration should load unchanged: %v", err)
	}
}

func TestWriteStarterConfigurationRefusesToOverwriteAnExistingFile(t *testing.T) {
	path := writeConfigurationFile(t, minimalConfigurationText)

	err := writeStarterConfiguration(path)

	contents, _ := os.ReadFile(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v, expected a refusal naming %s", err, path)
	}
	if string(contents) != minimalConfigurationText {
		t.Fatalf("the existing file was changed to %q", contents)
	}
}

func TestConfigurationPathPrefersTheFlagOverTheEnvironment(t *testing.T) {
	isolateConfigurationEnvironment(t)
	t.Setenv("PRS_CONFIG", "/from/environment.toml")
	t.Setenv("XDG_CONFIG_HOME", "/from/xdg")

	path, err := configurationPath("/from/flag.toml")

	if err != nil || path != "/from/flag.toml" {
		t.Fatalf("path = %q, err = %v", path, err)
	}
}

func TestConfigurationPathPrefersPRSConfigOverTheConfigDirectories(t *testing.T) {
	isolateConfigurationEnvironment(t)
	t.Setenv("PRS_CONFIG", "/from/environment.toml")
	t.Setenv("XDG_CONFIG_HOME", "/from/xdg")

	path, err := configurationPath("")

	if err != nil || path != "/from/environment.toml" {
		t.Fatalf("path = %q, err = %v", path, err)
	}
}

func TestConfigurationPathUsesXDGConfigHomeWhenSet(t *testing.T) {
	isolateConfigurationEnvironment(t)
	t.Setenv("XDG_CONFIG_HOME", "/from/xdg")

	path, err := configurationPath("")

	if expected := filepath.Join("/from/xdg", "prs", "config.toml"); err != nil || path != expected {
		t.Fatalf("path = %q, err = %v, expected %q", path, err, expected)
	}
}

func TestConfigurationPathFallsBackToTheConfigDirectoryInHome(t *testing.T) {
	home := isolateConfigurationEnvironment(t)

	path, err := configurationPath("")

	if expected := filepath.Join(home, ".config", "prs", "config.toml"); err != nil || path != expected {
		t.Fatalf("path = %q, err = %v, expected %q", path, err, expected)
	}
}

func TestResolveRepositoriesAcceptsShortNamesAndFullNames(t *testing.T) {
	available := testConfiguration().pullRequests.repositories

	names, err := resolveRepositories(available, []string{available[0].name, available[1].fullName()})

	expected := []string{available[0].fullName(), available[1].fullName()}
	if err != nil || !reflect.DeepEqual(names, expected) {
		t.Fatalf("names = %v, err = %v, expected %v", names, err, expected)
	}
}

func TestResolveRepositoriesRejectsAnUnknownRepositoryAndListsTheConfiguredOnes(t *testing.T) {
	available := testConfiguration().pullRequests.repositories

	_, err := resolveRepositories(available, []string{"nonexistent"})

	if err == nil || !strings.Contains(err.Error(), `"nonexistent"`) {
		t.Fatalf("error = %v", err)
	}
	for _, source := range available {
		if !strings.Contains(err.Error(), source.fullName()) {
			t.Fatalf("error %q should list %s", err, source.fullName())
		}
	}
}

func TestResolveRepositoriesRejectsAShortNameSharedByTwoOwners(t *testing.T) {
	available := []repository{{owner: "FirstOwner", name: "shared"}, {owner: "SecondOwner", name: "shared"}}

	_, err := resolveRepositories(available, []string{"shared"})

	if err == nil || !strings.Contains(err.Error(), "owner/name") {
		t.Fatalf("error = %v, expected a request to write owner/name", err)
	}
}

func TestResolveRepositoriesAcceptsTheFullNameOfAShortNameSharedByTwoOwners(t *testing.T) {
	available := []repository{{owner: "FirstOwner", name: "shared"}, {owner: "SecondOwner", name: "shared"}}

	names, err := resolveRepositories(available, []string{available[1].fullName()})

	if err != nil || !reflect.DeepEqual(names, []string{available[1].fullName()}) {
		t.Fatalf("names = %v, err = %v", names, err)
	}
}
