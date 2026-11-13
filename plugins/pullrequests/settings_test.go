package pullrequests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func loadConfigurationText(t *testing.T, text string) *config.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func TestLoadSettingsFillsDefaultsWhenTheSectionIsMissing(t *testing.T) {
	home := isolateHome(t)
	file := loadConfigurationText(t, "")

	settings, err := LoadSettings(file)

	if err != nil {
		t.Fatal(err)
	}
	if settings.ProviderName != DefaultProviderName || settings.CloneRoot != filepath.Join(home, "git") {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestLoadSettingsTakesTheRefreshIntervalAndEditorFromTheTopOfTheFile(t *testing.T) {
	isolateHome(t)
	file := loadConfigurationText(t, "refresh_interval = \"90s\"\n[editor]\nname = \"Zed\"\ncommand = \"zed\"\n")

	settings, err := LoadSettings(file)

	if err != nil || settings.RefreshInterval != file.RefreshInterval || settings.Editor.Name != "Zed" {
		t.Fatalf("settings = %+v, err = %v", settings, err)
	}
}

func TestLoadSettingsReadsTheProviderName(t *testing.T) {
	isolateHome(t)
	file := loadConfigurationText(t, "[pull_requests]\nprovider = \"gitlab\"\n")

	settings, err := LoadSettings(file)

	if err != nil || settings.ProviderName != "gitlab" {
		t.Fatalf("provider name = %q, err = %v", settings.ProviderName, err)
	}
}

func TestLoadSettingsExpandsALeadingTildeInCloneRoot(t *testing.T) {
	home := isolateHome(t)
	file := loadConfigurationText(t, "[pull_requests]\nclone_root = \"~/code\"\n")

	settings, err := LoadSettings(file)

	if err != nil || settings.CloneRoot != filepath.Join(home, "code") {
		t.Fatalf("clone root = %q, err = %v", settings.CloneRoot, err)
	}
}

func TestLoadSettingsLeavesAnAbsoluteCloneRootAlone(t *testing.T) {
	isolateHome(t)
	absolute := t.TempDir()
	file := loadConfigurationText(t, "[pull_requests]\nclone_root = \""+absolute+"\"\n")

	settings, err := LoadSettings(file)

	if err != nil || settings.CloneRoot != absolute {
		t.Fatalf("clone root = %q, err = %v", settings.CloneRoot, err)
	}
}

func TestLoadSettingsRejectsAnUnknownKeyNamingTheFileTheSectionAndTheKey(t *testing.T) {
	isolateHome(t)
	file := loadConfigurationText(t, "[pull_requests]\nclone_rot = \"~/code\"\n")

	_, err := LoadSettings(file)

	if err == nil {
		t.Fatal("expected an error")
	}
	for _, expected := range []string{file.Path, "[pull_requests]", "clone_rot"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error %q should contain %q", err, expected)
		}
	}
}

func TestLoadSettingsIgnoresTheTablesOfOtherPackages(t *testing.T) {
	isolateHome(t)
	file := loadConfigurationText(t, "[github]\nrepositories = [\"a/b\"]\n")

	_, err := LoadSettings(file)

	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadSettingsAcceptsTheStarterConfiguration(t *testing.T) {
	isolateHome(t)
	file := loadConfigurationText(t, config.StarterConfiguration)

	settings, err := LoadSettings(file)

	if err != nil || settings.ProviderName != DefaultProviderName {
		t.Fatalf("settings = %+v, err = %v", settings, err)
	}
}
