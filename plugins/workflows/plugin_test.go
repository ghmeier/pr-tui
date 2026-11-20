package workflows

import (
	"flag"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

const (
	allWorkflowsSection = "\n[[workflows.sections]]\ntitle = \"All\"\nkind = \"workflows\"\n"
	deploysSection      = `
[[workflows.sections]]
title = "Deploys"
workflows = ["Deploy"]
`
	nightlySection = `
[[workflows.sections]]
title = "Scheduled"
kind = "workflows"
trigger = ["schedule"]
`
)

func assertShows(t *testing.T, model *plugin.Model, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if !strings.Contains(screen(model), text) {
			t.Fatalf("the screen does not show %q:%s", text, describe(model))
		}
	}
}

func assertHides(t *testing.T, model *plugin.Model, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if strings.Contains(screen(model), text) {
			t.Fatalf("the screen shows %q:%s", text, describe(model))
		}
	}
}

func TestPluginRegistersItselfUnderTheNameOfItsConfigurationTable(t *testing.T) {
	var names []string
	for _, registered := range plugin.Registered() {
		names = append(names, registered.Name())
	}

	if !strings.Contains(strings.Join(names, ","), pluginName) {
		t.Fatalf("registered plugins = %v, expected %s", names, pluginName)
	}
}

func TestPluginTakesNoCommandLineFlags(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)

	(&workflowsPlugin{}).Flags(flags)

	flags.VisitAll(func(declared *flag.Flag) { t.Errorf("declared the flag %q", declared.Name) })
}

func TestSetupFailsWhenTheProviderIsNotRegistered(t *testing.T) {
	_, err := plugin.NewHost("config.toml", configtest.Load(t, "plugins = [\"workflows\"]\n[workflows]\nprovider = \"gitlab\"\n"+bothRepositories+"\n"), []plugin.Plugin{&workflowsPlugin{}})

	if err == nil || !strings.Contains(err.Error(), "[workflows] provider") || !strings.Contains(err.Error(), `"gitlab"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestWithoutSectionsOneBoardListsEveryRunNewestFirstAcrossRepositories(t *testing.T) {
	provider := newFakeProvider()

	model := startedModel(t, provider, "")

	runs := provider.runs
	newest := []string{runs[storefrontRepository][1].Title, runs[billingRepository][1].Title, runs[storefrontRepository][0].Title, runs[billingRepository][0].Title}
	positions := order(model, newest...)
	if positions[0] < 0 || !(positions[0] < positions[1] && positions[1] < positions[2] && positions[2] < positions[3]) {
		t.Fatalf("positions of %v = %v:%s", newest, positions, describe(model))
	}
}

func TestEverySectionIsAPaneOfOneTabInTheOrderOfTheFile(t *testing.T) {
	model := startedModel(t, newFakeProvider(), deploysSection+nightlySection)

	titles := strings.Split(screen(model), "\n")[0]

	deploys, scheduled := strings.Index(titles, "Deploys"), strings.Index(titles, "Scheduled")
	if deploys < 0 || scheduled < deploys {
		t.Fatalf("the first line does not name both sections in order:%s", describe(model))
	}
}

func TestRightMovesTheFocusFromOneSectionToTheNext(t *testing.T) {
	model := startedModel(t, newFakeProvider(), deploysSection+nightlySection)

	onDeploys := lineWith(model, "of")
	plugintest.Press(t, model, "right")

	if !strings.Contains(onDeploys, "runs") || !strings.Contains(lineWith(model, "of"), "workflows") {
		t.Fatalf("the status line before is %q and after is %q", onDeploys, lineWith(model, "of"))
	}
}

func TestEverySectionBoardIsAPaneOfTheWorkflowsTab(t *testing.T) {
	runs := runsBoard(newFakeProvider(), section{title: "Deploys", kind: sectionKindRuns})
	workflows := workflowsBoard(newFakeProvider(), section{title: "Scheduled", kind: sectionKindWorkflows})

	if runs.Tab != tabTitle || workflows.Tab != tabTitle {
		t.Fatalf("tabs = %q and %q", runs.Tab, workflows.Tab)
	}
}

func TestSectionBoardsRefreshOnTheConfiguredIntervalOfTheTable(t *testing.T) {
	model := startedModel(t, newFakeProvider(), "refresh_interval = \"30s\"\n"+deploysSection)

	assertShows(t, model, "refreshes every 30 seconds")
}

func TestWorkflowsSectionRefreshesOnTheProgramsIntervalWhateverTheTableSays(t *testing.T) {
	model := startedModel(t, newFakeProvider(), "refresh_interval = \"30s\"\n"+nightlySection)

	assertHides(t, model, "30 seconds")
	assertShows(t, model, "refreshes every 3 minutes")
}

func TestRunsSectionIsBusyOnlyWhileARunIsQueuedOrRunning(t *testing.T) {
	board := runsBoard(newFakeProvider(), section{title: "Deploys", kind: sectionKindRuns})
	runWith := func(status RunStatus) plugin.Row {
		return RunRow{Run: Run{URL: "https://example.test/" + status.mark(), Status: status}}
	}
	finished := runWith(RunStatusSucceeded)

	for _, status := range []RunStatus{RunStatusQueued, RunStatusRunning} {
		if !board.Busy([]plugin.Row{finished, runWith(status)}) {
			t.Errorf("a %v run should make the section busy", status)
		}
	}
	for _, status := range []RunStatus{RunStatusSucceeded, RunStatusFailed, RunStatusCancelled, RunStatusSkipped} {
		if board.Busy([]plugin.Row{runWith(status)}) {
			t.Errorf("a %v run should not make the section busy", status)
		}
	}
	if board.Busy(nil) {
		t.Errorf("a section without runs should not be busy")
	}
}

func TestMergeQueueSectionIsBusyOnlyWhileTheQueueHasAnEntry(t *testing.T) {
	board := mergeQueueBoard(newFakeProvider(), section{title: "Queue", kind: sectionKindMergeQueue})

	if !board.Busy([]plugin.Row{QueueRow{Entry: QueueEntry{Number: 1}}}) || board.Busy(nil) {
		t.Fatalf("busy with an entry = %v, with none = %v", board.Busy([]plugin.Row{QueueRow{}}), board.Busy(nil))
	}
}

func TestSectionBoardsRefreshOnTheProgramsIntervalWithoutOneInTheTable(t *testing.T) {
	model := startedModel(t, newFakeProvider(), deploysSection)

	assertHides(t, model, "30 seconds")
	assertShows(t, model, "refreshes every 3 minutes")
}

func TestRunsSectionKeepsOnlyTheRunsOfTheNamedWorkflows(t *testing.T) {
	provider := newFakeProvider()
	deploy, checks := provider.runs[storefrontRepository][1], provider.runs[storefrontRepository][0]

	model := startedModel(t, provider, deploysSection)

	assertShows(t, model, deploy.Title)
	assertHides(t, model, checks.Title)
}

func TestRunsSectionMatchesAWorkflowByItsFileName(t *testing.T) {
	provider := newFakeProvider()
	checks, deploy := provider.runs[storefrontRepository][0], provider.runs[storefrontRepository][1]

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"Checks\"\nworkflows = [\"checks.yml\"]\n")

	assertShows(t, model, checks.Title)
	assertHides(t, model, deploy.Title)
}

func TestRunsSectionKeepsOnlyTheRunsWithTheRequestedStatuses(t *testing.T) {
	provider := newFakeProvider()
	running, queued, failed := provider.runs[storefrontRepository][1], provider.runs[billingRepository][0], provider.runs[storefrontRepository][0]

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"Active\"\nstatus = [\"queued\", \"running\"]\n")

	assertShows(t, model, running.Title, queued.Title)
	assertHides(t, model, failed.Title)
}

func TestRunsSectionKeepsOnlyTheRunsWithTheRequestedTriggers(t *testing.T) {
	provider := newFakeProvider()
	scheduled, pushed := provider.runs[billingRepository][0], provider.runs[storefrontRepository][1]

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"Scheduled\"\ntrigger = [\"schedule\"]\n")

	assertShows(t, model, scheduled.Title)
	assertHides(t, model, pushed.Title)
}

func TestRunsSectionAsksTheProviderForEachRepositoryWithItsFilters(t *testing.T) {
	provider := newFakeProvider()

	startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"Failures\"\nstatus = [\"failed\"]\ntrigger = [\"push\"]\nlimit = 5\n")

	queried := map[Repository]RunQuery{}
	for _, query := range provider.recordedQueries() {
		queried[query.Repository] = query
	}
	for _, repository := range []Repository{storefrontRepository, billingRepository} {
		expected := RunQuery{Repository: repository, Statuses: []RunStatus{RunStatusFailed}, Triggers: []Trigger{TriggerPush}, Limit: 5}
		if got := queried[repository]; got.Limit != expected.Limit || len(got.Statuses) != 1 || got.Statuses[0] != RunStatusFailed || len(got.Triggers) != 1 || got.Triggers[0] != TriggerPush {
			t.Fatalf("query for %s = %+v, expected %+v", repository.FullName(), got, expected)
		}
	}
}

func TestRunsSectionWithNamedWorkflowsQueriesEachMatchingWorkflowAndNotTheWholeRepository(t *testing.T) {
	provider := newFakeProvider()

	startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"Some\"\nworkflows = [\"Deploy\", \"checks.yml\"]\nlimit = 7\n")

	queried := map[string]int{}
	for _, query := range provider.recordedQueries() {
		if query.WorkflowFile == "" || query.Limit != 7 {
			t.Fatalf("query = %+v", query)
		}
		queried[query.Repository.FullName()+"/"+query.WorkflowFile]++
	}
	expected := map[string]int{"acme/storefront/deploy.yaml": 1, "acme/storefront/checks.yml": 1}
	if !reflect.DeepEqual(queried, expected) {
		t.Fatalf("queried = %v, expected %v", queried, expected)
	}
}

func TestRunsSectionWithoutNamedWorkflowsMakesOneRepositoryWideQueryEach(t *testing.T) {
	provider := newFakeProvider()

	startedModel(t, provider, "")

	queries := provider.recordedQueries()
	if len(queries) != 2 {
		t.Fatalf("queries = %+v", queries)
	}
	for _, query := range queries {
		if query.WorkflowFile != "" {
			t.Fatalf("query = %+v", query)
		}
	}
}

func TestRunsSectionSkipsANameThatMatchesNoWorkflowOfARepositoryAndStillShowsTheOthers(t *testing.T) {
	provider := newFakeProvider()
	storefrontNightly, billingNightly := provider.runs[billingRepository][0], provider.runs[storefrontRepository][1]

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"Some\"\nworkflows = [\"Release\", \"checks.yml\"]\n")

	assertShows(t, model, provider.runs[storefrontRepository][0].Title)
	assertHides(t, model, storefrontNightly.Title, billingNightly.Title)
}

func TestRunsSectionFailsToLoadWhenTheWorkflowsOfARepositoryCannotBeListed(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("Workflows", errorForTests)
	configured := section{title: "Some", workflows: []string{"Deploy"}, repositories: []Repository{storefrontRepository}, limit: defaultLimit}

	rows, err := loadRuns(provider, configured)

	if err != errorForTests || rows != nil {
		t.Fatalf("rows = %v, error = %v", rows, err)
	}
}

func TestRunsSectionQueriesOnlyItsOwnRepositoriesWhenItOverridesTheList(t *testing.T) {
	provider := newFakeProvider()
	billing, storefront := provider.runs[billingRepository][1], provider.runs[storefrontRepository][1]

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"Billing\"\nrepositories = [\"acme/billing\"]\n")

	assertShows(t, model, billing.Title)
	assertHides(t, model, storefront.Title)
	for _, query := range provider.recordedQueries() {
		if query.Repository != billingRepository {
			t.Fatalf("queried %s", query.Repository.FullName())
		}
	}
}

func TestRunsSectionFailsToLoadWhenAnyRepositoryFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("Runs"+billingRepository.Name, errorForTests)
	configured := section{title: "Recent runs", repositories: []Repository{storefrontRepository, billingRepository}, limit: defaultLimit}

	rows, err := loadRuns(provider, configured)

	if err != errorForTests || rows != nil {
		t.Fatalf("rows = %v, error = %v", rows, err)
	}
}

func TestRunRowsAreKeyedByTheRunsURL(t *testing.T) {
	run := newFakeProvider().runs[storefrontRepository][0]

	if key := (RunRow{Run: run}).Key(); key != run.URL {
		t.Fatalf("key = %q, expected %q", key, run.URL)
	}
}

func TestRunsBoardShowsEachColumnOfARun(t *testing.T) {
	provider := newFakeProvider()
	failed := provider.runs[storefrontRepository][0]

	model := startedModel(t, provider, "")

	line := lineWith(model, failed.Title)
	for _, expected := range []string{failed.Repository.Name, failed.Workflow, failed.Branch, TriggerPullRequest.word(), RunStatusFailed.mark(), "12m"} {
		if !strings.Contains(line, expected) {
			t.Fatalf("the row %q does not show %q", line, expected)
		}
	}
}

func TestWorkflowsSectionKeepsOnlyTheWorkflowsWithTheTrigger(t *testing.T) {
	provider := newFakeProvider()

	model := startedModel(t, provider, nightlySection)

	assertShows(t, model, "Nightly")
	assertHides(t, model, "Deploy", "Checks", "Release")
}

func TestWorkflowsSectionKeepsOnlyTheNamedWorkflows(t *testing.T) {
	model := startedModel(t, newFakeProvider(), "\n[[workflows.sections]]\ntitle = \"Some\"\nkind = \"workflows\"\nworkflows = [\"release.yaml\"]\n")

	assertShows(t, model, "Release")
	assertHides(t, model, "Nightly", "Deploy")
}

func TestWorkflowRowsAreKeyedByRepositoryAndFile(t *testing.T) {
	workflow := newFakeProvider().workflows[billingRepository][1]

	if key := (WorkflowRow{Workflow: workflow}).Key(); key != "acme/billing/release.yaml" {
		t.Fatalf("key = %q", key)
	}
}

func TestWorkflowsBoardShowsAllTheSchedulesOfAWorkflow(t *testing.T) {
	provider := newFakeProvider()
	nightly := provider.workflows[storefrontRepository][2]

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"All\"\nkind = \"workflows\"\n")

	if line := lineWith(model, nightly.Schedules[0]); !strings.Contains(line, strings.Join(nightly.Schedules, ", ")) {
		t.Fatalf("the row %q does not show the schedules", line)
	}
	if line := lineWith(model, "Release"); strings.Contains(line, "0 6") {
		t.Fatalf("the row %q shows the schedule of another workflow", line)
	}
}

func TestLastRunColumnShowsTheMarkOfTheLatestRunAndSaysNeverForAWorkflowWithoutOne(t *testing.T) {
	provider := newFakeProvider()

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"All\"\nkind = \"workflows\"\n")

	if line := lineWith(model, "Checks"); !strings.Contains(line, RunStatusFailed.mark()) {
		t.Fatalf("the row %q does not show the failed mark", line)
	}
	if line := lineWith(model, "Release"); !strings.Contains(line, "never") || strings.Contains(line, "…") {
		t.Fatalf("the row %q does not say the workflow never ran", line)
	}
}

func TestLastRunColumnAsksForRecentRunsOncePerRepositoryAndForAWorkflowOnlyWhenItIsMissingFromThem(t *testing.T) {
	provider := newFakeProvider()
	checks := provider.workflows[storefrontRepository][1]
	provider.outsideRecentRuns[latestRunKey(checks)] = true

	startedModel(t, provider, allWorkflowsSection)

	repositories := provider.recentRunRepositories()
	slices.SortFunc(repositories, func(left, right Repository) int { return strings.Compare(left.Name, right.Name) })
	if !slices.Equal(repositories, []Repository{billingRepository, storefrontRepository}) {
		t.Fatalf("recent runs were asked for %v", repositories)
	}
	// Two more workflows never ran, so they are missing from the recent runs too.
	lookups := provider.latestRunWorkflows()
	expected := []string{
		latestRunKey(checks),
		latestRunKey(provider.workflows[storefrontRepository][2]),
		latestRunKey(provider.workflows[billingRepository][1]),
	}
	slices.Sort(lookups)
	slices.Sort(expected)
	if !slices.Equal(lookups, expected) {
		t.Fatalf("latest runs were asked for %v, expected %v", lookups, expected)
	}
}

func TestLastRunColumnShowsEachWorkflowItsOwnRunWhetherItCameFromRecentRunsOrItsOwnLookup(t *testing.T) {
	provider := newFakeProvider()
	provider.outsideRecentRuns[latestRunKey(provider.workflows[storefrontRepository][1])] = true

	model := startedModel(t, provider, allWorkflowsSection)

	if line := lineWith(model, "Checks"); !strings.Contains(line, RunStatusFailed.mark()) {
		t.Fatalf("the row %q should show the run found by its own lookup", line)
	}
	if line := lineWith(model, "Deploy"); !strings.Contains(line, RunStatusRunning.mark()) {
		t.Fatalf("the row %q should show the run from the recent runs", line)
	}
}

func TestLastRunColumnStaysUnloadedForAWorkflowWhoseOwnLookupFails(t *testing.T) {
	provider := newFakeProvider()
	provider.outsideRecentRuns[latestRunKey(provider.workflows[storefrontRepository][1])] = true
	provider.fail("LatestRun", errorForTests)

	model := startedModel(t, provider, allWorkflowsSection)

	if line := lineWith(model, "Checks"); !strings.Contains(line, "…") {
		t.Fatalf("the row %q should show the placeholder", line)
	}
	if line := lineWith(model, "Deploy"); strings.Contains(line, "…") {
		t.Fatalf("the row %q should not wait for another workflow's lookup", line)
	}
}

func TestLastRunColumnStaysUnloadedWhenTheLookupFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("LatestRuns", errorForTests)

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"All\"\nkind = \"workflows\"\n")

	line := lineWith(model, "Checks")
	if !strings.Contains(line, "…") || strings.Contains(line, RunStatusFailed.mark()) {
		t.Fatalf("the row %q should show the placeholder", line)
	}
}

func TestEnterOnARunOpensItsRunScreen(t *testing.T) {
	provider := newFakeProvider()
	newest := provider.runs[storefrontRepository][1]

	model := startedModel(t, provider, "")
	plugintest.Press(t, model, "enter")

	assertShows(t, model, newest.Repository.FullName()+"  "+newest.Workflow, newest.Title, "esc back")
}

func TestEnterOnAWorkflowOpensTheScreenOfItsLatestRun(t *testing.T) {
	provider := newFakeProvider()
	checks := provider.runs[storefrontRepository][0]

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"All\"\nkind = \"workflows\"\nworkflows = [\"Checks\"]\n")
	plugintest.Press(t, model, "enter")

	assertShows(t, model, checks.Title, "esc back")
}

func TestEnterOnAWorkflowThatNeverRanShowsANoticeAndOpensNoScreen(t *testing.T) {
	provider := newFakeProvider()

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"Some\"\nkind = \"workflows\"\nworkflows = [\"Release\"]\n")
	plugintest.Press(t, model, "enter")

	assertShows(t, model, "No runs yet")
	assertHides(t, model, "esc back")
}

func TestOpenKeyOnARunOpensItsURLInTheBrowser(t *testing.T) {
	calls := stubOpen(t)
	provider := newFakeProvider()
	newest := provider.runs[storefrontRepository][1]

	model := startedModel(t, provider, "")
	plugintest.Press(t, model, "o")

	if len(*calls) != 1 || (*calls)[0][0] != "open" || (*calls)[0][1] != newest.URL {
		t.Fatalf("commands run = %v", *calls)
	}
}

func TestOpenKeyOnAWorkflowOpensTheWorkflowsPage(t *testing.T) {
	calls := stubOpen(t)
	provider := newFakeProvider()
	deploy := provider.workflows[storefrontRepository][0]

	model := startedModel(t, provider, "\n[[workflows.sections]]\ntitle = \"Some\"\nkind = \"workflows\"\nworkflows = [\"Deploy\"]\n")
	plugintest.Press(t, model, "o")

	if len(*calls) != 1 || (*calls)[0][1] != deploy.URL {
		t.Fatalf("commands run = %v", *calls)
	}
}

func TestFacetsOfARunsSectionNarrowByRepositoryAndByWorkflow(t *testing.T) {
	model := startedModel(t, newFakeProvider(), "")

	plugintest.Press(t, model, "f")

	assertShows(t, model, "Repository", "acme/storefront", "acme/billing", "Workflow", "Deploy", "Checks", "Nightly")
}

const mergeQueueSection = "\n[[workflows.sections]]\ntitle = \"Merge queue\"\nkind = \"merge_queue\"\n"

func TestMergeQueueRowsAreKeyedByThePullRequestsURL(t *testing.T) {
	entry := newFakeProvider().queues[billingRepository][0]

	if key := (QueueRow{Entry: entry}).Key(); key != entry.URL {
		t.Fatalf("key = %q, expected %q", key, entry.URL)
	}
}

func TestMergeQueueSectionCombinesRepositoriesInConfiguredOrderThenByPosition(t *testing.T) {
	provider := newFakeProvider()
	storefront := provider.queues[storefrontRepository]
	billing := provider.queues[billingRepository]
	firstInStorefront, secondInStorefront := storefront[1], storefront[0]

	model := startedModel(t, provider, mergeQueueSection)

	positions := order(model, firstInStorefront.Title, secondInStorefront.Title, billing[0].Title)
	if !slices.IsSorted(positions) || positions[0] < 0 {
		t.Fatalf("titles appear at %v, expected storefront 1, storefront 2, then billing:%s", positions, describe(model))
	}
}

func TestMergeQueueSectionShowsEachColumnOfAnEntry(t *testing.T) {
	provider := newFakeProvider()
	entry := provider.queues[storefrontRepository][1]

	model := startedModel(t, provider, mergeQueueSection)

	line := lineWith(model, entry.Title)
	for _, expected := range []string{
		RunStatusSucceeded.mark(), entry.Repository.Name, "#" + strconv.Itoa(entry.Number), QueueStateMergeable.word(), "12m",
		"~" + durationText(entry.EstimatedTimeToMerge), entry.Author,
	} {
		if !strings.Contains(line, expected) {
			t.Fatalf("the row %q does not show %q", line, expected)
		}
	}
}

func TestMergeQueueSectionShowsAnEmptyEstimateWhenTheForgeGivesNone(t *testing.T) {
	provider := newFakeProvider()
	withoutEstimate := provider.queues[billingRepository][0]
	withEstimate := provider.queues[storefrontRepository][1]

	model := startedModel(t, provider, mergeQueueSection)

	if line := lineWith(model, withoutEstimate.Title); strings.Contains(line, "~") {
		t.Fatalf("the row %q shows an estimate", line)
	}
	if line := lineWith(model, withEstimate.Title); !strings.Contains(line, "~"+durationText(withEstimate.EstimatedTimeToMerge)) {
		t.Fatalf("the row %q does not show the estimate", line)
	}
}

func TestMergeQueueSectionFailsToLoadWhenAnyRepositoryFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("MergeQueue", errorForTests)

	model := startedModel(t, provider, mergeQueueSection)

	assertShows(t, model, "provider failed")
	assertHides(t, model, provider.queues[billingRepository][0].Title)
}

func TestOpenKeyOnAQueuedPullRequestOpensItsURLInTheBrowser(t *testing.T) {
	calls := stubOpen(t)
	provider := newFakeProvider()
	front := provider.queues[storefrontRepository][1]

	model := startedModel(t, provider, mergeQueueSection)
	plugintest.Press(t, model, "o")

	if len(*calls) != 1 || (*calls)[0][0] != "open" || (*calls)[0][1] != front.URL {
		t.Fatalf("commands run = %v", *calls)
	}
}

func TestMergeQueueSectionHasNoEnterAction(t *testing.T) {
	provider := newFakeProvider()

	model := startedModel(t, provider, mergeQueueSection)
	plugintest.Press(t, model, "enter")

	assertHides(t, model, "esc back")
}

func TestFacetOfAMergeQueueSectionNarrowsByRepository(t *testing.T) {
	model := startedModel(t, newFakeProvider(), mergeQueueSection)

	plugintest.Press(t, model, "f")

	assertShows(t, model, "Repository", "acme/storefront", "acme/billing")
}
