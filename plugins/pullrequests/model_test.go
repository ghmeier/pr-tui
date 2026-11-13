package pullrequests

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/config"
)

// settingsWithEditorCommand replaces the executable and arguments the editor is run with.
func settingsWithEditorCommand(command ...string) Settings {
	settings := testSettings()
	settings.Editor = config.Editor{Name: "Test Editor", Command: command}
	return settings
}

func TestBoardShowsPullRequestsFromPeopleInTheOrderTheProviderReturnsThem(t *testing.T) {
	provider := newFakeProvider()

	m := loadedBoardWith(t, provider)

	assertShown(t, m, provider.pullRequest(1), provider.pullRequest(3))
}

func TestBoardShowsTheProviderFailureWhenTheRefreshFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("PullRequests", errorForTests)

	m := loadedBoardWith(t, provider)

	if m.notice != "Refresh failed: "+errorForTests.Error() {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestFilterMenuSwitchesToBots(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), "f", "b", "esc")

	assertShown(t, m, provider.pullRequest(2))
	if m.mode != boardMode {
		t.Fatalf("escape should close the filter menu")
	}
}

func TestFilterMenuTogglesRepositoryByNumberKey(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), "f", "1")

	assertShown(t, m, provider.pullRequest(3))
}

func TestNumberKeysDoNothingOutsideFilterMenu(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), "1", "2", "b")

	assertShown(t, m, provider.pullRequest(1), provider.pullRequest(3))
}

func TestBoardActionsAreIgnoredWhileFilterMenuIsOpen(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)

	m := press(t, loadedBoardWith(t, provider), "f", "d", "o", "e")

	if m.mode != filterMode {
		t.Fatalf("mode = %v", m.mode)
	}
	assertCalls(t, provider)
	assertCommands(t, commands)
}

func TestOpenRunsOpenWithTheSelectedUrl(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)

	press(t, loadedBoardWith(t, provider), "down", "o")

	assertCommands(t, commands, []string{"open", provider.pullRequest(3).URL})
}

func TestDraftKeyMarksDraftReadyWithoutAsking(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "d")

	assertCalls(t, provider, draftCall(provider.pullRequest(1), false))
}

func TestDraftKeyConvertsReadyBackToDraftWithoutAsking(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "down", "d")

	assertCalls(t, provider, draftCall(provider.pullRequest(3), true))
}

func TestDraftKeyShowsTheFailureWhenTheProviderRefuses(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SetDraft", errorForTests)

	m := press(t, loadedBoardWith(t, provider), "d")

	if m.notice != "Failed: "+errorForTests.Error() {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestReviewKeyOnADraftMarksItReadyThenRequestsTheTeam(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "e")

	draft := provider.pullRequest(1)
	assertCalls(t, provider, draftCall(draft, false), teamReviewCall(draft))
}

func TestReviewKeyOnAReadyPullRequestOnlyRequestsTheTeam(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), "down", "e")

	assertCalls(t, provider, teamReviewCall(provider.pullRequest(3)))
}

func TestReviewKeyDoesNotRequestTheTeamWhenMarkingReadyFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SetDraft", errorForTests)

	m := press(t, loadedBoardWith(t, provider), "e")

	assertCalls(t, provider, draftCall(provider.pullRequest(1), false))
	if m.notice != "Failed: "+errorForTests.Error() {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestReviewKeyDoesNothingWithoutAReviewTeam(t *testing.T) {
	provider := newFakeProvider()
	provider.reviewTeam = ""

	m := press(t, loadedBoardWith(t, provider), "e")

	assertCalls(t, provider)
	if m.notice != "" {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestCursorStaysOnTheSamePullRequestAfterRefresh(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), "down", "r")

	selected, _ := m.selected()
	if expected := provider.pullRequest(3); selected.label() != expected.label() {
		t.Fatalf("selected = %s, expected %s", selected.label(), expected.label())
	}
}

func TestViewTruncatesLongTitlesToTheTerminalWidth(t *testing.T) {
	m := loadedBoard(t)
	m.pullRequests[0].Title = strings.Repeat("long title ", 30)
	m.applyFilters()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})

	view := next.(Model).View()

	for _, line := range strings.Split(view, "\n") {
		if width := len([]rune(line)); width > 100 {
			t.Fatalf("line is %d columns wide: %q", width, line)
		}
	}
}

func TestBoardShowsPullRequestsBeforeLineCountsArrive(t *testing.T) {
	provider := newFakeProvider()
	board := boardFor(provider, testSettings())

	next, _ := board.Update(board.load()())
	m := next.(Model)

	assertShown(t, m, provider.pullRequest(1), provider.pullRequest(3))
	if lines := linesColumn(m, 0); lines != "…" {
		t.Fatalf("lines column = %q before line counts arrive", lines)
	}
	if state := m.visible[0].stateMarks(m.viewer, m.reviewTeam()); state != "📝 …" {
		t.Fatalf("state column = %q before CI states arrive", state)
	}
}

func TestStateColumnShowsCIOnceLoaded(t *testing.T) {
	m := loadedBoard(t)

	failing := m.visible[0].stateMarks(m.viewer, m.reviewTeam())
	none := m.visible[1].stateMarks(m.viewer, m.reviewTeam())

	if failing != "📝 🔴" || none != "✋ ⏩" {
		t.Fatalf("state column = %q and %q", failing, none)
	}
}

func TestCIStatesSurviveARefresh(t *testing.T) {
	m := loadedBoard(t)

	next, _ := m.Update(m.load()())

	if state := next.(Model).visible[0].stateMarks(m.viewer, m.reviewTeam()); state != "📝 🔴" {
		t.Fatalf("state column = %q right after a refresh", state)
	}
}

// expectedLines is how the board writes the line count that the fake provider has for its
// first pull request.
func expectedLines(provider *fakeProvider) string {
	count := provider.lineCounts[provider.pullRequest(1).URL]
	return fmt.Sprintf("+%d −%d", count.Additions, count.Deletions)
}

func TestLineCountsSurviveARefresh(t *testing.T) {
	provider := newFakeProvider()
	m := loadedBoardWith(t, provider)

	next, _ := m.Update(m.load()())

	if lines := linesColumn(next.(Model), 0); lines != expectedLines(provider) {
		t.Fatalf("lines column = %q right after a refresh", lines)
	}
}

func TestLinesColumnShowsAdditionsAndDeletions(t *testing.T) {
	provider := newFakeProvider()
	m := loadedBoardWith(t, provider)

	if lines := linesColumn(m, 0); lines != expectedLines(provider) {
		t.Fatalf("lines column = %q", lines)
	}
}

func TestLinesColumnStaysUnknownWhenTheProviderCannotCountLines(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("LineCounts", errorForTests)

	m := loadedBoardWith(t, provider)

	if lines := linesColumn(m, 0); lines != "…" {
		t.Fatalf("lines column = %q", lines)
	}
}

func TestEnterOpensDiffViewListingChangedFiles(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), "enter")

	if m.mode != diffMode {
		t.Fatalf("mode = %v", m.mode)
	}
	assertCalls(t, provider)
	expected := []string{"+2 −1 first.py", "+1 −1 second/new_name.py"}
	listed := []string{}
	for _, file := range m.diffFiles {
		listed = append(listed, filePlainText(file))
	}
	if strings.Join(listed, "|") != strings.Join(expected, "|") {
		t.Fatalf("files = %v, expected %v", listed, expected)
	}
}

func TestDiffViewShowsOnlyTheSelectedFile(t *testing.T) {
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
	m := press(t, loadedBoard(t), "enter")

	atStart := press(t, m, "up")
	atEnd := press(t, m, "down", "down", "down")

	if atStart.fileCursor != 0 || atEnd.fileCursor != 1 {
		t.Fatalf("cursor at start = %d, at end = %d", atStart.fileCursor, atEnd.fileCursor)
	}
}

func TestDiffViewReportsAFailureToLoadTheDiffAndReturnsToTheBoard(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("Diff", errorForTests)

	m := press(t, loadedBoardWith(t, provider), "enter")

	if m.mode != boardMode || m.notice != "Diff failed: "+errorForTests.Error() {
		t.Fatalf("mode = %v, notice = %q", m.mode, m.notice)
	}
}

func TestEscapeReturnsFromDiffViewToTheSamePullRequest(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), "down", "enter", "down", "esc")

	selected, _ := m.selected()
	if expected := provider.pullRequest(3); m.mode != boardMode || selected.label() != expected.label() {
		t.Fatalf("mode = %v, selected = %s, expected %s", m.mode, selected.label(), expected.label())
	}
}

func TestQuitKeyLeavesDiffViewInsteadOfQuitting(t *testing.T) {
	m := press(t, loadedBoard(t), "enter")

	next, command := m.Update(keyMessage("q"))

	if next.(Model).mode != boardMode || command != nil {
		t.Fatalf("q in the diff view should return to the board")
	}
}

func TestEditorKeyOpensTheWorktreeAndTheProvidersChangedFiles(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)
	settings := testSettings()

	m := press(t, loadedBoardFor(t, provider, settings), "down", "v")

	assertCommands(t, commands, append([]string{
		"wt", "-C", filepath.Join(settings.CloneRoot, provider.pullRequest(3).Repository.Name), "switch", "pr:3",
		"--no-cd", "-x", settings.Editor.Command[0], "--", settings.Editor.Command[1], ".",
	}, provider.changedFiles...))
	if m.notice != "" {
		t.Fatalf("notice = %q after the editor opened", m.notice)
	}
}

func TestEditorKeyStillOpensTheWorktreeWhenTheChangedFilesCannotBeListed(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("ChangedFiles", errorForTests)
	commands := recordShell(t)
	settings := testSettings()

	press(t, loadedBoardFor(t, provider, settings), "down", "v")

	assertCommands(t, commands, []string{
		"wt", "-C", filepath.Join(settings.CloneRoot, provider.pullRequest(3).Repository.Name), "switch", "pr:3",
		"--no-cd", "-x", settings.Editor.Command[0], "--", settings.Editor.Command[1], ".",
	})
}

func TestEditorKeyPassesAStringCommandAsTheOnlyProgramWordEvenWithSpaces(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)
	settings := settingsWithEditorCommand("/Applications/Visual Studio Code - Insiders.app/Contents/Resources/app/bin/code")

	press(t, loadedBoardFor(t, provider, settings), "down", "v")

	assertCommands(t, commands, append([]string{
		"wt", "-C", filepath.Join(settings.CloneRoot, provider.pullRequest(3).Repository.Name), "switch", "pr:3",
		"--no-cd", "-x", settings.Editor.Command[0], "--", ".",
	}, provider.changedFiles...))
}

func TestEditorKeyShowsTheFailureWhenTheWorktreeCannotBeOpened(t *testing.T) {
	failShell(t, errorForTests)

	m := press(t, loadedBoard(t), "v")

	if m.notice != "Failed: "+errorForTests.Error() {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestReopeningADiffReusesTheCachedCopy(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), "enter", "esc", "enter")

	if provider.fetchesOfTheDiff() != 1 || len(m.diffFiles) != 2 {
		t.Fatalf("diff fetched %d times, %d files shown", provider.fetchesOfTheDiff(), len(m.diffFiles))
	}
}

func TestReopeningADiffAfterANewCommitFetchesItAgain(t *testing.T) {
	provider := newFakeProvider()
	m := press(t, loadedBoardWith(t, provider), "enter", "esc")
	m.pullRequests[0].HeadCommit = "second-commit"
	m.applyFilters()

	press(t, m, "enter")

	if provider.fetchesOfTheDiff() != 2 {
		t.Fatalf("diff fetched %d times, expected 2", provider.fetchesOfTheDiff())
	}
}

func TestDescriptionKeyShowsTheDescription(t *testing.T) {
	m := press(t, loadedBoard(t), "p")

	view := ansi.Strip(m.View())
	if m.mode != descriptionMode || !strings.Contains(view, "Original description") || !strings.Contains(view, "Second paragraph") {
		t.Fatalf("mode = %v, view:\n%s", m.mode, view)
	}
}

func TestEscapeReturnsFromDescriptionToWhereItWasOpened(t *testing.T) {
	fromBoard := press(t, loadedBoard(t), "p", "esc")
	fromDiff := press(t, loadedBoard(t), "enter", "p", "esc")

	if fromBoard.mode != boardMode || fromDiff.mode != diffMode {
		t.Fatalf("from board = %v, from diff = %v", fromBoard.mode, fromDiff.mode)
	}
}

func TestEditingTheDescriptionUpdatesThePullRequest(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Edited description\n")

	m := press(t, loadedBoardWith(t, provider), "p", "e")

	assertCalls(t, provider, saveDescriptionCall(provider.pullRequest(1), "Edited description"))
	if m.descriptionBody != "Edited description" || m.descriptionNotice != "Description updated." {
		t.Fatalf("body = %q, notice = %q", m.descriptionBody, m.descriptionNotice)
	}
}

func TestEditingTheDescriptionKeepsTheEditWhenTheProviderRefusesIt(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SaveDescription", errorForTests)
	editorWrites(t, "Edited description\n")

	m := press(t, loadedBoardWith(t, provider), "p", "e")

	if !strings.Contains(m.descriptionNotice, errorForTests.Error()) || !strings.Contains(m.descriptionNotice, "your edit is kept at") {
		t.Fatalf("notice = %q", m.descriptionNotice)
	}
}

func TestClosingTheEditorWithoutChangesUpdatesNothing(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "")

	m := press(t, loadedBoardWith(t, provider), "p", "e")

	assertCalls(t, provider)
	if m.descriptionNotice != "Description unchanged." {
		t.Fatalf("notice = %q", m.descriptionNotice)
	}
}

func TestSavingAnEmptyDescriptionUpdatesNothing(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "\n")

	m := press(t, loadedBoardWith(t, provider), "p", "e")

	assertCalls(t, provider)
	if m.descriptionNotice != "Description unchanged." {
		t.Fatalf("notice = %q", m.descriptionNotice)
	}
}

func TestDiffViewStartsSplitAndTogglesToUnified(t *testing.T) {
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

// pressToTheAddedLine opens the first pull request's diff and moves to the added line "added".
func pressToTheAddedLine() []string {
	return []string{"enter", "j", "j", "j", "j", "j"}
}

func TestCommentPostsOnTheNewLineUnderTheCursor(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), append(pressToTheAddedLine(), "c", "Looks good", "ctrl+s")...)

	target := CommentTarget{Path: "first.py", Line: 2, Side: CommentSideNew}
	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), target, "Looks good"))
	if m.mode != diffMode || m.diffNotice != "Comment posted on first.py line 2 (new)." {
		t.Fatalf("mode = %v, notice = %q", m.mode, m.diffNotice)
	}
}

func TestTabMovesTheCommentToTheRemovedLine(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), append(pressToTheAddedLine(), "tab", "c", "Why remove this?", "ctrl+s")...)

	target := CommentTarget{Path: "first.py", Line: 2, Side: CommentSideOld}
	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), target, "Why remove this?"))
}

func TestCommentBoxLeavesTheCodeOnScreen(t *testing.T) {
	view := ansi.Strip(press(t, loadedBoard(t), append(pressToTheAddedLine(), "c", "Draft text")...).View())

	for _, expected := range []string{"-removed", "+added", "Comment on first.py line 2 (new)", "Draft text", "ctrl+s post"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view is missing %q:\n%s", expected, view)
		}
	}
}

func TestCommentKeyOnAHeaderLinePostsNothing(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), "enter", "c")

	assertCalls(t, provider)
	if m.mode != diffMode || m.diffNotice == "" {
		t.Fatalf("mode = %v, notice = %q", m.mode, m.diffNotice)
	}
}

func TestCommentBoxShowsTheFailureWhenThePostFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("PostComment", errorForTests)

	m := press(t, loadedBoardWith(t, provider), append(pressToTheAddedLine(), "c", "Looks good", "ctrl+s")...)

	if m.mode != commentMode || m.commentError != errorForTests.Error() {
		t.Fatalf("mode = %v, comment error = %q", m.mode, m.commentError)
	}
}

func TestEscapeDiscardsTheCommentWithoutPosting(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), append(pressToTheAddedLine(), "c", "Never mind", "esc")...)

	assertCalls(t, provider)
	if m.mode != diffMode {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestTypingBoardKeysInTheCommentBoxOnlyTypes(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)

	m := press(t, loadedBoardWith(t, provider), append(pressToTheAddedLine(), "c", "q", "o", "v", "d")...)

	if m.mode != commentMode || m.commentInput.Value() != "qovd" {
		t.Fatalf("mode = %v, text = %q", m.mode, m.commentInput.Value())
	}
	assertCalls(t, provider)
	assertCommands(t, commands)
}

func TestDiffViewShowsACommentThreadUnderItsLine(t *testing.T) {
	lines := strings.Split(ansi.Strip(press(t, loadedBoard(t), "enter").View()), "\n")

	added := -1
	for index, line := range lines {
		if strings.Contains(line, "2 +added") {
			added = index
		}
	}
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
	view := press(t, loadedBoard(t), "enter").View()

	if strings.Contains(view, "Outdated remark") {
		t.Fatalf("an outdated comment was shown:\n%s", view)
	}
}

func TestCommentKeyOnAThreadRepliesToIt(t *testing.T) {
	provider := newFakeProvider()
	m := press(t, loadedBoardWith(t, provider), "enter", "j", "j", "j", "j", "j", "j", "j", "c")

	title := ansi.Strip(m.View())
	press(t, m, "Thanks", "ctrl+s")

	if !strings.Contains(title, "Reply to reviewer on first.py line 2 (new)") {
		t.Fatalf("comment box:\n%s", title)
	}
	target := CommentTarget{Path: "first.py", Line: 2, Side: CommentSideNew, ReplyTo: provider.comments[0].ID, ReplyAuthor: provider.comments[0].Author}
	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), target, "Thanks"))
}

func TestCursorStaysOnTheSameLineWhenTheLayoutChanges(t *testing.T) {
	m := press(t, loadedBoard(t), pressToTheAddedLine()...)
	line := m.cursorLine()

	unified := press(t, m, "s")
	backToSplit := press(t, unified, "s")

	if unified.cursorLine() != line || backToSplit.cursorLine() != line {
		t.Fatalf("cursor line unified = %d, split = %d, expected %d", unified.cursorLine(), backToSplit.cursorLine(), line)
	}
}

func TestAutoMergeKeyEnablesAutoMergeAfterConfirmation(t *testing.T) {
	provider := newFakeProvider()

	asked := press(t, loadedBoardWith(t, provider), "m")
	assertCalls(t, provider)
	press(t, asked, "y")

	selected := provider.pullRequest(1)
	if asked.mode != confirmMode || !strings.HasPrefix(asked.question, "Enable auto-merge (squash) on "+selected.label()+"?") {
		t.Fatalf("mode = %v, question = %q", asked.mode, asked.question)
	}
	assertCalls(t, provider, autoMergeCall(selected, true))
}

func TestAutoMergeKeyDisablesAutoMergeWhenAlreadyEnabled(t *testing.T) {
	provider := newFakeProvider()

	asked := press(t, loadedBoardWith(t, provider), "down", "m")
	press(t, asked, "y")

	selected := provider.pullRequest(3)
	if asked.question != "Disable auto-merge on "+selected.label()+"?" {
		t.Fatalf("question = %q", asked.question)
	}
	assertCalls(t, provider, autoMergeCall(selected, false))
}

func TestDecliningAutoMergeChangesNothing(t *testing.T) {
	provider := newFakeProvider()

	m := press(t, loadedBoardWith(t, provider), "m", "n")

	assertCalls(t, provider)
	if m.mode != boardMode || m.pendingAction != nil {
		t.Fatalf("mode = %v, an action is still pending: %v", m.mode, m.pendingAction != nil)
	}
}

func TestHelpOffersTheReviewKeyOnlyWithAReviewTeam(t *testing.T) {
	without := newFakeProvider()
	without.reviewTeam = ""

	withoutHelp := ansi.Strip(boardFor(without, testSettings()).help())
	withHelp := ansi.Strip(boardFor(newFakeProvider(), testSettings()).help())

	if strings.Contains(withoutHelp, "request") {
		t.Fatalf("help without a review team mentions a request: %s", withoutHelp)
	}
	if !strings.Contains(withHelp, "e request "+testReviewTeam) {
		t.Fatalf("help with a review team = %s", withHelp)
	}
}

func TestHelpNamesTheConfiguredEditor(t *testing.T) {
	settings := testSettings()

	help := ansi.Strip(boardFor(newFakeProvider(), settings).help())

	if expected := "v " + settings.Editor.Name; !strings.Contains(help, expected) {
		t.Fatalf("help = %s, expected it to contain %q", help, expected)
	}
}

func TestStatusLineShowsTheConfiguredRefreshInterval(t *testing.T) {
	settings := testSettings()

	status := ansi.Strip(loadedBoardFor(t, newFakeProvider(), settings).statusLine())

	expected := fmt.Sprintf("refreshes every %d minutes", int(settings.RefreshInterval.Minutes()))
	if !strings.Contains(status, expected) {
		t.Fatalf("status = %q, expected it to contain %q", status, expected)
	}
}

func TestStatusLineReadsEverySecondForAOneSecondInterval(t *testing.T) {
	settings := testSettings()
	settings.RefreshInterval = time.Second

	status := ansi.Strip(loadedBoardFor(t, newFakeProvider(), settings).statusLine())

	if !strings.Contains(status, "refreshes every second") {
		t.Fatalf("status = %q", status)
	}
}
