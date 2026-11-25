package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/internal/shell"
)

// fakeClaude stands in for the claude and git programs, and records the calls it received.
type fakeClaude struct {
	mutex sync.Mutex
	// agents is what "claude agents --json" lists.
	agents []map[string]any
	// commonDirectories is what git prints as the common directory, by the directory it runs in.
	// A directory that is not listed is not a git repository.
	commonDirectories map[string]string
	relayOutput       string
	relayError        error

	relayRuns      [][]string
	relayInputs    []string
	gitDirectories []string
	// home is the temporary home directory that transcripts are written under.
	home string
}

func installFakeClaude(t *testing.T) *fakeClaude {
	t.Helper()
	fake := &fakeClaude{home: t.TempDir(), commonDirectories: map[string]string{}, relayOutput: `{"is_error":false,"result":"sent"}`}
	originalRun, originalRunIn, originalRunWithInput, originalHome := shell.Run, shell.RunIn, shell.RunWithInput, homeDirectory
	t.Cleanup(func() {
		shell.Run, shell.RunIn, shell.RunWithInput, homeDirectory = originalRun, originalRunIn, originalRunWithInput, originalHome
	})
	homeDirectory = func() (string, error) { return fake.home, nil }
	shell.Run = func(arguments ...string) ([]byte, error) {
		if strings.Join(arguments, " ") != "claude agents --json" {
			return nil, fmt.Errorf("unexpected command %v", arguments)
		}
		fake.mutex.Lock()
		defer fake.mutex.Unlock()
		return json.Marshal(fake.agents)
	}
	shell.RunIn = func(directory string, arguments ...string) ([]byte, error) {
		fake.mutex.Lock()
		defer fake.mutex.Unlock()
		fake.gitDirectories = append(fake.gitDirectories, directory)
		if strings.Join(arguments, " ") != "git rev-parse --path-format=absolute --git-common-dir" {
			return nil, fmt.Errorf("unexpected command %v", arguments)
		}
		common, isRepository := fake.commonDirectories[directory]
		if !isRepository {
			return nil, errors.New("fatal: not a git repository")
		}
		return []byte(common + "\n"), nil
	}
	shell.RunWithInput = func(input string, arguments ...string) ([]byte, error) {
		fake.mutex.Lock()
		defer fake.mutex.Unlock()
		fake.relayRuns = append(fake.relayRuns, arguments)
		fake.relayInputs = append(fake.relayInputs, input)
		return []byte(fake.relayOutput), fake.relayError
	}
	return fake
}

// listed is the listing's entry for an interactive session.
func listed(id, name, directory, status string) map[string]any {
	return map[string]any{"sessionId": id, "name": name, "cwd": directory, "kind": "interactive", "startedAt": startedAt.UnixMilli(), "status": status}
}

var startedAt = time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)

// writeTranscript saves the lines as the transcript of the session, in a directory named like a
// project, and returns its path.
func (fake *fakeClaude) writeTranscript(t *testing.T, sessionID string, lines ...string) string {
	t.Helper()
	directory := filepath.Join(fake.home, ".claude", "projects", "-Users-someone-project")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// entry is one transcript line.
func entry(directory, branch string, at time.Time, text string) string {
	line, _ := json.Marshal(map[string]any{"cwd": directory, "gitBranch": branch, "timestamp": at.Format(time.RFC3339Nano), "message": text})
	return string(line)
}
