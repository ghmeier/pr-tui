package pullrequests

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFilterMenuTogglesTheRepositoryUnderTheCursorBeyondTheNumberKeys(t *testing.T) {
	repositories := repositoriesNamed(11)
	eleventh, tenth := repositories[10], repositories[9]
	presses := append([]string{"f"}, slices.Repeat([]string{"down"}, 10)...)

	m := press(t, boardFor(providerWithRepositories(repositories...), testSettings()), append(presses, " ")...)

	if m.filters.repositories[eleventh.FullName()] || !m.filters.repositories[tenth.FullName()] {
		t.Fatalf("selected repositories = %v after toggling %s", m.filters.repositories, eleventh.FullName())
	}
}

func TestFilterMenuMovesTheCursorWithEitherPairOfKeys(t *testing.T) {
	board := boardFor(providerWithRepositories(repositoriesNamed(3)...), testSettings())

	down := press(t, board, "f", "down", "j")
	up := press(t, down, "up", "k")

	if down.filters.cursor != 2 || up.filters.cursor != 0 {
		t.Fatalf("cursor = %d after moving down, %d after moving back up", down.filters.cursor, up.filters.cursor)
	}
}

func TestFilterMenuCursorStaysOnTheFirstAndLastRepository(t *testing.T) {
	repositories := repositoriesNamed(3)
	board := boardFor(providerWithRepositories(repositories...), testSettings())
	last := len(repositories) - 1

	atTop := press(t, board, "f", "up", "k")
	atBottom := press(t, atTop, "down", "down", "down", "j")

	if atTop.filters.cursor != 0 || atBottom.filters.cursor != last {
		t.Fatalf("cursor = %d at the top, %d at the bottom, expected 0 and %d", atTop.filters.cursor, atBottom.filters.cursor, last)
	}
}

func TestFilterMenuMarksOnlyTheRowUnderTheCursor(t *testing.T) {
	repositories := repositoriesNamed(3)
	board := boardFor(providerWithRepositories(repositories...), testSettings())
	board.filters.cursor = 1

	lines := strings.Split(ansi.Strip(board.filters.menu()), "\n")

	for index, source := range repositories {
		var row string
		for _, line := range lines {
			if strings.Contains(line, source.Name) {
				row = line
			}
		}
		if marked := strings.Contains(row, "▌"); marked != (index == board.filters.cursor) {
			t.Fatalf("row %q marked = %v, cursor is on row %d", row, marked, board.filters.cursor)
		}
	}
}

func TestFilterMenuNumbersOnlyTheFirstNineRepositories(t *testing.T) {
	repositories := repositoriesNamed(10)
	menu := ansi.Strip(boardFor(providerWithRepositories(repositories...), testSettings()).filters.menu())

	for index, source := range repositories {
		numbered := strings.Contains(menu, fmt.Sprintf("%d  ● %s", index+1, source.Name))
		if numbered != (index < 9) {
			t.Fatalf("repository %d numbered = %v:\n%s", index, numbered, menu)
		}
	}
}

func TestNumberShortcutsToggleRepositoriesWithoutMovingTheCursor(t *testing.T) {
	repositories := repositoriesNamed(11)
	ninth := repositories[8]

	m := press(t, boardFor(providerWithRepositories(repositories...), testSettings()), "f", "9")

	if m.filters.repositories[ninth.FullName()] || m.filters.cursor != 0 {
		t.Fatalf("selected = %v, cursor = %d", m.filters.repositories, m.filters.cursor)
	}
}

func TestNumberKeyTogglesTheRepositoryEvenWhenItsNameSharesAFirstLetter(t *testing.T) {
	billing := Repository{Owner: "CoProcure", Name: "billing"}
	gamma := Repository{Owner: "CoProcure", Name: "gamma"}
	provider := providerWithRepositories(gazeboRepository, secondRepository, billing, gamma)

	m := press(t, loadedBoardWith(t, provider), "f", "4")

	selected := m.filters.repositories
	if selected[gamma.FullName()] || !selected[gazeboRepository.FullName()] {
		t.Fatalf("selected repositories = %v after toggling %s", selected, gamma.FullName())
	}
}

func TestBKeyStillSwitchesAuthorsWhenARepositoryNameStartsWithB(t *testing.T) {
	billing := Repository{Owner: "CoProcure", Name: "billing"}
	provider := providerWithRepositories(gazeboRepository, secondRepository, billing)

	m := press(t, loadedBoardWith(t, provider), "f", "b")

	if !m.filters.showBots || !m.filters.repositories[billing.FullName()] {
		t.Fatalf("showBots = %v, selected = %v", m.filters.showBots, m.filters.repositories)
	}
}

func TestNumberKeyBeyondTheListedRepositoriesDoesNothing(t *testing.T) {
	provider := newFakeProvider()
	beyond := fmt.Sprint(len(provider.repositories) + 1)

	m := press(t, loadedBoardWith(t, provider), "f", beyond, "0")

	assertShown(t, m, provider.pullRequest(1), provider.pullRequest(3))
}

func TestFiltersKeepSameNamedRepositoriesFromDifferentOwnersApart(t *testing.T) {
	first := Repository{Owner: "FirstOwner", Name: "shared"}
	second := Repository{Owner: "SecondOwner", Name: "shared"}
	provider := providerWithRepositories(first, second)
	provider.pullRequests = []PullRequest{
		{Repository: first, Number: 1, URL: "https://example.test/first/1", Author: "person"},
		{Repository: second, Number: 3, URL: "https://example.test/second/3", Author: "person"},
	}

	both := loadedBoardWith(t, provider)
	onlySecond := press(t, both, "f", "1")

	assertShown(t, both, provider.pullRequests[0], provider.pullRequests[1])
	assertShown(t, onlySecond, provider.pullRequests[1])
}

func TestStartingFiltersSelectEveryRepositoryWhenNoneIsChosen(t *testing.T) {
	available := newFakeProvider().repositories

	f := newFilters(available, nil, false)

	for _, source := range available {
		if !f.repositories[source.FullName()] {
			t.Fatalf("selected = %v, expected %s to be selected", f.repositories, source.FullName())
		}
	}
}

func TestNewModelStartsWithOnlyTheRepositoriesNamedOnTheCommandLine(t *testing.T) {
	provider := newFakeProvider()
	chosen := provider.repositories[1]

	board, err := NewModel(testSettings(), provider, StartOptions{Repositories: []string{chosen.Name}, ShowBots: true})

	if err != nil || !board.filters.repositories[chosen.FullName()] || board.filters.repositories[provider.repositories[0].FullName()] {
		t.Fatalf("selected = %v, err = %v", board.filters.repositories, err)
	}
	if !board.filters.showBots {
		t.Fatalf("the board should start on bot-authored pull requests")
	}
}

func TestNewModelRejectsARepositoryTheProviderDoesNotList(t *testing.T) {
	provider := newFakeProvider()

	_, err := NewModel(testSettings(), provider, StartOptions{Repositories: []string{"nonexistent"}})

	if err == nil || !strings.Contains(err.Error(), `"nonexistent"`) {
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
