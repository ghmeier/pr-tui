package workflows

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// headerLines and footerLines are the lines of the screen around the jobs: the run's
// description above, and the notice and the help line below.
const (
	headerLines = 3
	footerLines = 2
)

type jobsLoadedMessage struct {
	runURL string
	jobs   []Job
	err    error
}

// runScreen shows a run's jobs, and the steps of the job under the cursor.
type runScreen struct {
	provider Provider
	run      Run
	// selection is what the screen was opened from, and how it hands effects to the board.
	selection plugin.Selection
	width     int
	height    int
	loading   bool
	jobs      []Job
	cursor    int
	notice    string
}

func newRunScreen(provider Provider, run Run, selection plugin.Selection) *runScreen {
	return &runScreen{provider: provider, run: run, selection: selection, width: 120, height: 30, loading: true}
}

func (screen *runScreen) Init() tea.Cmd {
	return screen.loadJobs()
}

func (screen *runScreen) loadJobs() tea.Cmd {
	run := screen.run
	return func() tea.Msg {
		jobs, err := screen.provider.Jobs(run)
		return jobsLoadedMessage{runURL: run.URL, jobs: jobs, err: err}
	}
}

// Update ignores messages about other runs, because screens are given every message that the
// host does not handle.
func (screen *runScreen) Update(message tea.Msg) (plugin.Screen, tea.Cmd) {
	switch message := message.(type) {
	case jobsLoadedMessage:
		if message.runURL != screen.run.URL {
			return screen, nil
		}
		screen.loading = false
		if message.err != nil {
			screen.notice = "Failed: " + ui.FirstLine(message.err)
			return screen, nil
		}
		screen.notice = ""
		screen.jobs = message.jobs
		screen.cursor = max(0, min(screen.cursor, len(screen.jobs)-1))
	case tea.KeyMsg:
		return screen.handleKey(message.String())
	}
	return screen, nil
}

func (screen *runScreen) handleKey(key string) (plugin.Screen, tea.Cmd) {
	switch key {
	case "esc", "q":
		return nil, nil
	case "up", "k":
		screen.cursor = max(screen.cursor-1, 0)
	case "down", "j":
		screen.cursor = max(0, min(screen.cursor+1, len(screen.jobs)-1))
	case "o":
		return screen, screen.selection.Run(openInBrowser(screen.urlToOpen()))
	case "r":
		screen.loading = true
		screen.notice = ""
		return screen, screen.loadJobs()
	}
	return screen, nil
}

// urlToOpen is the selected job's page, or the run's when there is no job to select.
func (screen *runScreen) urlToOpen() string {
	if screen.cursor < len(screen.jobs) && screen.jobs[screen.cursor].URL != "" {
		return screen.jobs[screen.cursor].URL
	}
	return screen.run.URL
}

// bodyLines lists the jobs with the steps of the selected one beneath it, and says which line
// the cursor is on.
func (screen *runScreen) bodyLines(now time.Time) (lines []string, cursorLine int) {
	for index, job := range screen.jobs {
		name := job.Name
		marker := "  "
		if index == screen.cursor {
			cursorLine = len(lines)
			marker = ui.CursorMarker + " "
			name = ui.BoldStyle.Render(name)
		}
		lines = append(lines, marker+job.Status.mark()+" "+name+durationSuffix(job.duration(now)))
		if index != screen.cursor {
			continue
		}
		for _, step := range job.Steps {
			lines = append(lines, "      "+step.Status.mark()+" "+step.Name+durationSuffix(step.duration(now)))
		}
	}
	return lines, cursorLine
}

func durationSuffix(duration time.Duration) string {
	if text := durationText(duration); text != "" {
		return ui.DimStyle.Render("  " + text)
	}
	return ""
}

// View adopts the size it is drawn at, and scrolls so that the cursor stays on screen.
func (screen *runScreen) View(width, height int) string {
	screen.width, screen.height = width, height
	now := time.Now()
	run := screen.run
	status := run.Status.mark() + " " + run.Status.name()
	if text := durationText(run.duration(now)); text != "" {
		status += " · " + text
	}
	header := []string{
		ui.BoldStyle.Render(run.Repository.FullName() + "  " + run.Workflow),
		run.Title,
		status,
	}

	rows := max(height-headerLines-footerLines, 1)
	body := []string{ui.DimStyle.Render("Loading jobs…")}
	switch {
	case screen.loading:
	case screen.notice != "":
		body = nil
	case len(screen.jobs) == 0:
		body = []string{ui.DimStyle.Render("This run has no jobs.")}
	default:
		lines, cursorLine := screen.bodyLines(now)
		first := max(0, min(cursorLine-rows/2, len(lines)-rows))
		body = lines[first:min(len(lines), first+rows)]
	}

	lines := append(header, body...)
	for len(lines) < headerLines+rows {
		lines = append(lines, "")
	}
	help := ui.HelpLine("↑/↓", "job", "o", "open", "r", "reload", "esc", "back")
	lines = append(lines, screen.notice, help)
	for index, line := range lines {
		lines[index] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}
