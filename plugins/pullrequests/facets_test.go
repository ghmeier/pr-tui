package pullrequests

import (
	"reflect"
	"strings"
	"testing"
)

func TestBoardStartsOnPeopleAndAllRepositories(t *testing.T) {
	provider := newFakeProvider()

	model := loadedBoardWith(t, provider)

	assertShown(t, model, provider, provider.pullRequest(1), provider.pullRequest(3))
	summary := strings.Split(view(model), "\n")[0]
	if !strings.HasPrefix(summary, "Repositories: ● storefront  ● billing  ● reports  ● platform") || !strings.HasSuffix(summary, "Authors: people") {
		t.Fatalf("summary = %q", summary)
	}
}

func TestFilterMenuSwitchesToBotsWithTheBKey(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "f", "b", "esc")

	assertShown(t, model, provider, provider.pullRequest(2))
}

func TestBKeyStillSwitchesAuthorsWhenARepositoryNameStartsWithB(t *testing.T) {
	backups := Repository{Owner: "acme", Name: "backups"}
	provider := providerWithRepositories(storefrontRepository, billingRepository, backups)
	model := loadedBoardWith(t, provider)

	press(t, model, "f", "b", "esc")

	assertShown(t, model, provider, provider.pullRequest(2))
	if summary := strings.Split(view(model), "\n")[0]; !strings.Contains(summary, "● "+backups.Name) {
		t.Fatalf("summary = %q", summary)
	}
}

func TestFiltersKeepSameNamedRepositoriesFromDifferentOwnersApart(t *testing.T) {
	first := Repository{Owner: "FirstOwner", Name: "shared"}
	second := Repository{Owner: "SecondOwner", Name: "shared"}
	provider := providerWithRepositories(first, second)
	provider.pullRequests = []PullRequest{
		{Repository: first, Number: 1, Title: "From the first owner", URL: "https://example.test/first/1", Author: "person"},
		{Repository: second, Number: 3, Title: "From the second owner", URL: "https://example.test/second/3", Author: "person"},
	}
	model := loadedBoardWith(t, provider)

	assertShown(t, model, provider, provider.pullRequests[0], provider.pullRequests[1])
	press(t, model, "f", "1", "esc")

	assertShown(t, model, provider, provider.pullRequests[1])
}

func TestRepoFlagStartsWithOnlyTheRepositoriesNamedOnTheCommandLine(t *testing.T) {
	provider := newFakeProvider()

	model := newSetup(provider).withArguments("--repo", provider.repositories[1].Name).started(t)

	assertShown(t, model, provider, provider.pullRequest(3))
}

func TestRepoFlagMayBeRepeatedAndTakesFullNames(t *testing.T) {
	provider := newFakeProvider()

	model := newSetup(provider).
		withArguments("--repo", provider.repositories[0].FullName(), "--repo", provider.repositories[1].Name).
		started(t)

	assertShown(t, model, provider, provider.pullRequest(1), provider.pullRequest(3))
}

func TestBotsFlagStartsOnBotAuthoredPullRequests(t *testing.T) {
	provider := newFakeProvider()

	model := newSetup(provider).withArguments("--bots").started(t)

	assertShown(t, model, provider, provider.pullRequest(2))
	if summary := strings.Split(view(model), "\n")[0]; !strings.HasSuffix(summary, "Authors: bots") {
		t.Fatalf("summary = %q", summary)
	}
}

func TestSetupRejectsARepositoryTheProviderDoesNotListWithoutNamingTheFile(t *testing.T) {
	provider := newFakeProvider()

	_, err := newSetup(provider).withArguments("--repo", "nonexistent").host(t)

	if err == nil || !strings.Contains(err.Error(), `"nonexistent"`) || strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveRepositoriesAcceptsShortNamesAndFullNames(t *testing.T) {
	available := newFakeProvider().repositories

	names, err := resolveRepositories(available, []string{available[0].Name, available[1].FullName()})

	expected := []string{available[0].FullName(), available[1].FullName()}
	if err != nil || !reflect.DeepEqual(names, expected) {
		t.Fatalf("names = %v, err = %v, expected %v", names, err, expected)
	}
}

func TestResolveRepositoriesRejectsAnUnknownRepositoryAndListsTheConfiguredOnes(t *testing.T) {
	available := newFakeProvider().repositories

	_, err := resolveRepositories(available, []string{"nonexistent"})

	if err == nil || !strings.Contains(err.Error(), `"nonexistent"`) {
		t.Fatalf("error = %v", err)
	}
	for _, source := range available {
		if !strings.Contains(err.Error(), source.FullName()) {
			t.Fatalf("error %q should list %s", err, source.FullName())
		}
	}
}

func TestResolveRepositoriesRejectsAShortNameSharedByTwoOwners(t *testing.T) {
	available := []Repository{{Owner: "FirstOwner", Name: "shared"}, {Owner: "SecondOwner", Name: "shared"}}

	_, err := resolveRepositories(available, []string{"shared"})

	if err == nil || !strings.Contains(err.Error(), "owner/name") {
		t.Fatalf("error = %v, expected a request to write owner/name", err)
	}
}

func TestResolveRepositoriesAcceptsTheFullNameOfAShortNameSharedByTwoOwners(t *testing.T) {
	available := []Repository{{Owner: "FirstOwner", Name: "shared"}, {Owner: "SecondOwner", Name: "shared"}}

	names, err := resolveRepositories(available, []string{available[1].FullName()})

	if err != nil || !reflect.DeepEqual(names, []string{available[1].FullName()}) {
		t.Fatalf("names = %v, err = %v", names, err)
	}
}
