package workflows

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/shell"
)

var (
	storefrontRepository = Repository{Owner: "acme", Name: "storefront"}
	billingRepository    = Repository{Owner: "acme", Name: "billing"}
)

var errorForTests = errors.New("provider failed\nsecond line")

// fakeProvider answers from canned runs and workflows, and records the queries it received.
type fakeProvider struct {
	mutex     sync.Mutex
	runs      map[Repository][]Run
	workflows map[Repository][]Workflow
	// latestRuns is by workflow file and repository; a workflow without an entry has never run.
	latestRuns map[string]Run
	jobs       map[int64][]Job
	queues     map[Repository][]QueueEntry
	failures   map[string]error
	queries    []RunQuery
	jobLoads   int
}

func latestRunKey(workflow Workflow) string {
	return workflow.Repository.FullName() + "/" + workflow.File
}

func at(hour, minute int) time.Time {
	return time.Date(2026, 11, 10, hour, minute, 0, 0, time.UTC)
}

// newFakeProvider has four runs, newest first: a running deploy and a failed pull request run
// in the storefront, then a successful deploy and a queued scheduled run in billing. The
// storefront has three workflows and billing two, one of which has never run.
func newFakeProvider() *fakeProvider {
	storefrontDeploy := Run{
		Repository: storefrontRepository, ID: 101, URL: "https://example.test/storefront/runs/101", Workflow: "Deploy",
		WorkflowFile: "deploy.yaml", Title: "Ship the cart", Branch: "main", Trigger: TriggerPush, Status: RunStatusRunning,
		Actor: "person", CreatedAt: at(12, 0), StartedAt: at(12, 0), UpdatedAt: at(12, 5),
	}
	storefrontCheck := Run{
		Repository: storefrontRepository, ID: 102, URL: "https://example.test/storefront/runs/102", Workflow: "Checks",
		WorkflowFile: "checks.yml", Title: "Fix the footer", Branch: "footer-fix", Trigger: TriggerPullRequest, Status: RunStatusFailed,
		Actor: "person", CreatedAt: at(11, 0), StartedAt: at(11, 0), UpdatedAt: at(11, 12),
	}
	billingDeploy := Run{
		Repository: billingRepository, ID: 201, URL: "https://example.test/billing/runs/201", Workflow: "Deploy",
		WorkflowFile: "deploy.yaml", Title: "Ship the invoices", Branch: "main", Trigger: TriggerPush, Status: RunStatusSucceeded,
		Actor: "person", CreatedAt: at(11, 30), StartedAt: at(11, 30), UpdatedAt: at(11, 31),
	}
	billingNightly := Run{
		Repository: billingRepository, ID: 202, URL: "https://example.test/billing/runs/202", Workflow: "Nightly",
		WorkflowFile: "nightly.yaml", Title: "Rebuild the catalogue", Branch: "main", Trigger: TriggerSchedule, Status: RunStatusQueued,
		Actor: "scheduler", CreatedAt: at(10, 0),
	}
	storefrontWorkflows := []Workflow{
		{Repository: storefrontRepository, Name: "Deploy", File: "deploy.yaml", URL: "https://example.test/storefront/deploy", Triggers: []Trigger{TriggerPush, TriggerManual}},
		{Repository: storefrontRepository, Name: "Checks", File: "checks.yml", URL: "https://example.test/storefront/checks", Triggers: []Trigger{TriggerPullRequest}},
		{Repository: storefrontRepository, Name: "Nightly", File: "nightly.yaml", URL: "https://example.test/storefront/nightly", Triggers: []Trigger{TriggerSchedule}, Schedules: []string{"0 6 * * *", "0 18 * * *"}},
	}
	billingWorkflows := []Workflow{
		{Repository: billingRepository, Name: "Nightly", File: "nightly.yaml", URL: "https://example.test/billing/nightly", Triggers: []Trigger{TriggerSchedule}, Schedules: []string{"30 2 * * *"}},
		{Repository: billingRepository, Name: "Release", File: "release.yaml", URL: "https://example.test/billing/release", Triggers: []Trigger{TriggerManual}},
	}
	return &fakeProvider{
		runs: map[Repository][]Run{
			storefrontRepository: {storefrontCheck, storefrontDeploy},
			billingRepository:    {billingNightly, billingDeploy},
		},
		workflows: map[Repository][]Workflow{storefrontRepository: storefrontWorkflows, billingRepository: billingWorkflows},
		latestRuns: map[string]Run{
			latestRunKey(storefrontWorkflows[0]): storefrontDeploy,
			latestRunKey(storefrontWorkflows[1]): storefrontCheck,
			latestRunKey(billingWorkflows[0]):    billingNightly,
		},
		jobs: map[int64][]Job{
			storefrontDeploy.ID: {
				{
					Name: "build", Status: RunStatusSucceeded, URL: "https://example.test/storefront/jobs/1",
					StartedAt: at(12, 0), CompletedAt: at(12, 1).Add(32 * time.Second),
					Steps: []Step{
						{Name: "Check out", Status: RunStatusSucceeded, StartedAt: at(12, 0), CompletedAt: at(12, 0).Add(5 * time.Second)},
						{Name: "Compile", Status: RunStatusSucceeded, StartedAt: at(12, 0).Add(5 * time.Second), CompletedAt: at(12, 1).Add(32 * time.Second)},
					},
				},
				{
					Name: "release", Status: RunStatusFailed, URL: "https://example.test/storefront/jobs/2",
					StartedAt: at(12, 2), CompletedAt: at(12, 14),
					Steps: []Step{
						{Name: "Upload", Status: RunStatusFailed, StartedAt: at(12, 2), CompletedAt: at(12, 3)},
					},
				},
			},
		},
		queues: map[Repository][]QueueEntry{
			// The storefront's queue is stored out of order, to show that the board orders it.
			storefrontRepository: {
				{
					Repository: storefrontRepository, Position: 2, State: QueueStateAwaitingChecks, Checks: RunStatusRunning, Number: 502,
					Title: "Resize the banner", URL: "https://example.test/storefront/pull/502", Author: "second",
					EnqueuedAt: time.Now().Add(-5 * time.Minute), EstimatedTimeToMerge: 14 * time.Minute,
				},
				{
					Repository: storefrontRepository, Position: 1, State: QueueStateMergeable, Checks: RunStatusSucceeded, Number: 501,
					Title: "Cache the cart", URL: "https://example.test/storefront/pull/501", Author: "first",
					EnqueuedAt: time.Now().Add(-12 * time.Minute), EstimatedTimeToMerge: 8 * time.Minute,
				},
			},
			billingRepository: {
				{
					Repository: billingRepository, Position: 1, State: QueueStateQueued, Checks: RunStatusFailed, Number: 601,
					Title: "Round the totals", URL: "https://example.test/billing/pull/601", Author: "third",
					EnqueuedAt: time.Now().Add(-3 * time.Hour),
				},
			},
		},
		failures: map[string]error{},
	}
}

func (fake *fakeProvider) fail(method string, err error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.failures[method] = err
}

func (fake *fakeProvider) recordedQueries() []RunQuery {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]RunQuery(nil), fake.queries...)
}

func (fake *fakeProvider) loadsOfJobs() int {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.jobLoads
}

// Runs returns every run of the repository, or of the workflow the query names, whatever else
// the query asks, so that what the plugin filters is visible.
func (fake *fakeProvider) Runs(query RunQuery) ([]Run, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.queries = append(fake.queries, query)
	runs := fake.runs[query.Repository]
	if query.WorkflowFile != "" {
		runs = slices.DeleteFunc(slices.Clone(runs), func(run Run) bool { return run.WorkflowFile != query.WorkflowFile })
	}
	return runs, fake.failures["Runs"+query.Repository.Name]
}

func (fake *fakeProvider) Workflows(repository Repository) ([]Workflow, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.workflows[repository], fake.failures["Workflows"]
}

func (fake *fakeProvider) LatestRun(workflow Workflow) (Run, bool, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	run, exists := fake.latestRuns[latestRunKey(workflow)]
	return run, exists, fake.failures["LatestRun"]
}

func (fake *fakeProvider) Jobs(run Run) ([]Job, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.jobLoads++
	return fake.jobs[run.ID], fake.failures["Jobs"]
}

func (fake *fakeProvider) MergeQueue(repository Repository) ([]QueueEntry, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return slices.Clone(fake.queues[repository]), fake.failures["MergeQueue"]
}

func registerTestProvider(t *testing.T, name string, factory ProviderFactory) {
	t.Helper()
	RegisterProvider(name, factory)
	t.Cleanup(func() { delete(providerFactories, name) })
}

const bothRepositories = `repositories = ["acme/storefront", "acme/billing"]`

// hostFor builds a host whose [workflows] table has the repositories, followed by the given
// sections, over the provider.
func hostFor(t *testing.T, provider *fakeProvider, sections string) (*plugin.Host, error) {
	t.Helper()
	registerTestProvider(t, "fake", func() (Provider, error) { return provider, nil })
	text := "plugins = [\"workflows\"]\n[workflows]\nprovider = \"fake\"\n" + bothRepositories + "\n" + sections
	return plugin.NewHost("config.toml", configtest.Load(t, text), []plugin.Plugin{&workflowsPlugin{}})
}

func startedModel(t *testing.T, provider *fakeProvider, sections string) *plugin.Model {
	t.Helper()
	host, err := hostFor(t, provider, sections)
	if err != nil {
		t.Fatal(err)
	}
	model := host.Model()
	plugintest.Resize(t, model, 160, 40)
	plugintest.Start(t, model)
	return model
}

func screen(model *plugin.Model) string {
	return ansi.Strip(model.View())
}

func lineWith(model *plugin.Model, text string) string {
	for _, line := range strings.Split(screen(model), "\n") {
		if strings.Contains(line, text) {
			return line
		}
	}
	return ""
}

// order is the positions of the texts on the screen, so that a test can say which comes first.
func order(model *plugin.Model, texts ...string) []int {
	view := screen(model)
	positions := make([]int, len(texts))
	for index, text := range texts {
		positions[index] = strings.Index(view, text)
	}
	return positions
}

// stubOpen replaces shell.Run so that no test opens a browser, and returns the arguments of
// every call.
func stubOpen(t *testing.T) *[][]string {
	t.Helper()
	var mutex sync.Mutex
	var calls [][]string
	original := shell.Run
	shell.Run = func(arguments ...string) ([]byte, error) {
		mutex.Lock()
		defer mutex.Unlock()
		calls = append(calls, arguments)
		return nil, nil
	}
	t.Cleanup(func() { shell.Run = original })
	return &calls
}

func describe(model *plugin.Model) string {
	return fmt.Sprintf("\n%s", screen(model))
}
