package workflows

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
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
	sectionKindMergeQueue
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
	// refreshInterval is how often a runs or merge queue section reloads while something in it is
	// in progress, or zero for the configured refresh_interval of the whole program.
	refreshInterval time.Duration
	sections        []section
}

// sectionKinds are the values of the kind key, in the order error messages list them. A kind that
// is left out is the first.
var sectionKinds = []struct {
	name string
	kind sectionKind
}{
	{"runs", sectionKindRuns},
	{"workflows", sectionKindWorkflows},
	{"merge_queue", sectionKindMergeQueue},
}

var allSectionKinds = []sectionKind{sectionKindRuns, sectionKindWorkflows, sectionKindMergeQueue}

// sectionKeyKinds says which kinds of section each optional key applies to.
var sectionKeyKinds = []struct {
	key   string
	kinds []sectionKind
	given func(entry rawSection) bool
}{
	{"workflows", []sectionKind{sectionKindRuns, sectionKindWorkflows}, func(entry rawSection) bool { return len(entry.Workflows) > 0 }},
	{"status", []sectionKind{sectionKindRuns}, func(entry rawSection) bool { return len(entry.Status) > 0 }},
	{"trigger", []sectionKind{sectionKindRuns, sectionKindWorkflows}, func(entry rawSection) bool { return len(entry.Trigger) > 0 }},
	{"limit", []sectionKind{sectionKindRuns}, func(entry rawSection) bool { return entry.Limit != nil }},
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

// repositoryPartPattern is the characters allowed in an owner or a name, which are also safe
// to put in a command line.
var repositoryPartPattern = regexp.MustCompile("^[A-Za-z0-9_.-]+$")

func parseRepository(text string) (Repository, bool) {
	owner, name, found := strings.Cut(text, "/")
	if !found || !repositoryPartPattern.MatchString(owner) || !repositoryPartPattern.MatchString(name) {
		return Repository{}, false
	}
	return Repository{Owner: owner, Name: name}, true
}

// rawSection is a [[workflows.sections]] entry as written. Limit is a pointer so that a limit
// left out can be told from one that was given.
type rawSection struct {
	Title        string   `toml:"title"`
	Kind         string   `toml:"kind"`
	Workflows    []string `toml:"workflows"`
	Status       []string `toml:"status"`
	Trigger      []string `toml:"trigger"`
	Repositories []string `toml:"repositories"`
	Limit        *int     `toml:"limit"`
}

// loadSettings reads the [workflows] table and checks it. Every error names the table and the
// key, and a section's errors also name its title.
func loadSettings(decodeSection func(name string, target any) error) (boardSettings, error) {
	var raw struct {
		Provider        string       `toml:"provider"`
		Repositories    []string     `toml:"repositories"`
		RefreshInterval string       `toml:"refresh_interval"`
		Sections        []rawSection `toml:"sections"`
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
	refreshInterval, err := parseRefreshInterval(raw.RefreshInterval)
	if err != nil {
		return boardSettings{}, err
	}
	loaded := boardSettings{providerName: cmp.Or(raw.Provider, defaultProviderName), refreshInterval: refreshInterval}
	if len(raw.Sections) == 0 {
		loaded.sections = []section{{title: defaultSectionTitle, repositories: pluginRepositories, limit: defaultLimit}}
		return loaded, nil
	}
	for index, entry := range raw.Sections {
		parsed, err := parseSection(sectionLabel(index, entry.Title), entry, pluginRepositories)
		if err != nil {
			return boardSettings{}, err
		}
		for _, earlier := range loaded.sections {
			if earlier.title == parsed.title {
				return boardSettings{}, fmt.Errorf("%s: title is used by more than one section, and the title is what tells the tabs apart", sectionLabel(index, entry.Title))
			}
		}
		loaded.sections = append(loaded.sections, parsed)
	}
	return loaded, nil
}

// parseRefreshInterval reads the interval at which the boards reload. Text that is left out
// gives zero, which means the interval of the whole program.
func parseRefreshInterval(text string) (time.Duration, error) {
	if text == "" {
		return 0, nil
	}
	interval, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf(`[%s] refresh_interval %q is not a duration such as "30s" or "2m"`, pluginName, text)
	}
	if interval < time.Second {
		return 0, fmt.Errorf("[%s] refresh_interval %q must be at least one second", pluginName, text)
	}
	return interval, nil
}

// parseSection checks one section. The repositories of the plugin apply unless the section
// lists its own.
func parseSection(label string, entry rawSection, pluginRepositories []Repository) (section, error) {
	if entry.Title == "" {
		return section{}, fmt.Errorf("%s: title is required", label)
	}
	parsed := section{title: entry.Title, workflows: entry.Workflows, limit: defaultLimit, repositories: pluginRepositories}
	kindKnown := false
	for _, known := range sectionKinds {
		if known.name == cmp.Or(entry.Kind, sectionKinds[0].name) {
			parsed.kind, kindKnown = known.kind, true
		}
	}
	if !kindKnown {
		return section{}, fmt.Errorf("%s: kind %q is not one of %s", label, entry.Kind, strings.Join(kindNames(allSectionKinds), ", "))
	}
	for _, rule := range sectionKeyKinds {
		if rule.given(entry) && !slices.Contains(rule.kinds, parsed.kind) {
			return section{}, fmt.Errorf("%s: %s only applies to sections of kind %s", label, rule.key, "\""+strings.Join(kindNames(rule.kinds), `" and "`)+"\"")
		}
	}
	for _, text := range entry.Status {
		status, known := parseStatus(text)
		if !known {
			return section{}, fmt.Errorf("%s: status %q is not one of %s", label, text, statusNames())
		}
		parsed.statuses = append(parsed.statuses, status)
	}
	for _, text := range entry.Trigger {
		trigger, known := parseTrigger(text)
		if !known {
			return section{}, fmt.Errorf("%s: trigger %q is not one of %s", label, text, triggerNames())
		}
		parsed.triggers = append(parsed.triggers, trigger)
	}
	if entry.Limit != nil {
		if *entry.Limit < 1 || *entry.Limit > maximumLimit {
			return section{}, fmt.Errorf("%s: limit %d must be between 1 and %d", label, *entry.Limit, maximumLimit)
		}
		parsed.limit = *entry.Limit
	}
	if entry.Repositories != nil {
		overridden, err := parseRepositories(label+": repositories", entry.Repositories)
		if err != nil {
			return section{}, err
		}
		if len(overridden) == 0 {
			return section{}, fmt.Errorf("%s: repositories must list at least one repository, or be left out to use the list of [workflows]", label)
		}
		parsed.repositories = overridden
	}
	return parsed, nil
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

// kindNames are the names of the kinds, in the order of sectionKinds.
func kindNames(kinds []sectionKind) []string {
	var names []string
	for _, known := range sectionKinds {
		if slices.Contains(kinds, known.kind) {
			names = append(names, known.name)
		}
	}
	return names
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
