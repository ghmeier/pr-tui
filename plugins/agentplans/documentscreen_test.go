package agentplans

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/shell"
	"github.com/ghmeier/pr-tui/internal/ui"
)

const framedBody = "---\ntitle: Framed plan\nstatus: active\n---\n\n# Framed heading\n\nThe body text.\n"

// openedDocument starts a board with one document and presses enter on it.
func openedDocument(t *testing.T, example doc) (*plugin.Model, string) {
	t.Helper()
	apl := installFakeApl(t, false)
	checkout := apl.newCheckout(t, "storefront", example)
	model := startedModel(t, checkout)
	plugintest.Press(t, model, "enter")
	return model, filepath.Join(checkout, ".apl", example.file)
}

func framedDoc() doc {
	return doc{file: "plan/framed.md", kind: "plan", title: "Framed plan", status: "active", updated: "2026-11-12", body: framedBody}
}

// recordEditor replaces shell.Run so that no test starts an editor, and returns what it was
// asked to run.
func recordEditor(t *testing.T) func() [][]string {
	t.Helper()
	var mutex sync.Mutex
	var recorded [][]string
	original := shell.Run
	t.Cleanup(func() { shell.Run = original })
	shell.Run = func(arguments ...string) ([]byte, error) {
		mutex.Lock()
		defer mutex.Unlock()
		recorded = append(recorded, arguments)
		return nil, nil
	}
	return func() [][]string {
		mutex.Lock()
		defer mutex.Unlock()
		return append([][]string{}, recorded...)
	}
}

func TestEnterOpensTheDocumentBodyWithoutItsFrontMatter(t *testing.T) {
	model, _ := openedDocument(t, framedDoc())

	shown := strings.Join(screenLines(model), "\n")

	if !strings.Contains(shown, "Framed heading") || !strings.Contains(shown, "The body text.") {
		t.Fatalf("the body is missing:\n%s", shown)
	}
	if strings.Contains(shown, "status: active") {
		t.Fatalf("the front matter is shown:\n%s", shown)
	}
}

func TestDocumentScreenHeaderNamesTheRepositoryFileAndTitle(t *testing.T) {
	example := framedDoc()
	model, _ := openedDocument(t, example)

	if line := screenLines(model)[0]; !strings.Contains(line, "storefront "+example.file+"  "+example.title) {
		t.Fatalf("header = %q", line)
	}
}

func TestDocumentWithoutFrontMatterShowsItsFirstLine(t *testing.T) {
	model, _ := openedDocument(t, doc{file: "plan/plain.md", kind: "plan", title: "Plain plan", updated: "2026-11-12", body: "First line of text\n\nSecond paragraph\n"})

	if lineWith(model, "First line of text") == "" {
		t.Fatalf("the first line is missing:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestDocumentScreenShowsAPlaceholderOnlyForAnEmptyBody(t *testing.T) {
	withoutText, _ := openedDocument(t, doc{file: "plan/empty.md", kind: "plan", title: "Empty plan", updated: "2026-11-12", body: "---\ntitle: Empty plan\n---\n"})
	withText, _ := openedDocument(t, framedDoc())

	if lineWith(withoutText, "This document is empty.") == "" || lineWith(withText, "This document is empty.") != "" {
		t.Fatalf("empty:\n%s\nwith text:\n%s", strings.Join(screenLines(withoutText), "\n"), strings.Join(screenLines(withText), "\n"))
	}
}

func TestLoadedMessageForAnotherDocumentLeavesTheScreenAlone(t *testing.T) {
	example := framedDoc()
	model, _ := openedDocument(t, example)
	before := model.View()

	plugintest.Send(t, model, documentLoadedMessage{key: "storefront/plan/other.md", body: "Intruding text"})

	if after := model.View(); after != before || lineWith(model, "Intruding text") != "" {
		t.Fatalf("the screen changed:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestDocumentScreenShowsFailureWhenTheFileIsMissing(t *testing.T) {
	apl := installFakeApl(t, false)
	example := framedDoc()
	checkout := apl.newCheckout(t, "storefront", example)
	if err := os.Remove(filepath.Join(checkout, ".apl", example.file)); err != nil {
		t.Fatal(err)
	}
	model := startedModel(t, checkout)

	plugintest.Press(t, model, "enter")

	if lineWith(model, "Failed:") == "" || lineWith(model, "Framed heading") != "" {
		t.Fatalf("screen:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestEscapeClosesTheDocumentScreenAndShowsTheBoard(t *testing.T) {
	example := framedDoc()
	model, _ := openedDocument(t, example)

	plugintest.Press(t, model, "esc")

	if lineWith(model, "Framed heading") != "" || lineWith(model, "Repository") == "" {
		t.Fatalf("screen:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestDocumentScreenHelpLineNamesTheEditor(t *testing.T) {
	model, _ := openedDocument(t, framedDoc())

	if line := lineWith(model, "scroll"); !strings.Contains(line, "v Test Editor") || !strings.Contains(line, "esc back") {
		t.Fatalf("help = %q", line)
	}
}

func TestDocumentScreenScrollsThroughALongDocument(t *testing.T) {
	var body strings.Builder
	for number := 1; number <= 100; number++ {
		body.WriteString("Paragraph number " + strings.Repeat("x", number%7) + "\n\n")
	}
	body.WriteString("The very last paragraph\n")
	model, _ := openedDocument(t, doc{file: "plan/long.md", kind: "plan", title: "Long plan", updated: "2026-11-12", body: body.String()})
	if lineWith(model, "The very last paragraph") != "" {
		t.Fatal("the end is visible before scrolling")
	}

	plugintest.Press(t, model, "end")

	if lineWith(model, "The very last paragraph") == "" {
		t.Fatalf("screen:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func longDocument() doc {
	var body strings.Builder
	for number := 1; number <= 100; number++ {
		fmt.Fprintf(&body, "Paragraph %03d\n\n", number)
	}
	return doc{file: "plan/long.md", kind: "plan", title: "Long plan", updated: "2026-11-12", body: body.String()}
}

func TestDocumentScreenWheelDownScrollsByOneNotch(t *testing.T) {
	model, _ := openedDocument(t, longDocument())
	top := screenLines(model)

	plugintest.Mouse(t, model, tea.MouseButtonWheelDown, tea.MouseActionPress)

	if screenLines(model)[1] != top[1+ui.WheelNotchLines] {
		t.Fatalf("wheel down did not move by a notch:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestDocumentScreenWheelUpScrollsBackByOneNotch(t *testing.T) {
	model, _ := openedDocument(t, longDocument())
	top := screenLines(model)
	plugintest.Mouse(t, model, tea.MouseButtonWheelDown, tea.MouseActionPress)

	plugintest.Mouse(t, model, tea.MouseButtonWheelUp, tea.MouseActionPress)

	if !reflect.DeepEqual(screenLines(model), top) {
		t.Fatalf("wheel up did not return to the top:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestDocumentScreenWheelUpAtTheTopStaysPut(t *testing.T) {
	model, _ := openedDocument(t, longDocument())
	top := screenLines(model)

	plugintest.Mouse(t, model, tea.MouseButtonWheelUp, tea.MouseActionPress)

	if !reflect.DeepEqual(screenLines(model), top) {
		t.Fatalf("wheel up at the top moved the view:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestDocumentScreenWheelDownAtTheEndStaysPut(t *testing.T) {
	model, _ := openedDocument(t, longDocument())
	plugintest.Press(t, model, "end")
	bottom := screenLines(model)

	plugintest.Mouse(t, model, tea.MouseButtonWheelDown, tea.MouseActionPress)

	if !reflect.DeepEqual(screenLines(model), bottom) {
		t.Fatalf("wheel down at the end moved the view:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestDocumentScreenIgnoresMouseEventsThatAreNotWheelPresses(t *testing.T) {
	model, _ := openedDocument(t, longDocument())
	top := screenLines(model)

	plugintest.Mouse(t, model, tea.MouseButtonLeft, tea.MouseActionPress)
	plugintest.Mouse(t, model, tea.MouseButtonWheelDown, tea.MouseActionRelease)

	if !reflect.DeepEqual(screenLines(model), top) {
		t.Fatalf("the view moved:\n%s", strings.Join(screenLines(model), "\n"))
	}
}

func TestVOnTheBoardRunsTheEditorOnTheDocumentFile(t *testing.T) {
	apl := installFakeApl(t, false)
	example := framedDoc()
	checkout := apl.newCheckout(t, "storefront", example)
	recorded := recordEditor(t)
	model := startedModel(t, checkout)

	plugintest.Press(t, model, "v")

	expected := [][]string{{"test-editor", "--wait", filepath.Join(checkout, ".apl", example.file)}}
	if !reflect.DeepEqual(recorded(), expected) {
		t.Fatalf("commands = %v, expected %v", recorded(), expected)
	}
}

func TestVOnTheDocumentScreenRunsTheEditorOnTheDocumentFile(t *testing.T) {
	recorded := recordEditor(t)
	model, path := openedDocument(t, framedDoc())

	plugintest.Press(t, model, "v")

	expected := [][]string{{"test-editor", "--wait", path}}
	if !reflect.DeepEqual(recorded(), expected) {
		t.Fatalf("commands = %v, expected %v", recorded(), expected)
	}
}

func TestWithoutFrontMatterDropsTheBlockAtTheTop(t *testing.T) {
	if body := withoutFrontMatter("---\ntitle: x\n---\nBody\n"); body != "Body\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestWithoutFrontMatterKeepsADocumentThatHasNone(t *testing.T) {
	text := "# Heading\n\n---\n\nAfter a rule\n"

	if body := withoutFrontMatter(text); body != text {
		t.Fatalf("body = %q", body)
	}
}

func TestWithoutFrontMatterKeepsADocumentWhoseBlockIsNeverClosed(t *testing.T) {
	text := "---\ntitle: x\nBody\n"

	if body := withoutFrontMatter(text); body != text {
		t.Fatalf("body = %q", body)
	}
}

func TestOpeningInTheEditorNamesTheFileAndTheEditorInItsNotice(t *testing.T) {
	configured := settings{}
	configured.editor.Name = "Test Editor"
	row := Row{Path: "/checkout/.apl/plan/a.md", Doc: Doc{File: "plan/a.md"}}

	effect := configured.openInEditor(row)

	if expected := "Opening plan/a.md in Test Editor…"; effect.Notice != expected {
		t.Fatalf("notice = %q, expected %q", effect.Notice, expected)
	}
}
