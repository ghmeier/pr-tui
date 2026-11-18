---
title: Branch and tags columns
status: completed
next_step: Done; pull request #1 is open for review.
tags: [pull-requests, agent-plans, host]
created: 2026-11-18
updated: 2026-11-18
---
# Branch and tags columns

## Goal

The pull request board shows each pull request's head branch, and `y` copies the full branch
name to the clipboard. `m` offers either merging now or turning auto-merge on or off. The
agent plans board shows each document's tags.

## Approach

No configuration changes.

**Host (`internal/plugin`)**

- `Column.MaxWidth int`: the widest the column is drawn; zero means no limit. `renderTable`
  caps the column at it (header and cells, ending in "…" through `ui.Fit`). A column whose
  values are all narrower stays at its widest value.
- `Effect.Copy string`: text the host puts on the clipboard on a background goroutine. On
  success the status line shows `Copied <text>`; on failure, `Failed: <first line>`, like a
  command. Carried through `commandFinishedMessage` with a success notice field.
- The clipboard write is a package variable in `internal/plugin/clipboard.go` that tests
  replace. It tries the system clipboard with `github.com/atotto/clipboard` (already in
  `go.sum`, promoted to a direct dependency), and when that fails, writes an OSC 52 sequence
  through `termenv` so it still works over SSH and without `xclip`.

**Pull request board**: a `Branch` column right after `Title`, `MaxWidth: 30`, showing
`PullRequest.HeadBranch`. An action `{Key: "y", Label: "copy branch"}` returning
`Effect{Copy: HeadBranch}`. It also works from the diff and description screens through the
existing screen-action path.

**Merging**: `m` (label "merge") returns a `Choice` instead of a confirmation. Options:
`merge now (squash)` and `enable auto-merge (squash)`, or `disable auto-merge` when auto-merge
is on. Merging now asks its own confirmation ("Merge <label> now (squash)?") because it cannot
be undone; the auto-merge options apply straight away, since picking one is already deliberate.
`Provider` gains `Merge(pullRequest PullRequest) error`; GitHub runs
`gh pr merge <url> --squash` (on a branch that requires a merge queue, gh queues it).

**Agent plans board**: a `Tags` column right after `Status`, `MaxWidth: 30`, tags joined by
", ".

## Steps

1. One branch, `branch-and-tags-columns`, from `main`: host `MaxWidth` and `Copy`, both
   columns, the `y` action, the merge choice and `Provider.Merge`, README key tables. Verified with `go vet`, `gofmt -l`,
   `go test ./...`, and a `--once` run. Done: pull request #1 (commits 8d96a51, ba3f325, and a README rewrap).

2. Same branch, added to pull request #1: double-clicking any table cell copies its full text.
   The host captures the mouse (`tea.WithMouseCellMotion`), a left click moves the cursor to
   the row, two left clicks on the same cell within 500 ms copy the cell's untruncated plain
   text, and the wheel moves the cursor. Only in board mode with no screen open. `y` and
   `Effect.Copy` are removed, since nothing else would use `Effect.Copy`; the host calls
   `writeClipboard` itself. Verified with tests and `--once`. Done: commits f18cbb8, 37801d2,
   89d815f on pull request #1; checked by hand in tmux.

## Risks

- OSC 52 written from a background goroutine could interleave with a frame. Each is one write,
  so the sequence is not split; it is only the fallback.
- Branch names longer than 30 are cut in the table; the `y` action is how to get the full name.

## Notes and decisions

- 2026-11-18: The user wants `MaxWidth` applied to every column that needs it, each with its own
  width (a host-wide cap was started and reverted). Commit 1ec9002: pull request board Repo 20,
  Author 20, Branch 30, Review requested 30, Plan 30; agent plans board Repository 20, Type 12,
  Status 12, Tags 30. Fixed-length columns and the flexible Title stay uncapped.

- 2026-11-18: Double-click checked in tmux against real data with SGR mouse sequences; a cut
  branch name copied in full. Left alone: `tableRows` still computes the space for rows with its
  own arithmetic, which matches `aboveTable` only while the facet summary is one line.

- 2026-11-18: The user asked for a general double-click to copy and accepted that capturing the
  mouse means the terminal's own drag selection needs a modifier key. They chose to remove `y`.

- 2026-11-18: Built and verified. Left alone: the OSC 52 fallback reports success even if the
  terminal ignores it (Terminal.app does), and was not tried in a real terminal. The Branch
  column cuts the rightmost columns in a 120-column terminal; a smaller cap or moving Branch
  later is the fix if that bothers anyone. Copy with an empty branch does nothing silently.

- 2026-11-18: The user asked for `m` to handle a regular merge as well as auto-merge. A choice
  was picked over guessing from mergeability, because the board does not load merge state.

- 2026-11-18: The user chose a Branch column on the pull request board and a Tags column on
  the agent plans board, and wants branch names copyable even when the column is capped.
