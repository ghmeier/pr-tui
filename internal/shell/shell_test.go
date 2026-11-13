package shell

import (
	"errors"
	"os/exec"
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
