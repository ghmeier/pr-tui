// Package claude is the agent session provider for Claude Code. It is the only place in the
// agent sessions plugin that runs claude or reads its files under ~/.claude.
package claude

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/agentsessions"
)

func init() {
	agentsessions.RegisterProvider("claude", func(settings agentsessions.ProviderSettings) (agentsessions.Provider, error) {
		return provider{relayModel: settings.RelayModel}, nil
	})
}

// homeDirectory is where the transcripts live under. Tests replace it.
var homeDirectory = os.UserHomeDir

type provider struct {
	relayModel string
}

// agent is one entry of what "claude agents --json" prints. A session that runs in the
// background reports a state instead of a status.
type agent struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Directory string `json:"cwd"`
	StartedAt int64  `json:"startedAt"`
	Status    string `json:"status"`
	State     string `json:"state"`
}

func listAgents() ([]agent, error) {
	output, err := shell.Run("claude", "agents", "--json")
	if err != nil {
		return nil, fmt.Errorf("listing Claude sessions: %w", err)
	}
	var agents []agent
	if err := json.Unmarshal(output, &agents); err != nil {
		return nil, fmt.Errorf("reading the list of Claude sessions: %w", err)
	}
	return agents, nil
}

func statusOf(entry agent) agentsessions.Status {
	switch cmp.Or(entry.Status, entry.State) {
	case "idle":
		return agentsessions.StatusIdle
	case "busy":
		return agentsessions.StatusBusy
	case "blocked":
		return agentsessions.StatusBlocked
	}
	return agentsessions.StatusUnknown
}

// Sessions lists the live sessions. Reading each session's transcript is independent of the
// others, so they are read at the same time.
func (provider) Sessions() ([]agentsessions.Session, error) {
	agents, err := listAgents()
	if err != nil {
		return nil, err
	}
	sessions := make([]agentsessions.Session, len(agents))
	var group sync.WaitGroup
	for index, entry := range agents {
		group.Add(1)
		go func() {
			defer group.Done()
			sessions[index] = describe(entry)
		}()
	}
	group.Wait()
	return sessions, nil
}

// describe adds what the transcript says to what the listing says. A session whose transcript
// cannot be found or read is still listed, with only what the listing says.
func describe(entry agent) agentsessions.Session {
	started := time.UnixMilli(entry.StartedAt)
	session := agentsessions.Session{ID: entry.SessionID, Name: entry.Name, Status: statusOf(entry), LastActive: started}
	directory := entry.Directory
	if scan, err := scanTranscript(entry.SessionID); err == nil {
		session.Branch = scan.branch
		session.PullRequests = scan.pullRequests
		session.Documents = scan.documents
		if !scan.lastActive.IsZero() {
			session.LastActive = scan.lastActive
		}
		if scan.directory != "" {
			directory = scan.directory
		}
	}
	session.Repository = repositoryName(directory)
	return session
}

// repositoryName is the base name of the checkout that owns the directory, which is the main
// checkout when the directory is in a worktree. It is empty outside a git repository.
func repositoryName(directory string) string {
	if directory == "" {
		return ""
	}
	output, err := shell.RunIn(directory, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return ""
	}
	return filepath.Base(filepath.Dir(strings.TrimSpace(string(output))))
}

// relayInstructions replace Claude Code's own system prompt, so a send costs few tokens.
const relayInstructions = "You relay one message. Call SendMessage once, with to set to the Recipient and message set to " +
	"everything after Message:, unchanged. Then reply with only the word sent, or the error SendMessage returned."

// Send hands the message to a one-shot Claude Code run whose only tool is SendMessage, because
// Claude Code has no command that sends one. A session is addressed by its name, so a name that
// two live sessions share cannot be sent to. The run's prompt goes on standard input, because
// the lists that --tools and --allowed-tools take would swallow a prompt given as an argument.
func (relay provider) Send(session agentsessions.Session, message string) error {
	agents, err := listAgents()
	if err != nil {
		return err
	}
	sharing := 0
	for _, entry := range agents {
		if entry.Name == session.Name {
			sharing++
		}
	}
	if sharing > 1 {
		return fmt.Errorf("Two Claude sessions are named %s; rename one with /rename", session.Name)
	}
	output, err := shell.RunWithInput("Recipient: "+session.Name+"\nMessage:\n"+message,
		"claude", "-p", "--safe-mode", "--model", relay.relayModel, "--effort", "low", "--no-session-persistence",
		"--name", "prs", "--strict-mcp-config", "--tools=SendMessage", "--allowed-tools=SendMessage",
		"--output-format", "json", "--system-prompt", relayInstructions)
	if err != nil {
		return err
	}
	var outcome struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(output, &outcome); err != nil {
		return fmt.Errorf("the relay printed something unexpected: %s", firstLine(output))
	}
	result := strings.TrimSpace(outcome.Result)
	if outcome.IsError || !strings.EqualFold(strings.TrimRightFunc(result, unicode.IsPunct), "sent") {
		if result == "" {
			return errors.New("the relay did not say the message was sent")
		}
		return errors.New(result)
	}
	return nil
}

func firstLine(output []byte) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return line
}
