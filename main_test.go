package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const cannedResponse = `{"data": {
  "viewer": {"login": "viewer"},
  "repository0": {"pullRequests": {"nodes": [
    {"number": 1, "title": "Draft from a person", "url": "https://example.test/gazebo/1", "isDraft": true, "headRefOid": "first-commit",
     "updatedAt": "2026-11-10T12:00:00Z", "reviewDecision": null,
     "author": {"login": "person", "__typename": "User"},
     "reviewRequests": {"nodes": []}},
    {"number": 2, "title": "Dependency update", "url": "https://example.test/gazebo/2", "isDraft": false,
     "updatedAt": "2026-11-10T11:00:00Z", "reviewDecision": null,
     "author": {"login": "renovate", "__typename": "Bot"},
     "reviewRequests": {"nodes": []}}
  ]}},
  "repository1": {"pullRequests": {"nodes": [
    {"number": 3, "title": "Ready with reviewers", "url": "https://example.test/u2/3", "isDraft": false,
     "updatedAt": "2026-11-10T10:00:00Z", "reviewDecision": "REVIEW_REQUIRED",
     "author": {"login": "person", "__typename": "User"},
     "reviewRequests": {"nodes": [
       {"requestedReviewer": {"__typename": "User", "login": "viewer"}},
       {"requestedReviewer": {"__typename": "Team", "slug": "engineering"}},
       {"requestedReviewer": {"__typename": "User", "login": "someone-else"}}
     ]}}
  ]}},
  "repository2": {"pullRequests": {"nodes": []}},
  "repository3": {"pullRequests": {"nodes": []}}
}}`

const cannedLineCounts = `{"data": {"repository": {"pullRequests": {"nodes": [
  {"url": "https://example.test/gazebo/1", "additions": 12, "deletions": 3}
]}}}}`

const cannedCIStates = `{"data": {"repository": {"pullRequests": {"nodes": [
  {"url": "https://example.test/gazebo/1", "commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "FAILURE"}}}]}},
  {"url": "https://example.test/u2/3", "commits": {"nodes": [{"commit": {"statusCheckRollup": null}}]}}
]}}}}`

const cannedComments = `[
  {"id": 10, "in_reply_to_id": null, "path": "first.py", "line": 2, "side": "RIGHT",
   "user": {"login": "reviewer"}, "body": "Why this change?", "created_at": "2026-11-10T09:00:00Z"},
  {"id": 11, "in_reply_to_id": 10, "path": "first.py", "line": 2, "side": "RIGHT",
   "user": {"login": "person"}, "body": "Because it was wrong.", "created_at": "2026-11-10T09:30:00Z"},
  {"id": 12, "in_reply_to_id": null, "path": "first.py", "line": null, "side": "RIGHT",
   "user": {"login": "reviewer"}, "body": "Outdated remark", "created_at": "2026-11-09T09:00:00Z"}
]`

const cannedChangedFiles = `[
  {"filename": "kept.py", "status": "modified"},
  {"filename": "deleted.py", "status": "removed"},
  {"filename": "tests/added.py", "status": "added"}
]`

const cannedDiff = `diff --git a/first.py b/first.py
index 111..222 100644
--- a/first.py
+++ b/first.py
@@ -1,3 +1,4 @@
 unchanged
-removed
+added
+added again
diff --git a/old_name.py b/second/new_name.py
similarity index 90%
rename from old_name.py
rename to second/new_name.py
--- a/old_name.py
+++ b/second/new_name.py
@@ -1 +1 @@
--- looks like a header
+++ looks like a header
`

var diffFetches int

var savedInputs []string

// recordCommands replaces execute so that no test reaches GitHub or the browser.
func recordCommands(t *testing.T) *[][]string {
	t.Helper()
	recorded := &[][]string{}
	diffFetches = 0
	savedInputs = nil
	original := execute
	execute = func(arguments ...string) ([]byte, error) {
		if len(arguments) > 4 && arguments[2] == "graphql" && strings.Contains(arguments[4], "statusCheckRollup") {
			return []byte(cannedCIStates), nil
		}
		if len(arguments) > 4 && arguments[2] == "graphql" && strings.Contains(arguments[4], "additions") {
			return []byte(cannedLineCounts), nil
		}
		if len(arguments) > 2 && arguments[2] == "graphql" {
			return []byte(cannedResponse), nil
		}
		if len(arguments) == 3 && arguments[1] == "api" && strings.Contains(arguments[2], "/files") {
			return []byte(cannedChangedFiles), nil
		}
		if len(arguments) == 3 && arguments[1] == "api" && strings.Contains(arguments[2], "/comments") {
			return []byte(cannedComments), nil
		}
		if len(arguments) == 3 && arguments[1] == "api" {
			return []byte(`{"body": "Original description\r\n\r\nSecond paragraph"}`), nil
		}
		if index := slices.Index(arguments, "--input"); index >= 0 {
			input, _ := os.ReadFile(arguments[index+1])
			savedInputs = append(savedInputs, string(input))
			*recorded = append(*recorded, arguments[:index])
			return nil, nil
		}
		if len(arguments) > 2 && arguments[1] == "pr" && arguments[2] == "diff" {
			diffFetches++
			return []byte(cannedDiff), nil
		}
		*recorded = append(*recorded, arguments)
		return nil, nil
	}
	t.Cleanup(func() { execute = original })
	return recorded
}

func settle(t *testing.T, m model, command tea.Cmd) model {
	t.Helper()
	if command == nil {
		return m
	}
	message := command()
	if batch, isBatch := message.(tea.BatchMsg); isBatch {
		for _, batched := range batch {
			m = settle(t, m, batched)
		}
		return m
	}
	if message == nil {
		return m
	}
	next, nextCommand := m.Update(message)
	return settle(t, next.(model), nextCommand)
}

func loadedBoard(t *testing.T) model {
	t.Helper()
	return settle(t, newModel(newFilters(nil, false)), load)
}

func keyMessage(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func press(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, key := range keys {
		next, command := m.Update(keyMessage(key))
		m = settle(t, next.(model), command)
	}
	return m
}

func visibleLabels(m model) []string {
	labels := []string{}
	for _, pr := range m.visible {
		labels = append(labels, pr.label())
	}
	return labels
}

func assertLabels(t *testing.T, m model, expected ...string) {
	t.Helper()
	if labels := visibleLabels(m); !reflect.DeepEqual(labels, expected) {
		t.Fatalf("visible pull requests = %v, expected %v", labels, expected)
	}
}

func TestBoardShowsPullRequestsFromPeopleNewestFirst(t *testing.T) {
	recordCommands(t)

	m := loadedBoard(t)

	assertLabels(t, m, "gazebo #1", "u2 #3")
}

func TestFilterMenuSwitchesToBots(t *testing.T) {
	recordCommands(t)

	m := press(t, loadedBoard(t), "f", "b", "esc")

	assertLabels(t, m, "gazebo #2")
	if m.mode != boardMode {
		t.Fatalf("escape should close the filter menu")
	}
}

func TestFilterMenuTogglesRepository(t *testing.T) {
	recordCommands(t)

	m := press(t, loadedBoard(t), "f", "g")

	assertLabels(t, m, "u2 #3")
}

func TestRepositoryKeyDoesNothingOutsideFilterMenu(t *testing.T) {
	recordCommands(t)

	m := press(t, loadedBoard(t), "g", "u", "b")

	assertLabels(t, m, "gazebo #1", "u2 #3")
}

func TestBoardActionsAreIgnoredWhileFilterMenuIsOpen(t *testing.T) {
	recorded := recordCommands(t)

	m := press(t, loadedBoard(t), "f", "d", "o", "e")

	if m.mode != filterMode || len(*recorded) != 0 {
		t.Fatalf("mode = %v, commands = %v", m.mode, *recorded)
	}
}

func TestOpenRunsOpenWithTheSelectedUrl(t *testing.T) {
	recorded := recordCommands(t)

	press(t, loadedBoard(t), "down", "o")

	expected := [][]string{{"open", "https://example.test/u2/3"}}
	if !reflect.DeepEqual(*recorded, expected) {
		t.Fatalf("commands = %v, expected %v", *recorded, expected)
	}
}

func TestDraftToggleAsksBeforeMarkingReady(t *testing.T) {
	recorded := recordCommands(t)

	m := press(t, loadedBoard(t), "d")

	if m.mode != confirmMode || m.question != "Mark gazebo #1 ready for review?" {
		t.Fatalf("mode = %v, question = %q", m.mode, m.question)
	}
	if len(*recorded) != 0 {
		t.Fatalf("nothing should run before confirmation, ran %v", *recorded)
	}
}

func TestDecliningDraftToggleRunsNothing(t *testing.T) {
	recorded := recordCommands(t)

	m := press(t, loadedBoard(t), "d", "n")

	if m.mode != boardMode || len(*recorded) != 0 {
		t.Fatalf("mode = %v, commands = %v", m.mode, *recorded)
	}
}

func TestConfirmingDraftToggleMarksDraftReady(t *testing.T) {
	recorded := recordCommands(t)

	press(t, loadedBoard(t), "d", "y")

	expected := [][]string{{"gh", "pr", "ready", "https://example.test/gazebo/1"}}
	if !reflect.DeepEqual(*recorded, expected) {
		t.Fatalf("commands = %v, expected %v", *recorded, expected)
	}
}

func TestConfirmingDraftToggleConvertsReadyBackToDraft(t *testing.T) {
	recorded := recordCommands(t)

	press(t, loadedBoard(t), "down", "d", "enter")

	expected := [][]string{{"gh", "pr", "ready", "https://example.test/u2/3", "--undo"}}
	if !reflect.DeepEqual(*recorded, expected) {
		t.Fatalf("commands = %v, expected %v", *recorded, expected)
	}
}

func TestRequestingTeamReviewPostsTheTeamAsReviewer(t *testing.T) {
	recorded := recordCommands(t)

	press(t, loadedBoard(t), "e", "y")

	expected := [][]string{{
		"gh", "api", "--method", "POST", "repos/CoProcure/gazebo/pulls/1/requested_reviewers",
		"-f", "team_reviewers[]=engineering",
	}}
	if !reflect.DeepEqual(*recorded, expected) {
		t.Fatalf("commands = %v, expected %v", *recorded, expected)
	}
}

func TestCursorStaysOnTheSamePullRequestAfterRefresh(t *testing.T) {
	recordCommands(t)

	m := press(t, loadedBoard(t), "down", "r")

	selected, _ := m.selected()
	if selected.label() != "u2 #3" {
		t.Fatalf("selected = %s, expected u2 #3", selected.label())
	}
}

func TestReviewColumnNamesViewerAndTeamBeforeOtherReviewers(t *testing.T) {
	recordCommands(t)
	m := loadedBoard(t)

	review := plainText(m.visible[1].reviewSegments(m.viewer))

	if review != "you, engineering, someone-else" {
		t.Fatalf("review column = %q", review)
	}
}

func TestViewTruncatesLongTitlesToTheTerminalWidth(t *testing.T) {
	recordCommands(t)
	m := loadedBoard(t)
	m.pullRequests[0].title = strings.Repeat("long title ", 30)
	m.applyFilters()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})

	view := next.(model).View()

	for _, line := range strings.Split(view, "\n") {
		if width := len([]rune(line)); width > 100 {
			t.Fatalf("line is %d columns wide: %q", width, line)
		}
	}
}

func TestBoardShowsPullRequestsBeforeLineCountsArrive(t *testing.T) {
	recordCommands(t)

	next, _ := newModel(newFilters(nil, false)).Update(load())
	m := next.(model)

	assertLabels(t, m, "gazebo #1", "u2 #3")
	if lines := plainText(m.visible[0].columns(m.viewer)[4]); lines != "…" {
		t.Fatalf("lines column = %q before line counts arrive", lines)
	}
	if ci := m.visible[0].ciSegment().text; ci != "…" {
		t.Fatalf("CI column = %q before CI states arrive", ci)
	}
}

func TestCIColumnShowsStateOnceLoaded(t *testing.T) {
	recordCommands(t)

	m := loadedBoard(t)

	if failing, none := m.visible[0].ciSegment().text, m.visible[1].ciSegment().text; failing != "✗ failing" || none != "– none" {
		t.Fatalf("CI column = %q and %q", failing, none)
	}
}

func TestCIStatesSurviveARefresh(t *testing.T) {
	recordCommands(t)
	m := loadedBoard(t)

	next, _ := m.Update(load())

	if ci := next.(model).visible[0].ciSegment().text; ci != "✗ failing" {
		t.Fatalf("CI column = %q right after a refresh", ci)
	}
}

func TestLineCountsSurviveARefresh(t *testing.T) {
	recordCommands(t)
	m := loadedBoard(t)

	next, _ := m.Update(load())

	if lines := plainText(next.(model).visible[0].columns(m.viewer)[4]); lines != "+12 −3" {
		t.Fatalf("lines column = %q right after a refresh", lines)
	}
}

func TestLinesColumnShowsAdditionsAndDeletions(t *testing.T) {
	recordCommands(t)
	m := loadedBoard(t)

	lines := plainText(m.visible[0].columns(m.viewer)[4])

	if lines != "+12 −3" {
		t.Fatalf("lines column = %q", lines)
	}
}

func TestEnterOpensDiffViewListingChangedFiles(t *testing.T) {
	recorded := recordCommands(t)

	m := press(t, loadedBoard(t), "enter")

	if m.mode != diffMode || len(*recorded) != 0 {
		t.Fatalf("mode = %v, commands = %v", m.mode, *recorded)
	}
	expected := []string{"+2 −1 first.py", "+1 −1 second/new_name.py"}
	listed := []string{}
	for _, file := range m.diffFiles {
		listed = append(listed, plainText(file.listSegments(80)))
	}
	if !reflect.DeepEqual(listed, expected) {
		t.Fatalf("files = %v, expected %v", listed, expected)
	}
}

func TestDiffViewShowsOnlyTheSelectedFile(t *testing.T) {
	recordCommands(t)
	m := press(t, loadedBoard(t), "enter")

	first := m.View()
	second := press(t, m, "down").View()

	if !strings.Contains(first, "+added again") || strings.Contains(first, "looks like a header") {
		t.Fatalf("first file view:\n%s", first)
	}
	if strings.Contains(second, "+added again") || !strings.Contains(second, "+++ looks like a header") {
		t.Fatalf("second file view:\n%s", second)
	}
}

func TestDiffViewArrowKeysStopAtFirstAndLastFile(t *testing.T) {
	recordCommands(t)
	m := press(t, loadedBoard(t), "enter")

	atStart := press(t, m, "up")
	atEnd := press(t, m, "down", "down", "down")

	if atStart.fileCursor != 0 || atEnd.fileCursor != 1 {
		t.Fatalf("cursor at start = %d, at end = %d", atStart.fileCursor, atEnd.fileCursor)
	}
}

func TestEscapeReturnsFromDiffViewToTheSamePullRequest(t *testing.T) {
	recordCommands(t)

	m := press(t, loadedBoard(t), "down", "enter", "down", "esc")

	selected, _ := m.selected()
	if m.mode != boardMode || selected.label() != "u2 #3" {
		t.Fatalf("mode = %v, selected = %s", m.mode, selected.label())
	}
}

func TestQuitKeyLeavesDiffViewInsteadOfQuitting(t *testing.T) {
	recordCommands(t)
	m := press(t, loadedBoard(t), "enter")

	next, command := m.Update(keyMessage("q"))

	if next.(model).mode != boardMode || command != nil {
		t.Fatalf("q in the diff view should return to the board")
	}
}

func TestEditorKeyOpensTheWorktreeAndItsChangedFilesExceptDeletedOnes(t *testing.T) {
	recorded := recordCommands(t)
	home, _ := os.UserHomeDir()

	m := press(t, loadedBoard(t), "down", "v")

	expected := [][]string{{
		"wt", "-C", filepath.Join(home, "git", "u2"), "switch", "pr:3",
		"--no-cd", "-x", editorCommand, "--", ".", "kept.py", "tests/added.py",
	}}
	if !reflect.DeepEqual(*recorded, expected) {
		t.Fatalf("commands = %v, expected %v", *recorded, expected)
	}
	if m.notice != "" {
		t.Fatalf("notice = %q after the editor opened", m.notice)
	}
}

func TestReopeningADiffReusesTheCachedCopy(t *testing.T) {
	recordCommands(t)

	m := press(t, loadedBoard(t), "enter", "esc", "enter")

	if diffFetches != 1 || len(m.diffFiles) != 2 {
		t.Fatalf("diff fetched %d times, %d files shown", diffFetches, len(m.diffFiles))
	}
}

func TestReopeningADiffAfterANewCommitFetchesItAgain(t *testing.T) {
	recordCommands(t)
	m := press(t, loadedBoard(t), "enter", "esc")
	m.pullRequests[0].headCommit = "second-commit"
	m.applyFilters()

	press(t, m, "enter")

	if diffFetches != 2 {
		t.Fatalf("diff fetched %d times, expected 2", diffFetches)
	}
}

// editorWrites replaces the editor with one that saves the given text.
func editorWrites(t *testing.T, text string) {
	t.Helper()
	original := editInEditor
	editInEditor = func(path string, finished func(error) tea.Msg) tea.Cmd {
		return func() tea.Msg {
			if text != "" {
				os.WriteFile(path, []byte(text), 0o600)
			}
			return finished(nil)
		}
	}
	t.Cleanup(func() { editInEditor = original })
}

func TestDescriptionKeyShowsTheDescription(t *testing.T) {
	recordCommands(t)

	m := press(t, loadedBoard(t), "p")

	view := ansi.Strip(m.View())
	if m.mode != descriptionMode || !strings.Contains(view, "Original description") || !strings.Contains(view, "Second paragraph") {
		t.Fatalf("mode = %v, view:\n%s", m.mode, view)
	}
}

func TestEscapeReturnsFromDescriptionToWhereItWasOpened(t *testing.T) {
	recordCommands(t)

	fromBoard := press(t, loadedBoard(t), "p", "esc")
	fromDiff := press(t, loadedBoard(t), "enter", "p", "esc")

	if fromBoard.mode != boardMode || fromDiff.mode != diffMode {
		t.Fatalf("from board = %v, from diff = %v", fromBoard.mode, fromDiff.mode)
	}
}

func TestEditingTheDescriptionUpdatesThePullRequest(t *testing.T) {
	recorded := recordCommands(t)
	editorWrites(t, "Edited description\n")

	m := press(t, loadedBoard(t), "p", "e")

	expectedCommands := [][]string{{"gh", "api", "--method", "PATCH", "repos/CoProcure/gazebo/pulls/1"}}
	expectedInputs := []string{`{"body":"Edited description"}`}
	if !reflect.DeepEqual(*recorded, expectedCommands) || !reflect.DeepEqual(savedInputs, expectedInputs) {
		t.Fatalf("commands = %v, inputs = %v", *recorded, savedInputs)
	}
	if m.descriptionBody != "Edited description" || m.descriptionNotice != "Description updated." {
		t.Fatalf("body = %q, notice = %q", m.descriptionBody, m.descriptionNotice)
	}
}

func TestClosingTheEditorWithoutChangesUpdatesNothing(t *testing.T) {
	recorded := recordCommands(t)
	editorWrites(t, "")

	m := press(t, loadedBoard(t), "p", "e")

	if len(*recorded) != 0 || m.descriptionNotice != "Description unchanged." {
		t.Fatalf("commands = %v, notice = %q", *recorded, m.descriptionNotice)
	}
}

func TestSavingAnEmptyDescriptionUpdatesNothing(t *testing.T) {
	recorded := recordCommands(t)
	editorWrites(t, "\n")

	m := press(t, loadedBoard(t), "p", "e")

	if len(*recorded) != 0 || m.descriptionNotice != "Description unchanged." {
		t.Fatalf("commands = %v, notice = %q", *recorded, m.descriptionNotice)
	}
}

func TestHighlightingKeepsEveryCharacterOfTheDiffLine(t *testing.T) {
	files := parseDiff(cannedDiff)

	rendered := []string{}
	for _, line := range files[0].lines {
		rendered = append(rendered, strings.TrimRight(line.render(80), " "))
	}

	expected := []string{
		"index 111..222 100644", "--- a/first.py", "+++ b/first.py", "@@ -1,3 +1,4 @@",
		" unchanged", "-removed", "+added", "+added again",
	}
	if !reflect.DeepEqual(rendered, expected) {
		t.Fatalf("rendered = %q", rendered)
	}
	if files[0].lines[4].code == nil {
		t.Fatalf("a Python file should be tokenised for highlighting")
	}
}

func TestSplitViewPutsRemovedLineBesideItsReplacement(t *testing.T) {
	file := parseDiff(cannedDiff)[0]

	rows := []string{}
	for _, row := range file.splitRows[4:] {
		rows = append(rows, strings.Join(strings.Fields(file.renderSplitRow(row, 60, " │ ")), " "))
	}

	expected := []string{"1 unchanged │ 1 unchanged", "2 -removed │ 2 +added", "│ 3 +added again"}
	if !reflect.DeepEqual(rows, expected) {
		t.Fatalf("rows = %q", rows)
	}
}

func TestDiffViewStartsSplitAndTogglesToUnified(t *testing.T) {
	recordCommands(t)
	m := press(t, loadedBoard(t), "enter")

	split := m.View()
	unified := press(t, m, "s").View()

	if !strings.Contains(split, "-removed") || !strings.Contains(split, "2 +added") {
		t.Fatalf("split view:\n%s", split)
	}
	if strings.Contains(unified, "2 +added") || !strings.Contains(unified, "+added again") {
		t.Fatalf("unified view:\n%s", unified)
	}
}

func TestCommentPostsOnTheNewLineUnderTheCursor(t *testing.T) {
	recorded := recordCommands(t)

	m := press(t, loadedBoard(t), "enter", "j", "j", "j", "j", "j", "c", "Looks good", "ctrl+s")

	expectedCommands := [][]string{{"gh", "api", "--method", "POST", "repos/CoProcure/gazebo/pulls/1/comments"}}
	expectedInputs := []string{`{"body":"Looks good","commit_id":"first-commit","line":2,"path":"first.py","side":"RIGHT"}`}
	if !reflect.DeepEqual(*recorded, expectedCommands) || !reflect.DeepEqual(savedInputs, expectedInputs) {
		t.Fatalf("commands = %v, inputs = %v", *recorded, savedInputs)
	}
	if m.mode != diffMode || m.diffNotice != "Comment posted on first.py line 2 (new)." {
		t.Fatalf("mode = %v, notice = %q", m.mode, m.diffNotice)
	}
}

func TestTabMovesTheCommentToTheRemovedLine(t *testing.T) {
	recordCommands(t)

	press(t, loadedBoard(t), "enter", "j", "j", "j", "j", "j", "tab", "c", "Why remove this?", "ctrl+s")

	expectedInputs := []string{`{"body":"Why remove this?","commit_id":"first-commit","line":2,"path":"first.py","side":"LEFT"}`}
	if !reflect.DeepEqual(savedInputs, expectedInputs) {
		t.Fatalf("inputs = %v", savedInputs)
	}
}

func TestCommentBoxLeavesTheCodeOnScreen(t *testing.T) {
	recordCommands(t)

	view := ansi.Strip(press(t, loadedBoard(t), "enter", "j", "j", "j", "j", "j", "c", "Draft text").View())

	for _, expected := range []string{"-removed", "+added", "Comment on first.py line 2 (new)", "Draft text", "ctrl+s post"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view is missing %q:\n%s", expected, view)
		}
	}
}

func TestCommentKeyOnAHeaderLinePostsNothing(t *testing.T) {
	recorded := recordCommands(t)

	m := press(t, loadedBoard(t), "enter", "c")

	if m.mode != diffMode || len(*recorded) != 0 || m.diffNotice == "" {
		t.Fatalf("mode = %v, commands = %v, notice = %q", m.mode, *recorded, m.diffNotice)
	}
}

func TestEscapeDiscardsTheCommentWithoutPosting(t *testing.T) {
	recorded := recordCommands(t)

	m := press(t, loadedBoard(t), "enter", "j", "j", "j", "j", "j", "c", "Never mind", "esc")

	if m.mode != diffMode || len(*recorded) != 0 {
		t.Fatalf("mode = %v, commands = %v", m.mode, *recorded)
	}
}

func TestTypingBoardKeysInTheCommentBoxOnlyTypes(t *testing.T) {
	recorded := recordCommands(t)

	m := press(t, loadedBoard(t), "enter", "j", "j", "j", "j", "j", "c", "q", "o", "v", "d")

	if m.mode != commentMode || m.commentInput.Value() != "qovd" || len(*recorded) != 0 {
		t.Fatalf("mode = %v, text = %q, commands = %v", m.mode, m.commentInput.Value(), *recorded)
	}
}

func TestDiffViewShowsACommentThreadUnderItsLine(t *testing.T) {
	recordCommands(t)

	lines := strings.Split(ansi.Strip(press(t, loadedBoard(t), "enter").View()), "\n")

	added := slices.IndexFunc(lines, func(line string) bool { return strings.Contains(line, "2 +added") })
	if added < 0 {
		t.Fatalf("view has no added line:\n%s", strings.Join(lines, "\n"))
	}
	for offset, expected := range []string{"┌ reviewer", "│ Why this change?", "├ person", "│ Because it was wrong.", "3 +added again"} {
		if !strings.Contains(lines[added+1+offset], expected) {
			t.Fatalf("row %d after the added line = %q, expected %q", offset+1, lines[added+1+offset], expected)
		}
	}
}

func TestDiffViewLeavesOutOutdatedComments(t *testing.T) {
	recordCommands(t)

	view := press(t, loadedBoard(t), "enter").View()

	if strings.Contains(view, "Outdated remark") {
		t.Fatalf("an outdated comment was shown:\n%s", view)
	}
}

func TestCommentKeyOnAThreadRepliesToIt(t *testing.T) {
	recordCommands(t)

	m := press(t, loadedBoard(t), "enter", "j", "j", "j", "j", "j", "j", "j", "c")
	title := ansi.Strip(m.View())
	press(t, m, "Thanks", "ctrl+s")

	if !strings.Contains(title, "Reply to reviewer on first.py line 2 (new)") {
		t.Fatalf("comment box:\n%s", title)
	}
	expectedInputs := []string{`{"body":"Thanks","in_reply_to":10}`}
	if !reflect.DeepEqual(savedInputs, expectedInputs) {
		t.Fatalf("inputs = %v", savedInputs)
	}
}

func TestCursorStaysOnTheSameLineWhenTheLayoutChanges(t *testing.T) {
	recordCommands(t)
	m := press(t, loadedBoard(t), "enter", "j", "j", "j", "j", "j")

	unified := press(t, m, "s")
	backToSplit := press(t, unified, "s")

	if unified.cursorLine() != 6 || backToSplit.cursorLine() != 6 {
		t.Fatalf("cursor line unified = %d, split = %d, expected 6", unified.cursorLine(), backToSplit.cursorLine())
	}
}
