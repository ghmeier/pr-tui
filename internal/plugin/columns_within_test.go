package plugin

import (
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// markColumn is drawn within the column with the given header and shows a mark for the rows
// whose key is in the set. It also delivers the mark as data, so tests can read it back.
func markColumn(within, mark string, keys ...string) Column {
	return Column{
		Header: "Mark " + mark,
		Within: within,
		Cell: func(row Row, data any) []ui.Segment {
			if data == nil {
				return nil
			}
			return plainCell(data.(string))
		},
		Load: func([]Row) []Fetch {
			values := map[string]any{}
			for _, key := range keys {
				values[key] = mark
			}
			return []Fetch{stockFor(values)}
		},
	}
}

func boardWithMarks(rows []Row, marks ...Column) Board {
	board := groceryBoard(newRowSource(rows...))
	board.Columns = append(board.Columns, marks...)
	return board
}

func TestColumnWithinAnotherIsDrawnAtTheEndOfThatColumnsCell(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, boardWithMarks(rows, markColumn("Group", "*", rows[0].Key())))

	marked := lineWith(model, rows[0].(fakeRow).name)
	unmarked := lineWith(model, rows[1].(fakeRow).name)

	if !strings.Contains(marked, rows[0].(fakeRow).group+" *") {
		t.Fatalf("line = %q, expected the group followed by the mark", marked)
	}
	if strings.Contains(unmarked, "*") {
		t.Fatalf("line = %q, expected no mark", unmarked)
	}
}

func TestColumnWithinAnotherHasNoHeaderOfItsOwn(t *testing.T) {
	rows := groceries()
	withMark := markColumn("Group", "*", rows[0].Key())
	model := modelForBoard(t, boardWithMarks(rows, withMark))

	if header := tableLines(model)[0]; strings.Contains(header, withMark.Header) {
		t.Fatalf("header = %q", header)
	}
}

func TestColumnWithinAnotherWidensTheColumnItIsDrawnIn(t *testing.T) {
	rows := groceries()
	plain := modelForBoard(t, boardWithMarks(rows))
	marked := modelForBoard(t, boardWithMarks(rows, markColumn("Group", "*******", rows[0].Key())))

	plainWidths := columnWidths(plain.current().drawnColumns(), plain.current().cells(), 200)
	markedWidths := columnWidths(marked.current().drawnColumns(), marked.current().cells(), 200)

	if len(markedWidths) != len(plainWidths) {
		t.Fatalf("widths = %v, expected as many as %v", markedWidths, plainWidths)
	}
	if markedWidths[1] <= plainWidths[1] {
		t.Fatalf("group width = %d, expected more than %d", markedWidths[1], plainWidths[1])
	}
}

func TestColumnsWithinTheSameColumnAreDrawnInTheOrderTheyWereAdded(t *testing.T) {
	rows := groceries()
	model := modelForBoard(t, boardWithMarks(rows,
		markColumn("Group", "A", rows[0].Key()), markColumn("Group", "B", rows[0].Key())))

	if line := lineWith(model, rows[0].(fakeRow).name); !strings.Contains(line, rows[0].(fakeRow).group+" A B") {
		t.Fatalf("line = %q", line)
	}
}

func TestColumnWithinAnotherDeliversItsDataToSelectionData(t *testing.T) {
	rows := groceries()
	withMark := markColumn("Group", "*", rows[0].Key())
	board := boardWithMarks(rows, withMark)
	var seen any
	board.Actions = []Action{{Key: "x", Run: func(selection Selection) Effect {
		seen = selection.Data(withMark.Header)
		return Effect{}
	}}}
	model := modelForBoard(t, board)

	plugintest.Press(t, model, "x")

	if seen != "*" {
		t.Fatalf("data = %v", seen)
	}
}

func TestColumnWithinAnotherIsNotAColumnOfTheOnceOutput(t *testing.T) {
	rows := groceries()
	withMark := markColumn("Group", "*", rows[0].Key())
	host, err := buildHost(t, `plugins = ["fake"]`, boardPlugin("fake", boardWithMarks(rows, withMark)))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder

	if err := host.PrintOnce(&out, 200); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out.String(), withMark.Header) || !strings.Contains(out.String(), rows[0].(fakeRow).group+" *") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestColumnWithinAColumnTheBoardLacksIsRejected(t *testing.T) {
	rows := groceries()
	withMark := markColumn("Colour", "*", rows[0].Key())

	_, err := buildHost(t, `plugins = ["fake"]`, boardPlugin("fake", boardWithMarks(rows, withMark)))

	if err == nil || !strings.Contains(err.Error(), withMark.Header) || !strings.Contains(err.Error(), "Colour") || !strings.Contains(err.Error(), groceryBoard(newRowSource()).Name) {
		t.Fatalf("error = %v, expected it to name the board and both headers", err)
	}
}

func TestColumnWithinAColumnThatIsItselfWithinAnotherIsRejected(t *testing.T) {
	rows := groceries()
	inner := markColumn("Group", "A", rows[0].Key())
	outer := markColumn(inner.Header, "B", rows[0].Key())

	_, err := buildHost(t, `plugins = ["fake"]`, boardPlugin("fake", boardWithMarks(rows, inner, outer)))

	if err == nil || !strings.Contains(err.Error(), outer.Header) || !strings.Contains(err.Error(), inner.Header) {
		t.Fatalf("error = %v", err)
	}
}

func TestColumnWithinAColumnOfTheBoardIsAccepted(t *testing.T) {
	rows := groceries()

	_, err := buildHost(t, `plugins = ["fake"]`, boardPlugin("fake", boardWithMarks(rows, markColumn("Group", "*", rows[0].Key()))))

	if err != nil {
		t.Fatal(err)
	}
}
