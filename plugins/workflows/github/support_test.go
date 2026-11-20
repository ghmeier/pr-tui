package github

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/plugins/workflows"
)

var (
	storefront = workflows.Repository{Owner: "acme", Name: "storefront"}
	billing    = workflows.Repository{Owner: "acme", Name: "billing"}
)

var errorFromGh = errors.New("gh failed")

// stubbedGh is what the stubbed shell saw and what the test wants it to answer.
type stubbedGh struct {
	mutex    sync.Mutex
	commands [][]string
	respond  func(arguments []string) ([]byte, error)
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
		stub.mutex.Lock()
		stub.commands = append(stub.commands, arguments)
		stub.mutex.Unlock()
		return stub.respond(arguments)
	}
	t.Cleanup(func() { shell.Run = original })
	return stub
}

func (stub *stubbedGh) assertCommands(t *testing.T, expected ...[]string) {
	t.Helper()
	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	if !reflect.DeepEqual(stub.commands, expected) {
		t.Fatalf("commands = %v, expected %v", stub.commands, expected)
	}
}

func apiCommand(endpoint string) []string {
	return []string{"gh", "api", endpoint}
}
