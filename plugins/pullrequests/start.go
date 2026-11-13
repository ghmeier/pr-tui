package pullrequests

import (
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// StartOptions is how the board looks when it opens, as chosen on the command line.
type StartOptions struct {
	// Repositories are names or owner/names to start with; none means every repository.
	Repositories []string
	ShowBots     bool
}

// NewModel builds the board. Its repositories are the provider's, so a repository named in
// the options that the provider does not list is an error.
func NewModel(settings Settings, provider Provider, options StartOptions) (Model, error) {
	available := provider.Repositories()
	selected, err := resolveRepositories(available, options.Repositories)
	if err != nil {
		return Model{}, err
	}
	return newModel(settings, provider, newFilters(available, selected, options.ShowBots)), nil
}

func terminalWidth() int {
	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		return width
	}
	return 120
}

// PrintOnce prints the table once, after the slow columns have loaded, instead of running
// the interactive board.
func (m Model) PrintOnce() error {
	pullRequests, viewer, err := m.provider.PullRequests()
	if err != nil {
		return err
	}
	m.pullRequests = pullRequests
	commands := m.detailCommands()
	messages := make(chan tea.Msg, len(commands))
	for _, command := range commands {
		go func() { messages <- command() }()
	}
	for range commands {
		m, _ = m.update(<-messages)
	}
	fmt.Println(m.filters.summary())
	fmt.Println(renderTable(m.visible, viewer, m.reviewTeam(), terminalWidth(), -1, 0, len(m.visible)))
	return nil
}
