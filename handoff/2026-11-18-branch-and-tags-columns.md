---
title: Branch and tags columns
status: closed
tags: [pull-requests, agent-plans, host]
created: 2026-11-18
updated: 2026-11-18
---
# Branch and tags columns

Continues [plan/2026-11-18-branch-and-tags-columns.md](../plan/2026-11-18-branch-and-tags-columns.md).
Pull request: https://github.com/ghmeier/pr-tui/pull/1 (branch `branch-and-tags-columns`).

## What was built

- Pull request board: a Branch column after Title, capped at 30 characters; `y` copies the head
  branch on the board and in the diff and description screens.
- `m` opens a choice between merging now with a squash commit, which asks for confirmation, and
  turning auto-merge on or off, which applies straight away. `Provider.Merge` runs
  `gh pr merge <url> --squash`.
- Agent plans board: a Tags column after Status, capped at 30 characters.
- Host: `Column.MaxWidth` and `Effect.Copy`.

## Key files

- `internal/plugin/table.go`: `renderTable` now takes `[]Column` and reads headers, the
  flexible column, and `MaxWidth` from them.
- `internal/plugin/clipboard.go`: `writeClipboard`, a package variable tests replace.
- `internal/plugin/model.go`: `commandFinishedMessage.successNotice` carries "Copied <text>".
- `plugins/pullrequests/actions.go`: `copyBranch`, `chooseMerge`, `mergeNow`, `setAutoMerge`.

## Gotchas

- `writeClipboard` always returns nil: when the system clipboard fails it falls back to OSC 52,
  which cannot report whether the terminal accepted it. macOS Terminal.app ignores OSC 52.
- The fallback has not been tried in a real terminal.
- `Effect.Copy` is carried out on its own, not after `Command`.
- The Branch column pushes the pull request board past 120 columns once Title is at its
  minimum; two tests resize to 200 for this reason.

## Next steps

- Review and merge pull request #1. If the width bothers anyone, lower the Branch cap or move
  the column later in the board.
