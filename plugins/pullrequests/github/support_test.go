package github

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/internal/config"
	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

const reviewTeamName = "engineering"

var (
	gazebo   = pullrequests.Repository{Owner: "CoProcure", Name: "gazebo"}
	partners = pullrequests.Repository{Owner: "Partner", Name: "u2"}
)

var errorFromGh = errors.New("gh failed")

// stubbedGh is what the stubbed shell saw and what the test wants it to answer.
type stubbedGh struct {
	// commands are the arguments of every call, without the temporary file given to --input.
	commands [][]string
	// inputs are the contents of each temporary file given to --input, in call order.
	inputs []string
	// inputPaths are those temporary files, to check they are cleaned up.
	inputPaths []string
	respond    func(arguments []string) ([]byte, error)
}

// stubShell replaces shell.Run so that no test reaches GitHub. The response is the same for
// every call.
func stubShell(t *testing.T, response string) *stubbedGh {
	t.Helper()
	return stubShellWith(t, func([]string) ([]byte, error) { return []byte(response), nil })
}

func stubShellWith(t *testing.T, respond func(arguments []string) ([]byte, error)) *stubbedGh {
	t.Helper()
	stub := &stubbedGh{respond: respond}
	original := shell.Run
	shell.Run = func(arguments ...string) ([]byte, error) {
		if index := slices.Index(arguments, "--input"); index >= 0 {
			input, _ := os.ReadFile(arguments[index+1])
			stub.inputs = append(stub.inputs, string(input))
			stub.inputPaths = append(stub.inputPaths, arguments[index+1])
			stub.commands = append(stub.commands, arguments[:index])
			return stub.respond(arguments[:index])
		}
		stub.commands = append(stub.commands, arguments)
		return stub.respond(arguments)
	}
	t.Cleanup(func() { shell.Run = original })
	return stub
}

func (stub *stubbedGh) assertCommands(t *testing.T, expected ...[]string) {
	t.Helper()
	if !reflect.DeepEqual(stub.commands, expected) {
		t.Fatalf("commands = %v, expected %v", stub.commands, expected)
	}
}

func (stub *stubbedGh) assertInputs(t *testing.T, expected ...string) {
	t.Helper()
	if !reflect.DeepEqual(stub.inputs, expected) {
		t.Fatalf("inputs = %v, expected %v", stub.inputs, expected)
	}
	for _, path := range stub.inputPaths {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("the temporary file %s was left behind", path)
		}
	}
}

// providerFor is a provider over the given repositories with the review team configured.
func providerFor(repositories ...pullrequests.Repository) *provider {
	return &provider{repositories: repositories, reviewTeam: reviewTeamName}
}

func fixturePullRequest(repository pullrequests.Repository) pullrequests.PullRequest {
	return pullrequests.PullRequest{
		Repository: repository, Number: 7, URL: "https://example.test/" + repository.Name + "/7",
		HeadCommit: "head-commit", UpdatedAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
	}
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

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
