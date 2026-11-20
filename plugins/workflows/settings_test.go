package workflows

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
)

const tablePrefix = "[workflows]\n" + bothRepositories + "\n"

func loadSettingsFrom(t *testing.T, text string) (boardSettings, error) {
	t.Helper()
	return loadSettings(configtest.Load(t, text).Section)
}

func assertRejected(t *testing.T, text string, fragments ...string) {
	t.Helper()
	_, err := loadSettingsFrom(t, text)
	if err == nil {
		t.Fatalf("expected an error naming %v", fragments)
	}
	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not mention %q", err, fragment)
		}
	}
}

func sectionText(lines ...string) string {
	return tablePrefix + "[[workflows.sections]]\n" + strings.Join(lines, "\n") + "\n"
}

func TestSettingsWithoutSectionsMakeOneRunsSectionOfEveryRepository(t *testing.T) {
	loaded, err := loadSettingsFrom(t, tablePrefix)

	expected := section{
		title: "Recent runs", kind: sectionKindRuns, limit: 20,
		repositories: []Repository{storefrontRepository, billingRepository},
	}
	if err != nil || loaded.providerName != "github" || !reflect.DeepEqual(loaded.sections, []section{expected}) {
		t.Fatalf("settings = %+v, error = %v", loaded, err)
	}
}

func TestSettingsReadEveryKeyOfASection(t *testing.T) {
	loaded, err := loadSettingsFrom(t, "[workflows]\nprovider = \"other\"\n"+bothRepositories+`
[[workflows.sections]]
title = "Deploys"
kind = "runs"
workflows = ["Deploy CI", "deploy.yaml"]
status = ["queued", "running"]
trigger = ["push", "merge_queue"]
repositories = ["acme/billing"]
limit = 50

[[workflows.sections]]
title = "Scheduled"
kind = "workflows"
trigger = ["schedule"]
`)

	expected := []section{
		{
			title: "Deploys", kind: sectionKindRuns, workflows: []string{"Deploy CI", "deploy.yaml"},
			statuses: []RunStatus{RunStatusQueued, RunStatusRunning}, triggers: []Trigger{TriggerPush, TriggerMergeQueue},
			repositories: []Repository{billingRepository}, limit: 50,
		},
		{
			title: "Scheduled", kind: sectionKindWorkflows, triggers: []Trigger{TriggerSchedule},
			repositories: []Repository{storefrontRepository, billingRepository}, limit: 20,
		},
	}
	if err != nil || loaded.providerName != "other" || !reflect.DeepEqual(loaded.sections, expected) {
		t.Fatalf("settings = %+v, error = %v", loaded, err)
	}
}

func TestSettingsAcceptEveryStatusAndTriggerWord(t *testing.T) {
	loaded, err := loadSettingsFrom(t, sectionText(
		`status = ["queued", "running", "succeeded", "failed", "cancelled"]`,
		`trigger = ["push", "pull_request", "schedule", "manual", "merge_queue"]`,
		`title = "Everything"`,
	))

	if err != nil || len(loaded.sections[0].statuses) != 5 || len(loaded.sections[0].triggers) != 5 {
		t.Fatalf("settings = %+v, error = %v", loaded, err)
	}
}

func TestSettingsRejectMissingRepositoriesNamingTheTableAndKey(t *testing.T) {
	assertRejected(t, "[workflows]\nprovider = \"github\"\n", "[workflows] repositories is required")
	assertRejected(t, "[workflows]\nrepositories = []\n", "[workflows] repositories is required")
}

func TestSettingsRejectARepositoryNotWrittenAsOwnerAndName(t *testing.T) {
	for _, text := range []string{"storefront", "acme/", "/storefront", "acme/store front", "acme/store/front"} {
		assertRejected(t, "[workflows]\nrepositories = [\""+text+"\"]\n", "[workflows]", "repositories", text, "owner/name")
	}
}

func TestSettingsRejectARepositoryListedTwice(t *testing.T) {
	assertRejected(t, "[workflows]\nrepositories = [\"acme/billing\", \"acme/billing\"]\n", "[workflows] repositories", "more than once")
}

func TestSettingsRejectAnUnknownKeyOfTheTableOrOfASection(t *testing.T) {
	assertRejected(t, tablePrefix+"colour = \"red\"\n", "[workflows]", "colour")
	assertRejected(t, sectionText(`title = "Deploys"`, `colour = "red"`), "[workflows]", "colour")
}

func TestSettingsRejectASectionWithoutATitleNamingItsPosition(t *testing.T) {
	assertRejected(t, sectionText(`title = "First"`)+"[[workflows.sections]]\nkind = \"runs\"\n", "[[workflows.sections]] #2", "title is required")
}

func TestSettingsRejectTwoSectionsWithTheSameTitle(t *testing.T) {
	text := sectionText(`title = "Deploys"`) + "[[workflows.sections]]\ntitle = \"Deploys\"\n"

	assertRejected(t, text, `[[workflows.sections]] "Deploys"`, "more than one section")
}

func TestSettingsRejectAKindThatIsNeitherRunsNorWorkflows(t *testing.T) {
	assertRejected(t, sectionText(`title = "Deploys"`, `kind = "jobs"`), `[[workflows.sections]] "Deploys"`, `kind "jobs"`, "runs, workflows")
}

func TestSettingsRejectAStatusThatIsNotOneOfTheFive(t *testing.T) {
	assertRejected(t, sectionText(`title = "Deploys"`, `status = ["done"]`),
		`[[workflows.sections]] "Deploys": status "done" is not one of queued, running, succeeded, failed, cancelled`)
	assertRejected(t, sectionText(`title = "Deploys"`, `status = ["skipped"]`), `status "skipped"`)
}

func TestSettingsRejectStatusAndLimitOnAWorkflowsSection(t *testing.T) {
	assertRejected(t, sectionText(`title = "Scheduled"`, `kind = "workflows"`, `status = ["failed"]`), `"Scheduled"`, "status only applies", "runs")
	assertRejected(t, sectionText(`title = "Scheduled"`, `kind = "workflows"`, "limit = 5"), `"Scheduled"`, "limit only applies", "runs")
}

func TestSettingsRejectATriggerThatIsNotOneOfTheFive(t *testing.T) {
	assertRejected(t, sectionText(`title = "Deploys"`, `trigger = ["pull request"]`),
		`"Deploys": trigger "pull request" is not one of push, pull_request, schedule, manual, merge_queue`)
}

func TestSettingsAcceptTheLimitsAtBothEndsOfTheRange(t *testing.T) {
	for _, limit := range []string{"1", "100"} {
		if _, err := loadSettingsFrom(t, sectionText(`title = "Deploys"`, "limit = "+limit)); err != nil {
			t.Errorf("limit %s was rejected: %v", limit, err)
		}
	}
}

func TestSettingsRejectALimitOutsideOneToOneHundred(t *testing.T) {
	for _, limit := range []string{"0", "101", "-3"} {
		assertRejected(t, sectionText(`title = "Deploys"`, "limit = "+limit), `"Deploys"`, "limit "+limit, "between 1 and 100")
	}
}

func TestSettingsRejectASectionRepositoryNotWrittenAsOwnerAndName(t *testing.T) {
	assertRejected(t, sectionText(`title = "Deploys"`, `repositories = ["billing"]`), `"Deploys"`, "repositories", `"billing"`, "owner/name")
}

func TestSettingsRejectASectionWithAnEmptyRepositoryList(t *testing.T) {
	assertRejected(t, sectionText(`title = "Deploys"`, "repositories = []"), `"Deploys"`, "repositories must list at least one")
}
