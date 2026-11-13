// Package configtest loads configuration text for the tests of packages that read a section.
package configtest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config"
)

// Load writes the text to a temporary file and loads it, failing the test if it is invalid.
func Load(t *testing.T, text string) *config.File {
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
