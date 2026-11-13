package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config"
)

// buildHostFromText writes the text as a configuration file and builds the host from it,
// returning the file's path so that error tests can check the message names it.
func buildHostFromText(t *testing.T, text string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := buildHost(path)
	return path, err
}

// assertRejectedNamingTheFileOnce checks that the message names the file exactly once, and
// every one of the given parts.
func assertRejectedNamingTheFileOnce(t *testing.T, text string, parts ...string) {
	t.Helper()
	path, err := buildHostFromText(t, text)
	if err == nil {
		t.Fatalf("expected an error containing %q", parts)
	}
	if strings.Count(err.Error(), path) != 1 {
		t.Fatalf("error %q should name the file %s exactly once", err, path)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("error %q should contain %q", err, part)
		}
	}
}

// validGithubTable is a [github] table that its provider accepts.
const validGithubTable = "[github]\nrepositories = [\"a/b\"]\n"

func TestBuildHostAcceptsTheStarterConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := config.WriteStarter(path); err != nil {
		t.Fatal(err)
	}

	_, err := buildHost(path)

	if err != nil {
		t.Fatal(err)
	}
}

func TestBuildHostRejectsASectionNoPluginReads(t *testing.T) {
	assertRejectedNamingTheFileOnce(t, validGithubTable+"[gitlab]\nrepositories = [\"a/b\"]\n", "[gitlab]")
}

func TestBuildHostRejectsAnUnknownKeyInASectionNamingTheSectionAndTheKey(t *testing.T) {
	assertRejectedNamingTheFileOnce(t, "[github]\nrepositories = [\"a/b\"]\nreview_teem = \"x\"\n", "[github]", "review_teem")
	assertRejectedNamingTheFileOnce(t, "[pull_requests]\nclone_rot = \"~/code\"\n"+validGithubTable, "[pull_requests]", "clone_rot")
}

func TestBuildHostRejectsAPluginThatIsNotRegisteredAndListsTheRegisteredOnes(t *testing.T) {
	assertRejectedNamingTheFileOnce(t, `plugins = ["calendar"]`, `"calendar"`, "pull_requests")
}

func TestBuildHostRejectsAProviderThatIsNotRegistered(t *testing.T) {
	assertRejectedNamingTheFileOnce(t, "[pull_requests]\nprovider = \"gitlab\"\n", `"gitlab"`)
}

func TestBuildHostRejectsTheOldShapeWhereRepositoriesLiveInPullRequests(t *testing.T) {
	assertRejectedNamingTheFileOnce(t, "[pull_requests]\nrepositories = [\"a/b\"]\n", "repositories", "[github]")
	assertRejectedNamingTheFileOnce(t, "[pull_requests]\nreview_team = \"x\"\n", "review_team", "[github]")
}
