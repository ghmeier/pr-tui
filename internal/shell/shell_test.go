package shell

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunKeepsTheKindOfFailureWhenTheProgramIsNotInstalled(t *testing.T) {
	_, err := Run("prs-test-program-that-does-not-exist")

	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("error = %v, expected it to wrap exec.ErrNotFound", err)
	}
}

func TestRunReportsWhatTheCommandPrintedToStandardErrorWhenItFails(t *testing.T) {
	_, err := Run("sh", "-c", "echo went wrong >&2; exit 1")

	if err == nil || err.Error() != "went wrong" {
		t.Fatalf("error = %v", err)
	}
	if errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("a failing command was reported as not installed")
	}
}

func TestRunInStartsTheCommandInTheGivenDirectory(t *testing.T) {
	directory := t.TempDir()

	output, err := RunIn(directory, "pwd")

	printed, resolveErr := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	expected, _ := filepath.EvalSymlinks(directory)
	if err != nil || resolveErr != nil || printed != expected {
		t.Fatalf("pwd printed %q, expected %q, error = %v", output, expected, err)
	}
}

func TestRunInReportsFailuresLikeRun(t *testing.T) {
	_, notInstalled := RunIn(t.TempDir(), "prs-test-program-that-does-not-exist")
	_, failing := RunIn(t.TempDir(), "sh", "-c", "echo went wrong >&2; exit 1")

	if !errors.Is(notInstalled, exec.ErrNotFound) {
		t.Fatalf("error = %v, expected it to wrap exec.ErrNotFound", notInstalled)
	}
	if failing == nil || failing.Error() != "went wrong" {
		t.Fatalf("error = %v", failing)
	}
}
