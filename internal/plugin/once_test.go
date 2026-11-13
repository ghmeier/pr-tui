package plugin

import (
	"bytes"
	"strings"
	"testing"
)

func printOnce(t *testing.T, width int, plugins ...Plugin) (string, error) {
	t.Helper()
	names := make([]string, len(plugins))
	for index, plugin := range plugins {
		names[index] = `"` + plugin.Name() + `"`
	}
	host, err := buildHost(t, "plugins = ["+strings.Join(names, ", ")+"]\n", plugins...)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = host.PrintOnce(&output, width)
	return output.String(), err
}

func TestPrintOnceWritesTheSummaryAndTheWholeTableWithoutATitleForOneBoard(t *testing.T) {
	rows := groceries()

	output, err := printOnce(t, 100, boardPlugin("fake", groceryBoard(newRowSource(rows...))))

	outputLines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if err != nil || len(outputLines) != 2+len(rows) {
		t.Fatalf("err = %v, output:\n%s", err, output)
	}
	if !strings.HasPrefix(outputLines[0], "Groups:") || !strings.Contains(outputLines[1], "Name") {
		t.Fatalf("output:\n%s", output)
	}
	if strings.Contains(output, "Groceries") {
		t.Fatalf("a single board needs no title:\n%s", output)
	}
}

func TestPrintOnceLeavesOutTheRowsTheFacetsHide(t *testing.T) {
	rows := groceries()
	board := groceryBoard(newRowSource(rows...))
	board.Facets[0].Selected = []string{"vegetable"}

	output, err := printOnce(t, 100, boardPlugin("fake", board))

	if err != nil || !strings.Contains(output, rows[1].(fakeRow).name) || strings.Contains(output, rows[0].(fakeRow).name) {
		t.Fatalf("err = %v, output:\n%s", err, output)
	}
}

func TestPrintOnceWaitsForTheColumnFetchesBeforePrinting(t *testing.T) {
	rows := groceries()
	board := boardWithStock(rows, func([]Row) []Fetch {
		return []Fetch{stockFor(map[string]any{rows[0].Key(): "12 left"})}
	})

	output, err := printOnce(t, 100, boardPlugin("fake", board))

	if err != nil || !strings.Contains(output, "12 left") {
		t.Fatalf("err = %v, output:\n%s", err, output)
	}
}

func TestPrintOnceLeavesACellUnloadedWhenItsFetchFails(t *testing.T) {
	rows := groceries()
	board := boardWithStock(rows, func([]Row) []Fetch {
		return []Fetch{func() (map[string]any, error) { return nil, errorForTests }}
	})

	output, err := printOnce(t, 100, boardPlugin("fake", board))

	if err != nil || !strings.Contains(output, "…") {
		t.Fatalf("err = %v, output:\n%s", err, output)
	}
}

func TestPrintOnceDrawsTheTableAtTheGivenWidth(t *testing.T) {
	long := fruit("long-key", strings.Repeat("long name ", 30))

	output, err := printOnce(t, 60, boardPlugin("fake", groceryBoard(newRowSource(long))))

	for _, line := range strings.Split(output, "\n") {
		if width := len([]rune(line)); width > 60 {
			t.Fatalf("line is %d columns wide: %q", width, line)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrintOnceTitlesEveryBoardWhenThereAreSeveral(t *testing.T) {
	output, err := printOnce(t, 100, twoBoards(newRowSource(groceries()...), newRowSource(groceries()...))...)

	first, second := strings.Index(output, "Groceries"), strings.Index(output, "Pantry")
	if err != nil || first < 0 || second < first {
		t.Fatalf("err = %v, output:\n%s", err, output)
	}
}

func TestPrintOnceReturnsTheLoadFailure(t *testing.T) {
	source := newRowSource(groceries()...)
	source.fail(errorForTests)

	_, err := printOnce(t, 100, boardPlugin("fake", groceryBoard(source)))

	if err == nil || err.Error() != errorForTests.Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestPrintOnceNamesTheBoardInALoadFailureWhenThereAreSeveral(t *testing.T) {
	failing := newRowSource(groceries()...)
	failing.fail(errorForTests)

	_, err := printOnce(t, 100, twoBoards(newRowSource(groceries()...), failing)...)

	if err == nil || !strings.Contains(err.Error(), "Pantry") || !strings.Contains(err.Error(), errorForTests.Error()) {
		t.Fatalf("err = %v", err)
	}
}
