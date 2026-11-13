package pullrequests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
)

// settingsFrom loads the text as a configuration file and reads the pull request settings
// from it.
func settingsFrom(t *testing.T, text string) (boardSettings, error) {
	t.Helper()
	return loadSettings(configtest.Load(t, text).Section)
}

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestSettingsFillsDefaultsWhenTheSectionIsMissing(t *testing.T) {
	home := isolateHome(t)

	settings, err := settingsFrom(t, "")

	if err != nil {
		t.Fatal(err)
	}
	if settings.ProviderName != defaultProviderName || settings.CloneRoot != filepath.Join(home, "git") {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestSettingsReadsTheProviderName(t *testing.T) {
	isolateHome(t)

	settings, err := settingsFrom(t, "[pull_requests]\nprovider = \"gitlab\"\n")

	if err != nil || settings.ProviderName != "gitlab" {
		t.Fatalf("provider name = %q, err = %v", settings.ProviderName, err)
	}
}

func TestSettingsExpandsALeadingTildeInCloneRoot(t *testing.T) {
	home := isolateHome(t)

	settings, err := settingsFrom(t, "[pull_requests]\nclone_root = \"~/code\"\n")

	if err != nil || settings.CloneRoot != filepath.Join(home, "code") {
		t.Fatalf("clone root = %q, err = %v", settings.CloneRoot, err)
	}
}

func TestSettingsLeavesAnAbsoluteCloneRootAlone(t *testing.T) {
	isolateHome(t)
	absolute := t.TempDir()

	settings, err := settingsFrom(t, "[pull_requests]\nclone_root = \""+absolute+"\"\n")

	if err != nil || settings.CloneRoot != absolute {
		t.Fatalf("clone root = %q, err = %v", settings.CloneRoot, err)
	}
}

func TestSettingsRejectsAnUnknownKeyNamingTheSectionAndTheKey(t *testing.T) {
	isolateHome(t)

	_, err := settingsFrom(t, "[pull_requests]\nclone_rot = \"~/code\"\n")

	if err == nil {
		t.Fatal("expected an error")
	}
	for _, expected := range []string{"[pull_requests]", "clone_rot"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error %q should contain %q", err, expected)
		}
	}
}

func TestSettingsIgnoresTheTablesOfOtherPackages(t *testing.T) {
	isolateHome(t)

	_, err := settingsFrom(t, "[another_package]\nsetting = 1\n")

	if err != nil {
		t.Fatal(err)
	}
}

func TestSettingsSaysWhichTableTheOldProviderKeysMovedTo(t *testing.T) {
	isolateHome(t)
	cases := map[string]string{
		"repositories": `repositories = ["a/b"]`,
		"review_team":  `review_team = "engineering"`,
	}

	for key, line := range cases {
		_, err := settingsFrom(t, "[pull_requests]\nprovider = \"gitlab\"\n"+line+"\n")

		if err == nil || !strings.Contains(err.Error(), "[pull_requests] "+key) || !strings.Contains(err.Error(), "[gitlab]") {
			t.Fatalf("error = %v, expected it to say %s now belongs under [gitlab]", err, key)
		}
	}
}

func TestSettingsNamesTheDefaultProviderTableForTheOldKeysWhenNoneIsChosen(t *testing.T) {
	isolateHome(t)

	_, err := settingsFrom(t, "[pull_requests]\nrepositories = [\"a/b\"]\n")

	if err == nil || !strings.Contains(err.Error(), "["+defaultProviderName+"]") {
		t.Fatalf("error = %v, expected it to name [%s]", err, defaultProviderName)
	}
}
