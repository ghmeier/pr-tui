package pullrequests

import (
	"path/filepath"
	"slices"
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
	press(t, model, "j")
	second := view(model)

	if !strings.Contains(first, "+added again") || strings.Contains(first, "looks like a header") {
		t.Fatalf("first file view:\n%s", first)
	}
	if strings.Contains(second, "+added again") || !strings.Contains(second, "+++ looks like a header") {
		t.Fatalf("second file view:\n%s", second)
	}
}

func TestDiffViewFileKeysStopAtFirstAndLastFile(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter", "k")
	atStart := view(model)

	press(t, model, "j", "j", "j")

	if !strings.Contains(atStart, "file 1 of 2") || !strings.Contains(view(model), "file 2 of 2") {
		t.Fatalf("at the start:\n%s\nat the end:\n%s", atStart, view(model))
	}
}

func TestDiffViewLeftAndRightArrowsKeepTheSelectedFile(t *testing.T) {
	model := loadedBoard(t)
	press(t, model, "enter", "j")

	press(t, model, "left", "right", "left")

	if !strings.Contains(view(model), "file 2 of 2") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestDiffViewRightArrowChoosesTheNewSideAgainAfterTheOldSide(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Looks good", "")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "left", "right", "c", "a", "3", "y")...)

	review := Review{Event: ReviewEventComment, Comments: []DraftComment{draftAt(addedLineTarget, "Looks good")}}
	assertCalls(t, provider, reviewCall(provider.pullRequest(1), review))
}

func TestDiffViewTabDoesNotChangeTheSideOfAComment(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "Looks good", "")
	model := loadedBoardWith(t, provider)

	press(t, model, append(pressToTheAddedLine(), "tab", "c", "a", "3", "y")...)

	review := Review{Event: ReviewEventComment, Comments: []DraftComment{draftAt(addedLineTarget, "Looks good")}}
	assertCalls(t, provider, reviewCall(provider.pullRequest(1), review))
}

func TestDiffHelpNamesTheSideKeysOnlyForASplitFile(t *testing.T) {
	provider := newFakeProvider()
	provider.comments = nil
	model := loadedBoardWith(t, provider)
	plugintest.Resize(t, model, 300, 30)

	press(t, model, "enter")
	inSplit := helpLine(model)
	press(t, model, "s")
	inUnified := helpLine(model)

	if !strings.Contains(inSplit, "←/→ side") || strings.Contains(inSplit, "tab") {
		t.Fatalf("help in the split layout = %q", inSplit)
	}
	if strings.Contains(inUnified, "←/→") {
		t.Fatalf("help in the unified layout = %q", inUnified)
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

func TestOpeningACachedDiffShowsItAtOnceWhileRefreshing(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "esc")

	model.Update(keyMessage("enter"))

	if !strings.Contains(view(model), "+added again") || !strings.Contains(view(model), "Refreshing…") {
		t.Fatalf("cached diff should show while refreshing:\n%s", view(model))
	}
}

func TestOpeningACachedDiffLoadsItAgainAndStopsRefreshing(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "esc")

	_, loading := model.Update(keyMessage("enter"))
	fetchesBeforeTheCommandRan := provider.fetchesOfTheDiff()
	plugintest.Settle(t, model, loading)

	if fetchesBeforeTheCommandRan != 1 || provider.fetchesOfTheDiff() != 2 || strings.Contains(view(model), "Refreshing…") {
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

func TestANewerDiffReplacesTheCachedOneWhenItArrives(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "esc")
	provider.diff = newerFixtureDiff

	press(t, model, "enter")

	if !strings.Contains(view(model), "file 2 of 2") || !strings.Contains(view(model), "aaa.py") {
		t.Fatalf("the newer diff should replace the cached one:\n%s", view(model))
	}
}

func TestANewerDiffIsCachedForTheNextOpening(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	press(t, model, "enter", "esc")
	provider.diff = newerFixtureDiff
	press(t, model, "enter", "esc")

	model.Update(keyMessage("enter"))

	if !strings.Contains(view(model), "aaa.py") {
		t.Fatalf("the cache should hold the newer diff:\n%s", view(model))
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

	press(t, model, "j", "j")

	if !showsSplitRows(model) || !strings.Contains(helpLine(model), "s unified") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestSplitKeyOnAOneSidedFileChangesOnlyThatFile(t *testing.T) {
	model := oneSidedBoard(t)

	press(t, model, "s")
	shownSplit := showsSplitRows(model)
	press(t, model, "j")
	otherOneSidedFile := showsSplitRows(model)
	press(t, model, "j")
	modifiedFile := showsSplitRows(model)
	press(t, model, "k", "k")
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

// firstPullRequestText is what the editor opens with for the first pull request of the fixture.
func firstPullRequestText(provider *fakeProvider) string {
	return provider.pullRequest(1).Title + "\n\n" + provider.description
}

func TestEditingTheDescriptionOpensTheTitleABlankLineAndTheBody(t *testing.T) {
	provider := newFakeProvider()
	session := editorWrites(t, "")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	if len(session.opened) != 1 || session.opened[0] != firstPullRequestText(provider) {
		t.Fatalf("the editor was opened with %q, expected %q", session.opened, firstPullRequestText(provider))
	}
}

func TestEditingTheTitleSavesItWithTheUnchangedBody(t *testing.T) {
	provider := newFakeProvider()
	original := provider.pullRequest(1)
	editorWrites(t, "Edited title\n\n"+provider.description)
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	assertCalls(t, provider, saveDescriptionCall(original, Description{Title: "Edited title", Body: provider.description}))
}

func TestEditingTheBodySavesItWithTheUnchangedTitle(t *testing.T) {
	provider := newFakeProvider()
	original := provider.pullRequest(1)
	editorWrites(t, original.Title+"\n\nEdited description\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	assertCalls(t, provider, saveDescriptionCall(original, Description{Title: original.Title, Body: "Edited description"}))
	if shown := view(model); !strings.Contains(shown, "Edited description") || !strings.Contains(shown, "Description updated.") {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestOnlyTheFirstLineOfTheEditedTextIsTheTitle(t *testing.T) {
	provider := newFakeProvider()
	original := provider.pullRequest(1)
	editorWrites(t, "  Edited title  \nStill the body\n\nMore body")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	assertCalls(t, provider, saveDescriptionCall(original, Description{Title: "Edited title", Body: "Still the body\n\nMore body"}))
}

func TestSavingANewTitleShowsItInTheHeaderAndReloadsOnlyThatRow(t *testing.T) {
	provider := newFakeProvider()
	selected := provider.pullRequest(1)
	editorWrites(t, "Edited title\n\n"+provider.description)
	model := loadedBoardWith(t, provider)
	listingsBefore, _ := provider.loads()

	press(t, model, "p", "e")
	shown := view(model)
	press(t, model, "esc")

	listings, reloaded := provider.loads()
	if !strings.Contains(shown, "Edited title") || strings.Contains(shown, selected.Title) {
		t.Fatalf("description view:\n%s", shown)
	}
	if expected := []string{selected.URL}; listings != listingsBefore || !slices.Equal(reloaded, expected) {
		t.Fatalf("reloaded %v, expected %v; %d more full loads", reloaded, expected, listings-listingsBefore)
	}
	if !strings.Contains(view(model), "Edited title") {
		t.Fatalf("board view:\n%s", view(model))
	}
}

func TestDescriptionHeaderShowsTheRowsTitleWhileLoadingAndTheLoadedTitleAfter(t *testing.T) {
	provider := newFakeProvider()
	model := loadedBoardWith(t, provider)
	title := provider.pullRequest(1).Title

	_, loading := model.Update(keyMessage("p"))
	whileLoading := view(model)
	plugintest.Settle(t, model, loading)

	if !strings.Contains(whileLoading, title) || !strings.Contains(whileLoading, "Loading description…") {
		t.Fatalf("while loading:\n%s", whileLoading)
	}
	if !strings.Contains(view(model), title) || strings.Contains(view(model), "Loading description…") {
		t.Fatalf("loaded:\n%s", view(model))
	}
}

func TestFailedSaveOfTheDescriptionKeepsTheEditAndSaysHowToEditItAgain(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SaveDescription", errorForTests)
	editorWrites(t, "Edited title\n\nEdited description\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	if shown := view(model); !strings.Contains(shown, "Failed: "+errorForTests.Error()+" · press e to edit again") {
		t.Fatalf("view:\n%s", shown)
	}
}

func TestEditingAgainAfterAFailedSaveOpensTheFailedEdit(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SaveDescription", errorForTests)
	original := firstPullRequestText(provider)
	session := editorWrites(t, "Edited title\n\nEdited description\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e", "e")

	if len(session.opened) != 2 || session.opened[0] != original || session.opened[1] != "Edited title\n\nEdited description" {
		t.Fatalf("the editor was opened with %q, the text was %q", session.opened, original)
	}
}

func TestEditingAfterASuccessfulSaveOpensTheSavedTitleAndDescription(t *testing.T) {
	provider := newFakeProvider()
	session := editorWrites(t, "Edited title\n\nEdited description\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e", "e")

	if len(session.opened) != 2 || session.opened[1] != "Edited title\n\nEdited description" {
		t.Fatalf("the editor was opened with %q", session.opened)
	}
}

func TestFailedSaveOfTheDescriptionLeavesANoticeOnTheBoardAfterTheScreenCloses(t *testing.T) {
	provider := newFakeProvider()
	provider.fail("SaveDescription", errorForTests)
	editorWrites(t, "Edited title\n\nEdited description\n")
	model := loadedBoardWith(t, provider)
	plugintest.Resize(t, model, 400, 30)

	press(t, model, "p", "e", "esc")

	if shown := view(model); !strings.Contains(shown, "Failed: "+errorForTests.Error()) {
		t.Fatalf("view:\n%s", shown)
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

func TestSavingAnEmptyTitleUpdatesNothingAndSaysWhy(t *testing.T) {
	provider := newFakeProvider()
	editorWrites(t, "\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e")

	assertCalls(t, provider)
	if !strings.Contains(view(model), "A pull request needs a title.") {
		t.Fatalf("view:\n%s", view(model))
	}
}

func TestEditingAgainAfterAnEmptyTitleOpensTheTitleAndDescriptionAgain(t *testing.T) {
	provider := newFakeProvider()
	session := editorWrites(t, "\n")
	model := loadedBoardWith(t, provider)

	press(t, model, "p", "e", "e")

	if expected := firstPullRequestText(provider); len(session.opened) != 2 || session.opened[1] != expected {
		t.Fatalf("the editor was opened with %q, expected %q twice", session.opened, expected)
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
	return []string{"enter", "down", "down", "down", "down", "down"}
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

			expected := "d ready · t request " + testReviewTeam + " · m merge · a review · esc back"
			if !strings.Contains(help, expected) {
				t.Fatalf("help = %q, expected it to contain %q", help, expected)
			}
		})
	}
}

func TestScreenHelpLabelsDraftFromTheReloadedPullRequestAfterItIsToggled(t *testing.T) {
	for name, opener := range screenOpeners {
		t.Run(name, func(t *testing.T) {
			model := loadedBoard(t)
			plugintest.Resize(t, model, 300, 30)
			press(t, model, opener...)
			before := helpLine(model)

			press(t, model, "d")

			after := helpLine(model)
			if !strings.Contains(before, "d ready") || !strings.Contains(after, "d draft") || strings.Contains(after, "d ready") {
				t.Fatalf("help before = %q, after = %q", before, after)
			}
		})
	}
}

func TestReviewTeamKeyOnAScreenDoesNothingOnceTheTeamIsRequested(t *testing.T) {
	for name, opener := range screenOpeners {
		t.Run(name, func(t *testing.T) {
			provider := newFakeProvider()
			model := loadedBoardWith(t, provider)
			plugintest.Resize(t, model, 300, 30)
			press(t, model, append([]string{"down"}, opener...)...)

			press(t, model, "t")

			assertCalls(t, provider)
			if help := helpLine(model); strings.Contains(help, "request") {
				t.Fatalf("help = %q", help)
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
	if strings.Contains(view(model), "Submit review") {
		t.Fatalf("view:\n%s", view(model))
	}
}
