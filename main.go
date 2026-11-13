// prs is a terminal board of open pull requests across a configured set of repositories.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/config"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
	// Imported for its side effect of registering the provider.
	_ "github.com/ghmeier/pr-tui/plugins/pullrequests/github"
)

// knownSections are the tables, besides [editor], that some package reads.
var knownSections = []string{"pull_requests", "github"}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// buildBoard loads the configuration file and builds the board from it. This is the one place
// that names the file in an error, so every message names it exactly once.
func buildBoard(path string, start pullrequests.StartOptions) (pullrequests.Model, error) {
	file, err := config.Load(path)
	if err != nil {
		return pullrequests.Model{}, err
	}
	if err := file.RejectUnknownSections(knownSections...); err != nil {
		return pullrequests.Model{}, fmt.Errorf("%s: %w", path, err)
	}
	settings, err := pullrequests.LoadSettings(file)
	if err != nil {
		return pullrequests.Model{}, fmt.Errorf("%s: %w", path, err)
	}
	provider, err := pullrequests.NewProvider(settings.ProviderName, file.Section)
	if err != nil {
		return pullrequests.Model{}, fmt.Errorf("%s: %w", path, err)
	}
	return pullrequests.NewModel(settings, provider, start)
}

func main() {
	var start pullrequests.StartOptions
	flag.Func("repo", "start with only this repository selected, as a name or owner/name (repeatable)", func(value string) error {
		start.Repositories = append(start.Repositories, value)
		return nil
	})
	flag.BoolVar(&start.ShowBots, "bots", false, "start on bot-authored pull requests")
	once := flag.Bool("once", false, "print the table and exit")
	configurationFlag := flag.String("config", "", "path to the configuration file")
	initialize := flag.Bool("init", false, "write a starter configuration file and exit")
	flag.Parse()

	path, err := config.ResolvePath(*configurationFlag)
	if err != nil {
		fail(err)
	}
	if *initialize {
		if err := config.WriteStarter(path); err != nil {
			fail(err)
		}
		fmt.Println("Wrote", path)
		return
	}
	board, err := buildBoard(path, start)
	if err != nil {
		fail(err)
	}

	if *once {
		err = board.PrintOnce()
	} else {
		_, err = tea.NewProgram(board, tea.WithAltScreen()).Run()
	}
	if err != nil {
		fail(err)
	}
}
