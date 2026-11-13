package pullrequests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestLoadSettingsFillsDefaultsWhenTheSectionIsMissing(t *testing.T) {
	home := isolateHome(t)
	file := configtest.Load(t, "")

	settings, err := LoadSettings(file)

	if err != nil {
		t.Fatal(err)
	}
	if settings.ProviderName != defaultProviderName || settings.CloneRoot != filepath.Join(home, "git") {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestLoadSettingsTakesTheRefreshIntervalAndEditorFromTheTopOfTheFile(t *testing.T) {
	isolateHome(t)
	file := configtest.Load(t, "refresh_interval = \"90s\"\n[editor]\nname = \"Zed\"\ncommand = \"zed\"\n")

	settings, err := LoadSettings(file)

	if err != nil || settings.RefreshInterval != file.RefreshInterval || settings.Editor.Name != "Zed" {
		t.Fatalf("settings = %+v, err = %v", settings, err)
	}
}

func TestLoadSettingsReadsTheProviderName(t *testing.T) {
	isolateHome(t)
	file := configtest.Load(t, "[pull_requests]\nprovider = \"gitlab\"\n")

	settings, err := LoadSettings(file)

	if err != nil || settings.ProviderName != "gitlab" {
		t.Fatalf("provider name = %q, err = %v", settings.ProviderName, err)
	}
}

func TestLoadSettingsExpandsALeadingTildeInCloneRoot(t *testing.T) {
	home := isolateHome(t)
	file := configtest.Load(t, "[pull_requests]\nclone_root = \"~/code\"\n")

	settings, err := LoadSettings(file)

	if err != nil || settings.CloneRoot != filepath.Join(home, "code") {
		t.Fatalf("clone root = %q, err = %v", settings.CloneRoot, err)
	}
}

func TestLoadSettingsLeavesAnAbsoluteCloneRootAlone(t *testing.T) {
	isolateHome(t)
	absolute := t.TempDir()
	file := configtest.Load(t, "[pull_requests]\nclone_root = \""+absolute+"\"\n")

	settings, err := LoadSettings(file)

	if err != nil || settings.CloneRoot != absolute {
		t.Fatalf("clone root = %q, err = %v", settings.CloneRoot, err)
	}
}

func TestLoadSettingsRejectsAnUnknownKeyNamingTheSectionAndTheKey(t *testing.T) {
	isolateHome(t)
	file := configtest.Load(t, "[pull_requests]\nclone_rot = \"~/code\"\n")

	_, err := LoadSettings(file)

	if err == nil {
		t.Fatal("expected an error")
	}
	for _, expected := range []string{"[pull_requests]", "clone_rot"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error %q should contain %q", err, expected)
		}
	}
}

func TestLoadSettingsIgnoresTheTablesOfOtherPackages(t *testing.T) {
	isolateHome(t)
	file := configtest.Load(t, "[another_package]\nsetting = 1\n")

	_, err := LoadSettings(file)

	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadSettingsSaysWhichTableTheOldProviderKeysMovedTo(t *testing.T) {
	isolateHome(t)
	cases := map[string]string{
		"repositories": `repositories = ["a/b"]`,
		"review_team":  `review_team = "engineering"`,
	}

	for key, line := range cases {
		file := configtest.Load(t, "[pull_requests]\nprovider = \"gitlab\"\n"+line+"\n")

		_, err := LoadSettings(file)

		if err == nil || !strings.Contains(err.Error(), "[pull_requests] "+key) || !strings.Contains(err.Error(), "[gitlab]") {
			t.Fatalf("error = %v, expected it to say %s now belongs under [gitlab]", err, key)
		}
	}
}

func TestLoadSettingsNamesTheDefaultProviderTableForTheOldKeysWhenNoneIsChosen(t *testing.T) {
	isolateHome(t)
	file := configtest.Load(t, "[pull_requests]\nrepositories = [\"a/b\"]\n")

	_, err := LoadSettings(file)

	if err == nil || !strings.Contains(err.Error(), "["+defaultProviderName+"]") {
		t.Fatalf("error = %v, expected it to name [%s]", err, defaultProviderName)
	}
}
