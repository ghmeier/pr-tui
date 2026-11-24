package pullrequests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
	"github.com/ghmeier/pr-tui/internal/ui"
)

func TestEnterOpensTheDiffListingTheChangedFilesWithoutChangingAnything(t *testing.T) {
	provider := newFakeProvider()

	model := loadedBoardWith(t, provider)
	press(t, model, "enter")

	for _, expected := range []string{"+2 −1 first.py", "+1 −1 second/new_name.py", "file 1 of 2"} {
		if !strings.Contains(view(model), expected) {
			t.Fatalf("diff view is missing %q:\n%s", expected, view(model))
		}
	}
	assertCalls(t, provider)
}

func TestDiffHeaderNamesThePullRequestAndShowsItsLoadedLineCount(t *testing.T) {
	provider := newFakeProvider()
	selected := provider.pullRequest(1)

	model := loadedBoardWith(t, provider)
	press(t, model, "enter")

	header := strings.Split(view(model), "\n")[0]
	for _, expected := range []string{selected.label(), selected.Title, expectedLines(provider)} {
		if !strings.Contains(header, expected) {
			t.Fatalf("header %q should contain %q", header, expected)
		}
	}
}

func TestDiffHeaderShowsAPlaceholderForALineCountThatIsNotLoaded(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("LineCounts", errorForTests)
	selected := provider.pullRequest(1)

	model := loadedBoardWith(t, provider)
	press(t, model, "enter")

	header := strings.Split(view(model), "\n")[0]
	if !strings.HasSuffix(strings.TrimSpace(header), "…") || strings.Contains(header, expectedLines(provider)) || !strings.Contains(header, selected.Title) {
		t.Fatalf("header = %q", header)
	}
}

func TestDiffViewShowsOnlyTheSelectedFile(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter")

	first := view(model)
	press(t, model, "down")
	second := view(model)

	if !strings.Contains(first, "+added again") || strings.Contains(first, "looks like a header") {
		t.Fatalf("first file view:\n%s", first)
	}
	if strings.Contains(second, "+added again") || !strings.Contains(second, "+++ looks like a header") {
		t.Fatalf("second file view:\n%s", second)
	}
}

func TestDiffViewArrowKeysStopAtFirstAndLastFile(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter", "up")
	atStart := view(model)

	press(t, model, "down", "down", "down")

	if !strings.Contains(atStart, "file 1 of 2") || !strings.Contains(view(model), "file 2 of 2") {
		t.Fatalf("at the start:\n%s\nat the end:\n%s", atStart, view(model))
	}
}

func TestDiffViewReportsAFailureToLoadTheDiffAndReturnsToTheBoard(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("Diff", errorForTests)
	model := loadedBoardWith(t, provider)

	press(t, model, "enter")

	if !strings.Contains(view(model), "Diff failed: "+errorForTests.Error()) || !strings.Contains(view(model), provider.pullRequest(3).Title) {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestEscapeReturnsFromDiffViewToTheSamePullRequest(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)
	model := loadedBoardWith(t, provider)

	press(t, model, "down", "enter", "down", "esc", "o")

	assertCommands(t, commands, []string{"open", provider.pullRequest(3).URL})
}

func TestQuitKeyLeavesDiffViewInsteadOfQuitting(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter")

	quits := plugintest.Quits(model, "q")

	if quits || !strings.Contains(view(model), "Repo") {
		t.Fatalf("q in the diff view should return to the board:\n%s", view(model))
	}
}

func TestDiffViewOpensTheBrowserAndTheEditorForItsPullRequest(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)
	model := loadedBoardWith(t, provider)

	press(t, model, "down", "enter", "o", "v")

	pr := provider.pullRequest(3)
	assertCommands(t, commands,
		[]string{"open", pr.URL},
		append([]string{
			"wt", "-C", filepath.Join(testCloneRoot, pr.Repository.Name), "switch", "pr:3",
			"--no-cd", "-x", testEditorCommand[0], "--", testEditorCommand[1], ".",
		}, provider.changedFiles...),
	)
}

// newerFixtureDiff is fixtureDiff after a push: a new file sorts ahead of the others, and the
// first file gains a removed line above the line the cursor tests stop on.
const newerFixtureDiff = `diff --git a/aaa.py b/aaa.py
new file mode 100644
--- /dev/null
+++ b/aaa.py
@@ -0,0 +1 @@
+brand new
diff --git a/first.py b/first.py
index 111..222 100644
--- a/first.py
+++ b/first.py
@@ -1,4 +1,4 @@
-dropped
 unchanged
-removed
+added
+added again
`

// removedFileDiff is a diff without the first file of fixtureDiff.
const removedFileDiff = `diff --git a/other.py b/other.py
--- a/other.py
+++ b/other.py
@@ -1 +1 @@
-before
+after
`

func TestOpeningACachedDiffShowsItAtOnceAndLoadsItAgain(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "esc")

	_, loading := model.Update(keyMessage("enter"))

	if !strings.Contains(view(model), "+added again") || !strings.Contains(view(model), "Refreshing…") {
		t.Fatalf("cached diff should show while refreshing:\n%s", view(model))
	}
	if provider.fetchesOfTheDiff() != 1 {
		t.Fatalf("diff fetched %d times before the command ran", provider.fetchesOfTheDiff())
	}

	plugintest.Settle(t, model, loading)

	if provider.fetchesOfTheDiff() != 2 || strings.Contains(view(model), "Refreshing…") {
		t.Fatalf("diff fetched %d times:\n%s", provider.fetchesOfTheDiff(), view(model))
	}
}

func TestOpeningADiffThatIsNotCachedShowsLoadingInsteadOfRefreshing(t *testing.T) {
	model := loadedBoard(t)

	model.Update(keyMessage("enter"))

	if !strings.Contains(view(model), "Loading diff…") || strings.Contains(view(model), "Refreshing…") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestANewerDiffReplacesTheCachedOneWhenItArrivesAndIsCached(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "esc")
	provider.diff = newerFixtureDiff

	press(t, model, "enter")
	replaced := view(model)
	press(t, model, "esc")
	model.Update(keyMessage("enter"))
	cached := view(model)

	if !strings.Contains(replaced, "file 2 of 2") || !strings.Contains(replaced, "aaa.py") {
		t.Fatalf("the newer diff should replace the cached one:\n%s", replaced)
	}
	if !strings.Contains(cached, "aaa.py") {
		t.Fatalf("the cache should hold the newer diff:\n%s", cached)
	}
}

func TestANewerDiffKeepsTheSelectedFileAndTheCursorLine(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, pressToTheAddedLine()...)
	if !strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("the cursor should start on the added line:\n%s", view(model))
	}

	plugintest.Send(t, model, diffLoadedMessage{pullRequest: provider.pullRequest(1), files: parseDiff(newerFixtureDiff)})

	if !strings.Contains(view(model), "file 2 of 2") {
		t.Fatalf("the selected file should be followed to its new position:\n%s", view(model))
	}
	if !strings.Contains(lineWith(t, model, "2 +added"), ui.CursorMarker) {
		t.Fatalf("the cursor should stay on the added line:\n%s", view(model))
	}
}

func TestANewerDiffThatDropsTheSelectedFileShowsTheFirstFile(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "down")

	plugintest.Send(t, model, diffLoadedMessage{pullRequest: provider.pullRequest(1), files: parseDiff(removedFileDiff)})

	if !strings.Contains(view(model), "file 1 of 1") || !strings.Contains(view(model), "other.py") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestAFailedRefreshKeepsTheCachedDiffAndShowsANotice(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "esc")
	provider.fail("Diff", errorForTests)

	press(t, model, "enter")

	shown := view(model)
	for _, expected := range []string{"+added again", "Diff failed: " + errorForTests.Error()} {
		if !strings.Contains(shown, expected) {
			t.Fatalf("view is missing %q:\n%s", expected, shown)
		}
	}
	if strings.Contains(shown, "Refreshing…") {
		t.Fatalf("view still says it is refreshing:\n%s", shown)
	}
}

func TestAFailedFirstLoadClosesTheDiffWithANotice(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("Diff", errorForTests)
	model := loadedBoardWith(t, provider)

	press(t, model, "enter")

	shown := view(model)
	if strings.Contains(shown, "file 1 of") || !strings.Contains(shown, "Diff failed: "+errorForTests.Error()) {
		t.Fatalf("view:\n%s", shown)
	}
}

// oneSidedBoard opens the diff of oneSidedFixtureDiff, whose files are created.py, deleted.py,
// and modified.py, with the first file selected.
func oneSidedBoard(t *testing.T) *plugin.Model {
	t.Helper()
	provider := newFakeProvider()
	provider.diff = oneSidedFixtureDiff
	model := loadedBoardWith(t, provider)
	press(t, model, "enter")
	return model
}

// showsSplitRows is whether a row of the view has a separator between two sides of the diff,
// besides the one between the file list and the diff.
func showsSplitRows(model *plugin.Model) bool {
	for _, line := range strings.Split(view(model), "\n") {
		if strings.Count(line, "│") >= 2 {
			return true
		}
	}
	return false
}

func TestAnAddedFileOpensUnifiedWhileTheLayoutPreferenceIsSplit(t *testing.T) {
	model := oneSidedBoard(t)

	if showsSplitRows(model) || !strings.Contains(helpLine(model), "s split") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestAModifiedFileIsShownSplitWhileTheLayoutPreferenceIsSplit(t *testing.T) {
	model := oneSidedBoard(t)

	press(t, model, "down", "down")

	if !showsSplitRows(model) || !strings.Contains(helpLine(model), "s unified") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestSplitKeyOnAOneSidedFileChangesOnlyThatFile(t *testing.T) {
	model := oneSidedBoard(t)

	press(t, model, "s")
	shownSplit := showsSplitRows(model)
	press(t, model, "down")
	otherOneSidedFile := showsSplitRows(model)
	press(t, model, "down")
	modifiedFile := showsSplitRows(model)
	press(t, model, "up", "up")
	backOnTheFirstFile := showsSplitRows(model)

	if !shownSplit || otherOneSidedFile || !modifiedFile || !backOnTheFirstFile {
		t.Fatalf("split: first %v, other one-sided %v, modified %v, first again %v", shownSplit, otherOneSidedFile, modifiedFile, backOnTheFirstFile)
	}
}

func TestSplitKeyOnATwoSidedFileChangesThePreferenceForTheOtherTwoSidedFiles(t *testing.T) {
	provider := newFakeProvider()
	provider.comments = nil
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "s")
	firstFileSplit := showsSplitRows(model)
	press(t, model, "down")

	if firstFileSplit || showsSplitRows(model) {
		t.Fatalf("both modified files should be unified:\n%s", view(model))
	}
}

func TestADiffThatFinishesLoadingAfterItsScreenWasClosedStillFillsTheCache(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	_, loading := model.Update(keyMessage("enter"))
	model.Update(keyMessage("esc"))

	plugintest.Settle(t, model, loading)
	model.Update(keyMessage("enter"))

	if provider.fetchesOfTheDiff() != 1 || !strings.Contains(view(model), "+added again") {
		t.Fatalf("diff fetched %d times:\n%s", provider.fetchesOfTheDiff(), view(model))
	}
}

func TestDescriptionKeyShowsTheDescription(t *testing.T) {
	model := loadedBoard(t)

	press(t, model, "p")

	if shown := view(model); !strings.Contains(shown, "Original description") || !strings.Contains(shown, "Second paragraph") {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestDescriptionScreenShowsAPlaceholderOnlyForAnEmptyDescription(t *testing.T) {
	empty := newFakeProvider()
	empty.description = ""
	withText := loadedBoard(t)
	withoutText := loadedBoardWith(t, empty)

	press(t, withText, "p")
	press(t, withoutText, "p")

	if strings.Contains(view(withText), "No description.") || !strings.Contains(view(withoutText), "No description.") {
		t.Fatalf("with text:\n%s\nwithout text:\n%s", view(withText), view(withoutText))
	}
}

func TestEscapeReturnsFromDescriptionToWhereItWasOpened(t *testing.T) {
	fromBoard := loadedBoard(t)
	fromDiff := loadedBoard(t)

	press(t, fromBoard, "p", "esc")
	press(t, fromDiff, "enter", "p", "esc")

	if !strings.Contains(view(fromBoard), "Repo") || !strings.Contains(view(fromDiff), "file 1 of 2") {
		t.Fatalf("from the board:\n%s\nfrom the diff:\n%s", view(fromBoard), view(fromDiff))
	}
}

func TestDescriptionFailureToLoadIsShownInTheDescription(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("Description", errorForTests)

	model := loadedBoardWith(t, provider)
	press(t, model, "p")

	if !strings.Contains(view(model), "Failed: "+errorForTests.Error()) {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestEditingTheDescriptionUpdatesThePullRequest(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Edited description\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	assertCalls(t, provider, saveDescriptionCall(provider.pullRequest(1), "Edited description"))
	if shown := view(model); !strings.Contains(shown, "Edited description") || !strings.Contains(shown, "Description updated.") {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestEditingTheDescriptionKeepsTheEditWhenTheProviderRefusesIt(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SaveDescription", errorForTests)
	editorWrites(t, "Edited description\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	if shown := view(model); !strings.Contains(shown, errorForTests.Error()) || !strings.Contains(shown, "your edit is kept at") {
		t.Fatalf("view:\n%s", shown)
	}
}

// keptEdit is the path of the one temporary file an edit left in the directory.
func keptEdit(t *testing.T, temporaryDirectory string) string {
	t.Helper()
	kept, err := filepath.Glob(filepath.Join(temporaryDirectory, "prs-description-*.md"))
	if err != nil || len(kept) != 1 {
		t.Fatalf("kept files = %v, error = %v", kept, err)
	}
	return kept[0]
}

func TestFailedSaveOfTheDescriptionLeavesANoticeWithTheKeptPathOnTheBoardAfterTheScreenCloses(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SaveDescription", errorForTests)
	temporaryDirectory := editorWrites(t, "Edited description\n")
	model := loadedBoardWith(t, provider)
	plugintest.Resize(t, model, 400, 30)

	press(t, model, "p", "e", "esc")

	expected := "your edit is kept at " + keptEdit(t, temporaryDirectory)
	if shown := view(model); !strings.Contains(shown, errorForTests.Error()) || !strings.Contains(shown, expected) {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestFailedSaveOfTheDescriptionAfterTheScreenWasClosedLeavesANoticeWithTheKeptPath(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SaveDescription", errorForTests)
	temporaryDirectory := editorWrites(t, "Edited description\n")
	model := loadedBoardWith(t, provider)
	plugintest.Resize(t, model, 400, 30)
	press(t, model, "p")
	_, editing := model.Update(keyMessage("e"))
	_, saving := model.Update(editing())

	model.Update(keyMessage("esc"))
	plugintest.Settle(t, model, saving)

	expected := "your edit is kept at " + keptEdit(t, temporaryDirectory)
	if shown := view(model); !strings.Contains(shown, errorForTests.Error()) || !strings.Contains(shown, expected) {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestSuccessfulSaveOfTheDescriptionLeavesNoNoticeAndNoKeptFile(t *testing.T) {
	provider := newFakeProvider()
	temporaryDirectory := editorWrites(t, "Edited description\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e", "esc")

	kept, _ := filepath.Glob(filepath.Join(temporaryDirectory, "prs-description-*.md"))
	if shown := view(model); strings.Contains(shown, "your edit is kept at") || len(kept) != 0 {
		t.Fatalf("kept files = %v, view:\n%s", kept, shown)
	}
}

func TestClosingTheEditorWithoutChangesUpdatesNothing(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	assertCalls(t, provider)
	if !strings.Contains(view(model), "Description unchanged.") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestSavingAnEmptyDescriptionUpdatesNothing(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	assertCalls(t, provider)
	if !strings.Contains(view(model), "Description unchanged.") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestDiffViewStartsSplitAndTogglesToUnified(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter")

	split := view(model)
	press(t, model, "s")
	unified := view(model)

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
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "Looks good", "ctrl+s")...)

	target := CommentTarget{Path: "first.py", Line: 2, Side: CommentSideNew}
	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), target, "Looks good"))
	if !strings.Contains(view(model), "Comment posted on first.py line 2 (new).") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestTabMovesTheCommentToTheRemovedLine(t *testing.T) {
	provider := newFakeProvider()

	press(t, loadedBoardWith(t, provider), append(pressToTheAddedLine(), "tab", "c", "Why remove this?", "ctrl+s")...)

	target := CommentTarget{Path: "first.py", Line: 2, Side: CommentSideOld}
	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), target, "Why remove this?"))
}

func TestCommentBoxLeavesTheCodeOnScreen(t *testing.T) {
	model := loadedBoard(t)

	press(t, model, append(pressToTheAddedLine(), "c", "Draft text")...)

	for _, expected := range []string{"-removed", "+added", "Comment on first.py line 2 (new)", "Draft text", "ctrl+s post"} {
		if !strings.Contains(view(model), expected) {
			t.Fatalf("view is missing %q:\n%s", expected, view(model))
		}
	}
}

func TestCommentKeyOnAHeaderLinePostsNothingAndSaysWhy(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, "enter", "c")

	assertCalls(t, provider)
	if !strings.Contains(view(model), "Move to a code line to comment on it.") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestCommentBoxShowsTheFailureWhenThePostFails(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("PostComment", errorForTests)
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "Looks good", "ctrl+s")...)

	if shown := view(model); !strings.Contains(shown, "Failed: "+errorForTests.Error()) || !strings.Contains(shown, "Looks good") {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestEscapeDiscardsTheCommentWithoutPosting(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "Never mind", "esc")...)

	assertCalls(t, provider)
	if shown := view(model); strings.Contains(shown, "Never mind") || !strings.Contains(shown, "file 1 of 2") {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestTypingBoardKeysInTheCommentBoxOnlyTypes(t *testing.T) {
	provider := newFakeProvider()
	commands := recordShell(t)
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "c", "q", "o", "v", "d")...)

	if !strings.Contains(view(model), "qovd") {
		t.Fatalf("view:\n%s", view(model))
	}
	assertCalls(t, provider)
	assertCommands(t, commands)
}

func TestCtrlCQuitsFromTheCommentBox(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, append(pressToTheAddedLine(), "c")...)

	if !plugintest.Quits(model, "ctrl+c") {
		t.Fatalf("ctrl+c should quit")
	}
}

func TestDiffViewShowsACommentThreadUnderItsLine(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter")
	viewLines := strings.Split(view(model), "\n")

	added := -1
	for index, line := range viewLines {
		if strings.Contains(line, "2 +added") {
			added = index
		}
	}
	if added < 0 {
		t.Fatalf("view has no added line:\n%s", view(model))
	}
	for offset, expected := range []string{"┌ reviewer", "│ Why this change?", "├ person", "│ Because it was wrong.", "3 +added again"} {
		if !strings.Contains(viewLines[added+1+offset], expected) {
			t.Fatalf("row %d after the added line = %q, expected %q", offset+1, viewLines[added+1+offset], expected)
		}
	}
}

func TestDiffViewLeavesOutOutdatedComments(t *testing.T) {
	model := loadedBoard(t)

	press(t, model, "enter")

	if strings.Contains(view(model), "Outdated remark") {
		t.Fatalf("an outdated comment was shown:\n%s", view(model))
	}
}

func TestCommentKeyOnAThreadRepliesToIt(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "j", "j", "j", "j", "j", "j", "j", "c")

	title := view(model)
	press(t, model, "Thanks", "ctrl+s")

	if !strings.Contains(title, "Reply to reviewer on first.py line 2 (new)") {
		t.Fatalf("comment box:\n%s", title)
	}
	target := CommentTarget{Path: "first.py", Line: 2, Side: CommentSideNew, ReplyTo: provider.comments[0].ID, ReplyAuthor: provider.comments[0].Author}
	assertCalls(t, provider, postCommentCall(provider.pullRequest(1), target, "Thanks"))
}

// cursorRow is the line of the view that carries the cursor marker.
func cursorRow(t *testing.T, model *plugin.Model) string {
	t.Helper()
	return lineWith(t, model, "▌")
}

func TestCursorStaysOnTheSameLineWhenTheLayoutChanges(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, pressToTheAddedLine()...)
	split := cursorRow(t, model)

	press(t, model, "s")
	unified := cursorRow(t, model)
	press(t, model, "s")
	backToSplit := cursorRow(t, model)

	for _, row := range []string{split, unified, backToSplit} {
		if !strings.Contains(row, "+added") || strings.Contains(row, "+added again") {
			t.Fatalf("the cursor should be on the line \"added\": %q", row)
		}
	}
}

func TestDiffViewFollowsTheTerminalSize(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter")

	plugintest.Resize(t, model, 70, 12)

	viewLines := strings.Split(view(model), "\n")
	if len(viewLines) != 12 {
		t.Fatalf("the diff view has %d lines, expected 12", len(viewLines))
	}
	for _, line := range viewLines {
		if width := len([]rune(line)); width > 70 {
			t.Fatalf("line is %d columns wide: %q", width, line)
		}
	}
}

// screenOpeners are the keys that open each screen from the board.
var screenOpeners = map[string][]string{
	"diff screen":        {"enter"},
	"description screen": {"p"},
}

func TestDraftKeyOnAScreenUsesTheRefreshedPullRequestTheSecondTime(t *testing.T) {
	for name, opener := range screenOpeners {
		t.Run(name, func(t *testing.T) {
			provider := newFakeProvider()
			model := loadedBoardWith(t, provider)
			selected := provider.pullRequest(1)
			press(t, model, opener...)

			press(t, model, "d", "d")

			assertCalls(t, provider, draftCall(selected, false), draftCall(selected, true))
		})
	}
}

func TestMergeKeyOnAScreenOffersTheMergeChoices(t *testing.T) {
	for name, opener := range screenOpeners {
		t.Run(name, func(t *testing.T) {
			provider := newFakeProvider()
			model := loadedBoardWith(t, provider)
			selected := provider.pullRequest(1)
			press(t, model, opener...)

			press(t, model, "m")
			asked := view(model)
			press(t, model, "2")

			if !strings.Contains(asked, "Merge "+selected.label()) || !strings.Contains(asked, enableAutoMergeOption) {
				t.Fatalf("view while asking:\n%s", asked)
			}
			assertCalls(t, provider, autoMergeCall(selected, true))
		})
	}
}

func TestApproveKeyOnAScreenAsksThenApproves(t *testing.T) {
	for name, opener := range screenOpeners {
		t.Run(name, func(t *testing.T) {
			provider := newFakeProvider()
			model := loadedBoardWith(t, provider)
			selected := provider.pullRequest(1)
			press(t, model, opener...)

			press(t, model, "a")
			asked := view(model)
			press(t, model, "y")

			if !strings.Contains(asked, selected.approveQuestion()) {
				t.Fatalf("view while asking:\n%s", asked)
			}
			assertCalls(t, provider, approveCall(selected))
		})
	}
}

func TestReviewKeyOnAScreenRequestsTheTeam(t *testing.T) {
	for name, opener := range screenOpeners {
		t.Run(name, func(t *testing.T) {
			provider := newFakeProvider()
			model := loadedBoardWith(t, provider)
			selected := provider.pullRequest(1)
			press(t, model, opener...)

			press(t, model, "t")

			assertCalls(t, provider, draftCall(selected, false), teamReviewCall(selected))
		})
	}
}

func TestReviewKeyOnAScreenDoesNothingWithoutAReviewTeam(t *testing.T) {
	for name, opener := range screenOpeners {
		t.Run(name, func(t *testing.T) {
			provider := newFakeProvider()
			provider.reviewTeam = ""
			model := loadedBoardWith(t, provider)
			press(t, model, opener...)

			press(t, model, "t")

			assertCalls(t, provider)
		})
	}
}

func TestScreenHelpOffersTheBoardsChangeKeysBeforeBack(t *testing.T) {
	for name, opener := range screenOpeners {
		t.Run(name, func(t *testing.T) {
			model := loadedBoard(t)
			plugintest.Resize(t, model, 300, 30)
			press(t, model, opener...)

			help := helpLine(model)

			expected := "d draft / ready · t request " + testReviewTeam + " · m merge · a approve · esc back"
			if !strings.Contains(help, expected) {
				t.Fatalf("help = %q, expected it to contain %q", help, expected)
			}
		})
	}
}

func TestDescriptionScreenKeepsEForEditing(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "p")

	press(t, model, "e")

	assertCalls(t, provider)
	if strings.Contains(view(model), "Approve") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestChangeKeysTypedIntoACommentDoNotRunActions(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, append(pressToTheAddedLine(), "c")...)

	press(t, model, "d", "t", "m", "a")

	assertCalls(t, provider)
	if !strings.Contains(view(model), "dtma") {
		t.Fatalf("the keys should be in the comment:\n%s", view(model))
	}
}
