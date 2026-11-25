package pullrequests

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/shell"
)

// actions are listed in the order the help line shows them. Requesting a review is offered only
// when a review team is configured.
func (board *pullRequestBoard) actions() []plugin.Action {
	actions := []plugin.Action{
		{Key: "enter", Label: "diff", Run: board.showDiff},
		{Key: "p", Label: "description", Run: board.showDescription},
		{Key: "o", Label: "open", Run: board.openInBrowser},
		{Key: "v", Label: board.editor.Name, Run: board.openInEditor},
		{Key: "d", LabelFor: draftLabel, Run: board.toggleDraft},
	}
	if board.reviewTeam != "" {
		actions = append(actions, plugin.Action{Key: "t", LabelFor: board.requestReviewLabel, Run: board.chooseReviewer})
	}
	return append(actions,
		plugin.Action{Key: "m", Label: "merge", Run: board.chooseMerge},
		plugin.Action{Key: "a", LabelFor: board.reviewLabel, Run: board.review},
	)
}

// draftLabel names the change the key makes: a draft is marked ready, anything else goes back to
// draft.
func draftLabel(row plugin.Row) string {
	if row.(Row).PullRequest.IsDraft {
		return "ready"
	}
	return "draft"
}

// requestReviewLabel is empty once nobody is left to ask, because asking again does nothing.
func (board *pullRequestBoard) requestReviewLabel(row plugin.Row) string {
	if len(board.reviewerOptions(row.(Row))) == 0 {
		return ""
	}
	return "request"
}

// teamOption is how the review team is listed among the people. A login cannot contain a space,
// so it never equals a person's.
func (board *pullRequestBoard) teamOption() string {
	return board.reviewTeam + " (team)"
}

// reviewerOptions are the review team, unless it is already requested, followed by its members
// in order, leaving out the author and anyone already requested.
func (board *pullRequestBoard) reviewerOptions(row Row) []string {
	pr := row.PullRequest
	var options []string
	if !pr.requestsTeam(board.reviewTeam) {
		options = append(options, board.teamOption())
	}
	for _, member := range slices.Sorted(slices.Values(row.TeamMembers)) {
		isRequested := slices.ContainsFunc(pr.RequestedUsers, func(login string) bool { return strings.EqualFold(login, member) })
		if !strings.EqualFold(member, pr.Author) && !isRequested {
			options = append(options, member)
		}
	}
	return options
}

// reviewLabel counts the comments waiting to be submitted, so that they are not forgotten.
func (board *pullRequestBoard) reviewLabel(row plugin.Row) string {
	pending := board.reviews.count(row.(Row).PullRequest.URL)
	if pending == 0 {
		return "review"
	}
	return fmt.Sprintf("review (%d pending)", pending)
}

var screenActionKeys = []string{"d", "t", "m", "a"}

// screenActions are the board's actions that the diff and description screens offer as well,
// so that a pull request can be changed without going back to the board.
func (board *pullRequestBoard) screenActions() []plugin.Action {
	var offered []plugin.Action
	for _, action := range board.actions() {
		if slices.Contains(screenActionKeys, action.Key) {
			offered = append(offered, action)
		}
	}
	return offered
}

// screenActionHelp lists the keys and labels of the screen actions that apply to the selection as
// the board has it now, for a screen's help line.
func (board *pullRequestBoard) screenActionHelp(selection plugin.Selection) []string {
	selection = selection.Latest()
	var keysAndLabels []string
	for _, action := range board.screenActions() {
		if label, _ := action.LabelOf(selection); label != "" {
			keysAndLabels = append(keysAndLabels, action.Key, label)
		}
	}
	return keysAndLabels
}

// runScreenAction hands the key to the board when it is one of the screen actions. The board
// runs it on the pull request as the board has it now, which a screen's own copy may not be, and
// does nothing when the action does not apply to it.
func (board *pullRequestBoard) runScreenAction(selection plugin.Selection, key string) (tea.Cmd, bool) {
	for _, action := range board.screenActions() {
		if action.Key == key {
			return selection.RunAction(key), true
		}
	}
	return nil, false
}

func pullRequestOf(selection plugin.Selection) PullRequest {
	return selection.Row.(Row).PullRequest
}

func (board *pullRequestBoard) showDiff(selection plugin.Selection) plugin.Effect {
	return plugin.Effect{Screen: board.newDiffScreen(selection)}
}

func (board *pullRequestBoard) showDescription(selection plugin.Selection) plugin.Effect {
	return plugin.Effect{Screen: board.newDescriptionScreen(selection)}
}

func (board *pullRequestBoard) openInBrowser(selection plugin.Selection) plugin.Effect {
	return plugin.Effect{Command: func() error {
		shell.Run("open", pullRequestOf(selection).URL)
		return nil
	}}
}

func (board *pullRequestBoard) openInEditor(selection plugin.Selection) plugin.Effect {
	pr := pullRequestOf(selection)
	return plugin.Effect{
		Notice:  "Opening " + pr.label() + " in " + board.editor.Name + "…",
		Command: func() error { return board.runEditor(pr) },
	}
}

// runEditor still opens the worktree when the changed files cannot be listed.
func (board *pullRequestBoard) runEditor(pr PullRequest) error {
	files, _ := board.provider.ChangedFiles(pr)
	_, err := shell.Run(pr.openInEditorCommand(board.editor.Command, board.settings.CloneRoot, files)...)
	if errors.Is(err, exec.ErrNotFound) {
		return errWorktrunkMissing
	}
	return err
}

var errWorktrunkMissing = errors.New("v needs worktrunk (wt), which is not installed: https://worktrunk.dev")

func (board *pullRequestBoard) toggleDraft(selection plugin.Selection) plugin.Effect {
	pr := pullRequestOf(selection)
	return plugin.Effect{
		Notice:     pr.toggleDraftNotice(),
		Command:    func() error { return board.provider.SetDraft(pr, !pr.IsDraft) },
		RefreshRow: selection.Row,
	}
}

const (
	mergeNowOption         = "merge now (squash)"
	enableAutoMergeOption  = "enable auto-merge (squash)"
	disableAutoMergeOption = "disable auto-merge"
)

// chooseMerge offers merging now or changing auto-merge, which is a different choice for a
// pull request that has auto-merge on.
func (board *pullRequestBoard) chooseMerge(selection plugin.Selection) plugin.Effect {
	pr := pullRequestOf(selection)
	autoMergeOption := enableAutoMergeOption
	if pr.AutoMerge {
		autoMergeOption = disableAutoMergeOption
	}
	return plugin.Effect{Choice: &plugin.Choice{
		Question: "Merge " + pr.label(),
		Options:  []string{mergeNowOption, autoMergeOption},
		Pick: func(option string) plugin.Effect {
			if option == mergeNowOption {
				return board.mergeNow(selection)
			}
			return board.setAutoMerge(selection, option == enableAutoMergeOption)
		},
	}}
}

func (board *pullRequestBoard) mergeNow(selection plugin.Selection) plugin.Effect {
	pr := pullRequestOf(selection)
	return plugin.Effect{
		Confirm:    "Merge " + pr.label() + " now (squash)? This cannot be undone.",
		Notice:     "Merging " + pr.label() + "…",
		Command:    func() error { return board.provider.Merge(pr) },
		RefreshRow: selection.Row,
	}
}

func (board *pullRequestBoard) setAutoMerge(selection plugin.Selection, enabled bool) plugin.Effect {
	pr := pullRequestOf(selection)
	return plugin.Effect{
		Notice:     pr.autoMergeNotice(enabled),
		Command:    func() error { return board.provider.SetAutoMerge(pr, enabled) },
		RefreshRow: selection.Row,
	}
}

// chooseReviewer lists the review team and the people who can still be asked.
func (board *pullRequestBoard) chooseReviewer(selection plugin.Selection) plugin.Effect {
	pr := pullRequestOf(selection)
	return plugin.Effect{Choice: &plugin.Choice{
		Question: "Request review on " + pr.label() + " from",
		Options:  board.reviewerOptions(selection.Row.(Row)),
		Menu:     true,
		Pick: func(option string) plugin.Effect {
			if option == board.teamOption() {
				return board.requestReview(selection, Reviewer{Login: board.reviewTeam, Team: true})
			}
			return board.requestReview(selection, Reviewer{Login: option})
		},
	}}
}

// requestReview marks a draft ready first, because a review request on a draft does not ask
// anyone to look yet. If marking it ready fails, no review is requested.
func (board *pullRequestBoard) requestReview(selection plugin.Selection, reviewer Reviewer) plugin.Effect {
	pr := pullRequestOf(selection)
	return plugin.Effect{
		Notice: pr.requestReviewNotice(reviewer.Login),
		Command: func() error {
			if pr.IsDraft {
				if err := board.provider.SetDraft(pr, false); err != nil {
					return err
				}
			}
			return board.provider.RequestReview(pr, reviewer)
		},
		RefreshRow: selection.Row,
	}
}

// openInEditorCommand has worktrunk find or create the pull request's worktree in the local
// clone at <cloneRoot>/<repository>, then open that worktree and the given files in the editor.
// Worktrunk passes everything after "--" straight to the program given to -x, so the editor's
// own arguments go there too, ahead of the worktree path.
func (pr PullRequest) openInEditorCommand(editorCommand []string, cloneRoot string, files []string) []string {
	command := []string{
		"wt", "-C", filepath.Join(cloneRoot, pr.Repository.Name), "switch", fmt.Sprintf("pr:%d", pr.Number),
		"--no-cd", "-x", editorCommand[0], "--",
	}
	command = append(command, editorCommand[1:]...)
	command = append(command, ".")
	return append(command, files...)
}

func (pr PullRequest) toggleDraftNotice() string {
	if pr.IsDraft {
		return "Marking " + pr.label() + " ready for review…"
	}
	return "Converting " + pr.label() + " back to draft…"
}

func (pr PullRequest) autoMergeNotice(enabled bool) string {
	if enabled {
		return "Enabling auto-merge on " + pr.label() + "…"
	}
	return "Disabling auto-merge on " + pr.label() + "…"
}

func (pr PullRequest) requestReviewNotice(reviewer string) string {
	if pr.IsDraft {
		return "Marking " + pr.label() + " ready and requesting review from " + reviewer + "…"
	}
	return "Requesting review from " + reviewer + " on " + pr.label() + "…"
}

// commandToOpenInBrowser is for the screens, which cannot return an Effect.
func commandToOpenInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		shell.Run("open", url)
		return nil
	}
}

// commandToOpenInEditor is for the screens. A failure shows on the selection's board as
// "Failed: ", which is there to read once the screens are closed.
func (board *pullRequestBoard) commandToOpenInEditor(selection plugin.Selection) tea.Cmd {
	pr := pullRequestOf(selection)
	return selection.Run(plugin.Effect{Command: func() error { return board.runEditor(pr) }})
}
