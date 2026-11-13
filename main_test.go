package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config"
)

func loadText(t *testing.T, text string) *config.File {
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

func TestRejectUnknownSectionsAcceptsTheTablesSomePackageReads(t *testing.T) {
	file := loadText(t, "[editor]\nname = \"x\"\n[pull_requests]\n[github]\n")

	if err := rejectUnknownSections(file); err != nil {
		t.Fatal(err)
	}
}

func TestRejectUnknownSectionsNamesTheFileAndTheSection(t *testing.T) {
	file := loadText(t, "[pull_requests]\n[gitlab]\nrepositories = [\"a/b\"]\n")

	err := rejectUnknownSections(file)

	if err == nil || !strings.Contains(err.Error(), file.Path) || !strings.Contains(err.Error(), "[gitlab]") {
		t.Fatalf("error = %v, expected it to name %s and [gitlab]", err, file.Path)
	}
}

func TestTheStarterConfigurationOnlyUsesKnownSections(t *testing.T) {
	file := loadText(t, config.StarterConfiguration)

	if err := rejectUnknownSections(file); err != nil {
		t.Fatal(err)
	}
}
