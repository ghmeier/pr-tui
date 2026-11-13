// prs is a terminal board of open pull requests across a configured set of repositories, and
// of whatever other boards its plugins add.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/ghmeier/pr-tui/internal/config"
	"github.com/ghmeier/pr-tui/internal/plugin"
	// Imported for their side effect of registering themselves.
	_ "github.com/ghmeier/pr-tui/plugins/agentplans"
	_ "github.com/ghmeier/pr-tui/plugins/pullrequests"
	_ "github.com/ghmeier/pr-tui/plugins/pullrequests/github"
	_ "github.com/ghmeier/pr-tui/plugins/tickets"
)

// version is set by the release build. A build from source reports "dev".
var version = "dev"

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// buildHost loads the configuration file and sets up the plugins it enables. The host is
// what names the file in an error, so every message names it exactly once.
func buildHost(path string) (*plugin.Host, error) {
	file, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	return plugin.NewHost(path, file, plugin.Registered())
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

func main() {
	plugin.RegisterFlags(plugin.Registered(), flag.CommandLine)
	once := flag.Bool("once", false, "print the tables and exit")
	configurationFlag := flag.String("config", "", "path to the configuration file")
	initialize := flag.Bool("init", false, "write a starter configuration file and exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("prs", version)
		return
	}

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
	host, err := buildHost(path)
	if err != nil {
		fail(err)
	}

	if *once {
		err = host.PrintOnce(os.Stdout, terminalWidth())
	} else {
		_, err = tea.NewProgram(host.Model(), tea.WithAltScreen()).Run()
	}
	if err != nil {
		fail(err)
	}
}
