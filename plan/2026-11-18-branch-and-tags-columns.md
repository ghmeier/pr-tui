---
title: Branch and tags columns
status: active
next_step: Implementer is building step 1 on branch-and-tags-columns; then refactor pass and verification.
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
   `go test ./...`, and a `--once` run.

## Risks

- OSC 52 written from a background goroutine could interleave with a frame. Each is one write,
  so the sequence is not split; it is only the fallback.
- Branch names longer than 30 are cut in the table; the `y` action is how to get the full name.

## Notes and decisions

- 2026-11-18: The user asked for `m` to handle a regular merge as well as auto-merge. A choice
  was picked over guessing from mergeability, because the board does not load merge state.

- 2026-11-18: The user chose a Branch column on the pull request board and a Tags column on
  the agent plans board, and wants branch names copyable even when the column is capped.
