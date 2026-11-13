// prs is a terminal board of open pull requests across a configured set of repositories.
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

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

func rejectUnknownSections(file *config.File) error {
	for _, name := range file.SectionNames() {
		if !slices.Contains(knownSections, name) {
			return fmt.Errorf(
				"%s: unknown section [%s]; known sections: editor, %s", file.Path, name, strings.Join(knownSections, ", "),
			)
		}
	}
	return nil
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
	file, err := config.Load(path)
	if err != nil {
		fail(err)
	}
	if err := rejectUnknownSections(file); err != nil {
		fail(err)
	}
	settings, err := pullrequests.LoadSettings(file)
	if err != nil {
		fail(file.Annotate(err))
	}
	provider, err := pullrequests.NewProvider(settings.ProviderName, file.Section)
	if err != nil {
		fail(file.Annotate(err))
	}
	board, err := pullrequests.NewModel(settings, provider, start)
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
