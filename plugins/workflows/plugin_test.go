package workflows

import (
	"flag"
	"reflect"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

const (
	deploysSection = `
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

	if runs.Tab != "Workflows" || workflows.Tab != "Workflows" {
		t.Fatalf("tabs = %q and %q", runs.Tab, workflows.Tab)
	}
}

func TestSectionBoardsRefreshOnTheConfiguredIntervalOfTheTable(t *testing.T) {
	model := startedModel(t, newFakeProvider(), "refresh_interval = \"30s\"\n"+deploysSection)

	assertShows(t, model, "refreshes every 30 seconds")
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

func TestLastRunColumnStaysUnloadedWhenTheLookupFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("LatestRun", errorForTests)

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
