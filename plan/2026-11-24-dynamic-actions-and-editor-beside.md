---
title: Actions that fit the pull request, arrow keys for diff sides, and an editor beside prs
status: active
tags: [pull-requests, host, configuration]
next_step: Steps 1 and 2 dispatched in parallel.
created: 2026-11-24
updated: 2026-11-24
---
# Actions that fit the pull request, arrow keys for diff sides, and an editor beside prs

## Goal
1. The help line names what each key will do to the selected pull request: `d ready` on a draft and
   `d draft` otherwise; `a comment` on your own pull request and `a review` on others'; `t request <team>`
   disappears once the team is requested. A key whose action does not apply does nothing.
2. In the diff view, `←`/`→` choose the old or new side of a split row; `tab` no longer does.
3. Comments, replies, review bodies, and descriptions can be written in an editor opened beside prs (for
   example a kitty split), so the diff stays visible and usable while writing.

## Approach

### Host: per-row action labels (internal/plugin)
```go
type Action struct {
	Key   string
	Label string
	// LabelFor, when set, replaces Label for the selected row. An empty label means the action
	// does not apply to that row: it is hidden from the help line and its key does nothing.
	LabelFor func(row Row) string
	Run      func(selection Selection) Effect
}
// LabelOf is the action's label for the row, and whether the action applies to it.
func (action Action) LabelOf(row Row) (label string, applies bool)
```
The board help line and key handling use `LabelOf` with the cursor row. `Selection` gains
`Current() (Row, bool)`, the board's current version of the selection's row (looked up by key on the update
loop), so screens label their actions from fresh state rather than the copy they were opened with.

### Pull request actions
- `d`: "ready" on a draft, "draft" otherwise.
- `t`: "request <team>", hidden when the team is already requested.
- `a`: "comment" on the viewer's own pull request, "review" otherwise.
- The diff and description screens' help lines use the same labels for the current row.

### Diff sides
`←` chooses the old side and `→` the new side of a split row; file navigation is `↑`/`↓` only. The help line
shows `←/→ side` only in the split layout.

### Editor beside prs (configuration)
```toml
[editor]
# Where comments, reviews, and descriptions are written. Leave it out to hand the terminal to $VISUAL or
# $EDITOR. A command runs beside prs instead, with the file's path added at the end; prs stays usable
# and reads the file when the command exits. For a kitty split (needs allow_remote_control in kitty.conf):
# text_command = ["kitty", "@", "launch", "--type=window", "--location=vsplit", "--cwd=current", "--wait-for-child-to-exit", "nvim"]
```
`Edit` runs `text_command` with `shell.Run` on a background goroutine when it is set (no `ExecProcess`, no
mouse restore needed), with the notice "Writing in your editor…" on the board, then calls `Done` as before.

## Steps
1. **`dynamic-actions`** from main: `Action.LabelFor`, `Action.LabelOf`, `Selection.Current`, the pull request
   labels, and arrow keys for diff sides.
2. **`editor-beside`** from main, in parallel: `[editor] text_command`, validation, starter file, README, host.

## Risks
- Two branches touch README and possibly `internal/plugin/plugin.go`; the second to land rebases.
- kitty remote control is off in the user's kitty.conf; the README must say how to turn it on.

## Notes and decisions
- 2026-11-24: Labels name the action, not the state ("d ready" marks a draft ready). Configuration key
  `text_command` chosen by the lead; cheap to rename.
