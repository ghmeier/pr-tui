package workflows

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	defaultProviderName = "github"
	defaultLimit        = 20
	maximumLimit        = 100
	defaultSectionTitle = "Recent runs"
)

type sectionKind int

const (
	sectionKindRuns sectionKind = iota
	sectionKindWorkflows
)

// section is one configured board, with the plugin-level repositories already applied.
type section struct {
	title string
	kind  sectionKind
	// workflows are names or file base names; empty means every workflow.
	workflows    []string
	statuses     []RunStatus
	triggers     []Trigger
	repositories []Repository
	limit        int
}

// boardSettings is what the boards need besides their provider.
type boardSettings struct {
	providerName string
	sections     []section
}

// The statuses and triggers a section can filter on, in the order error messages list them.
var (
	filterableStatuses = []RunStatus{RunStatusQueued, RunStatusRunning, RunStatusSucceeded, RunStatusFailed, RunStatusCancelled}
	triggerSettings    = []struct {
		trigger Trigger
		name    string
	}{
		{TriggerPush, "push"},
		{TriggerPullRequest, "pull_request"},
		{TriggerSchedule, "schedule"},
		{TriggerManual, "manual"},
		{TriggerMergeQueue, "merge_queue"},
	}
)

// repositoryPartPattern is what GitHub allows in an owner or a name, which is also what is
// safe to put in a command line.
var repositoryPartPattern = regexp.MustCompile("^[A-Za-z0-9_.-]+$")

func parseRepository(text string) (Repository, bool) {
	owner, name, found := strings.Cut(text, "/")
	if !found || !repositoryPartPattern.MatchString(owner) || !repositoryPartPattern.MatchString(name) {
		return Repository{}, false
	}
	return Repository{Owner: owner, Name: name}, true
}

// loadSettings reads the [workflows] table and checks it. Every error names the table and the
// key, and a section's errors also name its title.
func loadSettings(decodeSection func(name string, target any) error) (boardSettings, error) {
	var raw struct {
		Provider     string   `toml:"provider"`
		Repositories []string `toml:"repositories"`
		Sections     []struct {
			Title        string   `toml:"title"`
			Kind         string   `toml:"kind"`
			Workflows    []string `toml:"workflows"`
			Status       []string `toml:"status"`
			Trigger      []string `toml:"trigger"`
			Repositories []string `toml:"repositories"`
			Limit        *int     `toml:"limit"`
		} `toml:"sections"`
	}
	if err := decodeSection(pluginName, &raw); err != nil {
		return boardSettings{}, err
	}
	pluginRepositories, err := parseRepositories("[workflows] repositories", raw.Repositories)
	if err != nil {
		return boardSettings{}, err
	}
	if len(pluginRepositories) == 0 {
		return boardSettings{}, fmt.Errorf(`[%s] repositories is required; list each one as "owner/name"`, pluginName)
	}
	loaded := boardSettings{providerName: cmp.Or(raw.Provider, defaultProviderName)}
	if len(raw.Sections) == 0 {
		loaded.sections = []section{{title: defaultSectionTitle, repositories: pluginRepositories, limit: defaultLimit}}
		return loaded, nil
	}
	for index, entry := range raw.Sections {
		label := sectionLabel(index, entry.Title)
		if entry.Title == "" {
			return boardSettings{}, fmt.Errorf("%s: title is required", label)
		}
		for _, earlier := range loaded.sections {
			if earlier.title == entry.Title {
				return boardSettings{}, fmt.Errorf("%s: title is used by more than one section, and the title is what tells the tabs apart", label)
			}
		}
		next := section{title: entry.Title, workflows: entry.Workflows, limit: defaultLimit, repositories: pluginRepositories}
		switch entry.Kind {
		case "", "runs":
			next.kind = sectionKindRuns
		case "workflows":
			next.kind = sectionKindWorkflows
		default:
			return boardSettings{}, fmt.Errorf(`%s: kind %q is not one of runs, workflows`, label, entry.Kind)
		}
		if next.kind == sectionKindWorkflows {
			if len(entry.Status) > 0 {
				return boardSettings{}, fmt.Errorf(`%s: status only applies to sections of kind "runs"`, label)
			}
			if entry.Limit != nil {
				return boardSettings{}, fmt.Errorf(`%s: limit only applies to sections of kind "runs"`, label)
			}
		}
		for _, text := range entry.Status {
			status, known := parseStatus(text)
			if !known {
				return boardSettings{}, fmt.Errorf("%s: status %q is not one of %s", label, text, statusNames())
			}
			next.statuses = append(next.statuses, status)
		}
		for _, text := range entry.Trigger {
			trigger, known := parseTrigger(text)
			if !known {
				return boardSettings{}, fmt.Errorf("%s: trigger %q is not one of %s", label, text, triggerNames())
			}
			next.triggers = append(next.triggers, trigger)
		}
		if entry.Limit != nil {
			if *entry.Limit < 1 || *entry.Limit > maximumLimit {
				return boardSettings{}, fmt.Errorf("%s: limit %d must be between 1 and %d", label, *entry.Limit, maximumLimit)
			}
			next.limit = *entry.Limit
		}
		if entry.Repositories != nil {
			overridden, err := parseRepositories(label+": repositories", entry.Repositories)
			if err != nil {
				return boardSettings{}, err
			}
			if len(overridden) == 0 {
				return boardSettings{}, fmt.Errorf("%s: repositories must list at least one repository, or be left out to use the list of [workflows]", label)
			}
			next.repositories = overridden
		}
		loaded.sections = append(loaded.sections, next)
	}
	return loaded, nil
}

// sectionLabel names a section in an error by its title, or by its position when it has none.
func sectionLabel(index int, title string) string {
	if title == "" {
		return fmt.Sprintf("[[workflows.sections]] #%d", index+1)
	}
	return fmt.Sprintf("[[workflows.sections]] %q", title)
}

// parseRepositories turns "owner/name" entries into repositories; subject names the key they
// were written in, with the section when there is one, so that an error can name it.
func parseRepositories(subject string, entries []string) ([]Repository, error) {
	var repositories []Repository
	for _, text := range entries {
		parsed, valid := parseRepository(text)
		if !valid {
			return nil, fmt.Errorf(`%s entry %q must be written as "owner/name"`, subject, text)
		}
		if slices.Contains(repositories, parsed) {
			return nil, fmt.Errorf("%s lists %q more than once", subject, text)
		}
		repositories = append(repositories, parsed)
	}
	return repositories, nil
}

func parseStatus(text string) (RunStatus, bool) {
	for _, status := range filterableStatuses {
		if status.name() == text {
			return status, true
		}
	}
	return 0, false
}

func parseTrigger(text string) (Trigger, bool) {
	for _, setting := range triggerSettings {
		if setting.name == text {
			return setting.trigger, true
		}
	}
	return 0, false
}

func statusNames() string {
	names := make([]string, len(filterableStatuses))
	for index, status := range filterableStatuses {
		names[index] = status.name()
	}
	return strings.Join(names, ", ")
}

func triggerNames() string {
	names := make([]string, len(triggerSettings))
	for index, setting := range triggerSettings {
		names[index] = setting.name
	}
	return strings.Join(names, ", ")
}
