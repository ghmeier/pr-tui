---
title: Code review, one editing pattern, and diff fixes
status: active
tags: [pull-requests, host]
next_step: Step 1 (editor-effect) dispatched to an implementer.
created: 2026-11-24
updated: 2026-11-24
---
# Code review, one editing pattern, and diff fixes

## Goal

1. Double-click to copy keeps working after the editor has been opened.
2. Every piece of prose (description, comment, reply, review body) is written in `$VISUAL`/`$EDITOR`,
   the way the description is today. The in-app comment box with `ctrl+s` goes away.
3. A reviewer can collect line comments into a pending review and submit it once as approve,
   request changes, or comment, with a body.
4. Long diff lines wrap instead of being cut off, in both layouts.
5. A file that was added or deleted outright opens in the unified layout.
6. Opening a diff always shows the current diff, not a stale cached one.

## Findings

- **Copy bug.** `tea.ExecProcess` (used to edit the description) calls `ReleaseTerminal`, whose
  `restoreTerminalState` turns mouse reporting off (bubbletea v1.3.10 `tty.go:45`).
  `RestoreTerminal` never turns it back on. After the first edit, clicks never reach prs, while
  kitty turns the wheel into arrow keys in the alternate screen, so scrolling still seems to
  work. That matches "it stops working after some interaction". The fix is to send
  `tea.EnableMouseCellMotion` after the editor returns. Making every text entry use the editor
  would have made this bug far more frequent, so the fix belongs with the shared editing code.
- **Stale diff.** `diffCache` reuses a diff while the pull request's head commit matches, but the
  head commit comes from the board's row, which is only as new as the last board refresh. After
  a push and before the next refresh, the old diff is shown.
- **Truncation.** `diffLine.render` and `renderSide` truncate with `ansi.Truncate`; comment threads
  already wrap.

## Approach

### Host: an `Edit` effect (internal/plugin)

```go
// Edit asks for text in the user's editor, $VISUAL or $EDITOR, or vi.
type Edit struct {
	// Text is what the editor opens with.
	Text string
	// Done receives the saved text with surrounding space trimmed, on the update loop. It
	// returns what to do next, such as selection.Run(effect) or a screen's own command.
	Done func(text string) tea.Cmd
}
// Effect gains:   Edit *Edit
```

The host writes `Text` to a temporary `.md` file, runs the editor with `tea.ExecProcess`, turns
mouse reporting back on afterwards, reads the file, removes it, and calls `Done`. An editor that
fails puts "Editor failed: …" on the board. `editInEditor` moves from the pull request plugin
into the host as the replaceable runner for tests. Screens reach it through
`selection.Run(plugin.Effect{Edit: …})`. The description screen is the first user.

A failed save no longer keeps the text in a temporary file. The plugin keeps it in memory
instead, and the next edit of the same thing opens with it (description: next `e`; comment:
next `c` on the same line; review body: next submission).

### Review (plugins/pullrequests and its GitHub provider)

- `c` on a code line opens the editor. An empty save cancels. A new comment is added to the
  pull request's **pending review**, held by the board in a mutex-guarded store keyed by pull
  request URL (like `diffCache`), and shown in the diff as a thread headed "pending". `c` on a
  pending comment edits it; saving it empty removes it.
- `c` on an existing thread replies, and the reply posts immediately, as now. GitHub's review
  submission cannot carry replies.
- `a` becomes **review** on the board, the diff screen, and the description screen: a choice of
  approve, request changes, or comment (only comment on the viewer's own pull request), then the
  editor for the body, then a confirmation naming the event and the number of pending comments.
  Request changes, and comment with no pending comments, need a body; an empty one shows a
  notice and submits nothing. On success the pending review is cleared and the row reloads.
- The diff header shows the pending comment count when there is one.
- Provider: `Approve` is replaced by
  `SubmitReview(pullRequest PullRequest, review Review) error`, with
  `Review{Event ReviewEvent; Body string; Comments []DraftComment}`,
  `ReviewEvent` one of `ReviewApprove`, `ReviewRequestChanges`, `ReviewComment`, and
  `DraftComment{Path string; Line int; Side CommentSide; Body string}`. GitHub sends one
  `POST /repos/{owner}/{repo}/pulls/{number}/reviews` with `commit_id`, `body`,
  `event` (APPROVE, REQUEST_CHANGES, COMMENT), and `comments` (`path`, `line`, `side`, `body`),
  through the existing `sendJSON`.

### Diff wrapping

- `ui.WrapSegments(segments []ui.Segment, width int) [][]ui.Segment` splits styled text into
  lines of at most `width` cells, keeping each piece's style.
- A diff line renders as one or more lines. In unified, continuation lines keep the line's
  background and leave the marker column blank. In split, each side wraps to its own width, the
  row is as tall as the taller side, and the line number shows only on the first line.
- `viewRow` records which wrapped part it is. `j`/`k` move between diff lines and comment lines,
  skipping continuation parts, and the cursor marker sits on a line's first part.
- Wrapped rows are rebuilt only when the file, layout, width, or threads change, not on every
  key, because `keepPositionVisible` runs after every update and diffs can have thousands of lines.

### Fresh diffs and one-sided files

- Opening a diff shows the cached copy at once, if any, and always loads the diff again. The new
  one replaces it, keeping the selected file by path and the cursor on the same line where it
  still exists. The cache is keyed by URL only.
- `parseDiff` marks a file whose metadata has `new file mode` or `deleted file mode` as one-sided.
  Moving to a one-sided file shows it unified; other files use the layout last chosen with `s` on
  a two-sided file. `s` still toggles the current file.

No configuration changes.

## Steps

Steps 2 to 4 all touch `diffscreen.go`, so they run in sequence.

1. **`editor-effect`** from `main`: the host `Edit` effect with mouse re-enabled, the description
   screen moved onto it, and `enter` instead of `ctrl+s` submitting the workflow run form. Verify with host tests for the effect
   (text in, trimmed text to `Done`, editor failure notice, file removed) and a manual check in
   kitty that double-click copies after editing a description.
2. **`wrap-diff-lines`**: `ui.WrapSegments` and wrapped diff rendering. Verify with tests for
   wrapping in both layouts and cursor movement over wrapped lines, plus a look at a real pull
   request with long lines.
3. **`fresh-diff`**: always reload the diff, and one-sided files open unified.
4. **`pending-review`**: comments through the editor, pending review, the review action, and
   the provider's `SubmitReview`; README key tables updated.

## Risks

- Wrapping changes how rows map to the cursor, which every diff feature depends on. Step 2
  comes before the review work so pending threads are added to the new row layout once.
- A pending comment can point at a line that a later push moved. GitHub rejects the review then,
  and the pending comments stay so nothing is lost.
- Pending reviews live in memory, so quitting prs drops them. The confirmation before quitting is
  not in scope; say so to the user.

## Notes and decisions

- 2026-11-24: The user chose the editor for all prose, pending reviews submitted at once, and
  unified for added or deleted files.
- 2026-11-24: The workflows run form is a form of single-line fields, not prose, so it does not
  use the editor. At the user's request, `enter` submits it instead of `ctrl+s`; this rides along
  in step 1.
