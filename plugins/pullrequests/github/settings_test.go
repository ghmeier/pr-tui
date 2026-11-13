package github

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config"
	"github.com/ghmeier/pr-tui/plugins/pullrequests"
)

func newProviderFromText(t *testing.T, text string) (pullrequests.Provider, error) {
	t.Helper()
	return newProvider(loadConfigurationText(t, text).Section)
}

func assertRejected(t *testing.T, text, expected string) {
	t.Helper()
	_, err := newProviderFromText(t, text)
	if err == nil || !containsAll(err.Error(), expected) {
		t.Fatalf("error = %v, expected it to contain %q", err, expected)
	}
}

func TestProviderReadsRepositoriesAndReviewTeamFromTheGithubSection(t *testing.T) {
	result, err := newProviderFromText(t, fmt.Sprintf(`
[github]
repositories = [%q, %q]
review_team = %q
`, gazebo.FullName(), partners.FullName(), reviewTeamName))

	if err != nil {
		t.Fatal(err)
	}
	if expected := []pullrequests.Repository{gazebo, partners}; !reflect.DeepEqual(result.Repositories(), expected) {
		t.Fatalf("repositories = %v, expected %v", result.Repositories(), expected)
	}
	if result.ReviewTeam() != reviewTeamName {
		t.Fatalf("review team = %q, expected %q", result.ReviewTeam(), reviewTeamName)
	}
}

func TestProviderHasNoReviewTeamWhenTheKeyIsOmitted(t *testing.T) {
	result, err := newProviderFromText(t, fmt.Sprintf("[github]\nrepositories = [%q]\n", gazebo.FullName()))

	if err != nil || result.ReviewTeam() != "" {
		t.Fatalf("review team = %q, err = %v", result.ReviewTeam(), err)
	}
}

func TestProviderRejectsMissingRepositories(t *testing.T) {
	assertRejected(t, "[github]\nreview_team = \"engineering\"\n", "repositories")
}

func TestProviderRejectsAMissingSectionBecauseRepositoriesAreRequired(t *testing.T) {
	assertRejected(t, "", "repositories")
}

func TestProviderRejectsRepositoriesNotWrittenAsOwnerSlashName(t *testing.T) {
	for _, entry := range []string{"gazebo", "CoProcure/", "/gazebo", "a/b/c", `a/b"c`, "a/b c"} {
		assertRejected(t, fmt.Sprintf("[github]\nrepositories = [%q]\n", entry), fmt.Sprintf("%q", entry))
	}
}

func TestProviderRejectsARepositoryListedTwice(t *testing.T) {
	assertRejected(t, "[github]\nrepositories = [\"a/b\", \"a/b\"]\n", "more than once")
}

func TestProviderAcceptsTheSameNameUnderTwoOwners(t *testing.T) {
	result, err := newProviderFromText(t, "[github]\nrepositories = [\"First/shared\", \"Second/shared\"]\n")

	if err != nil || len(result.Repositories()) != 2 {
		t.Fatalf("repositories = %v, err = %v", result.Repositories(), err)
	}
}

func TestProviderRejectsAnUnknownKeyNamingTheFileTheSectionAndTheKey(t *testing.T) {
	text := "[github]\nrepositories = [\"a/b\"]\nreview_teem = \"engineering\"\n"
	file := loadConfigurationText(t, text)

	_, err := newProvider(file.Section)

	if err == nil || !containsAll(err.Error(), file.Path, "[github]", "review_teem") {
		t.Fatalf("error = %v", err)
	}
}

func TestProviderAcceptsTheStarterConfiguration(t *testing.T) {
	_, err := newProviderFromText(t, config.StarterConfiguration)

	if err != nil {
		t.Fatal(err)
	}
}

func TestGithubRegistersItselfUnderTheNameTheConfigurationUses(t *testing.T) {
	file := loadConfigurationText(t, fmt.Sprintf("[github]\nrepositories = [%q]\n", gazebo.FullName()))

	result, err := pullrequests.NewProvider("github", file.Section)

	if err != nil || !reflect.DeepEqual(result.Repositories(), []pullrequests.Repository{gazebo}) {
		t.Fatalf("repositories = %v, err = %v", result.Repositories(), err)
	}
}
