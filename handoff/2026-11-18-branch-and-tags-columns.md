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

- Pull request board: a Branch column after Title, capped at 30 characters.
- Every board: double-clicking a cell copies its full text; a click selects the row and the
  wheel moves the cursor. The host captures the mouse (`tea.WithMouseCellMotion` in `main.go`).
- `m` opens a choice between merging now with a squash commit, which asks for confirmation, and
  turning auto-merge on or off, which applies straight away. `Provider.Merge` runs
  `gh pr merge <url> --squash`.
- Agent plans board: a Tags column after Status, capped at 30 characters.
- Host: `Column.MaxWidth`, and mouse handling in `internal/plugin/mouse.go`.

## Key files

- `internal/plugin/table.go`: `renderTable` now takes `[]Column` and reads headers, the
  flexible column, and `MaxWidth` from them.
- `internal/plugin/clipboard.go`: `writeClipboard`, a package variable tests replace.
- `internal/plugin/mouse.go`: click, double-click, and wheel handling; `now` is replaceable.
- `internal/plugin/view.go`: `aboveTable` and `firstRowLine`, shared by drawing and clicks.
- `internal/plugin/model.go`: `commandFinishedMessage.successNotice` carries "Copied <text>".
- `plugins/pullrequests/actions.go`: `chooseMerge`, `mergeNow`, `setAutoMerge`.

## Gotchas

- `writeClipboard` always returns nil: when the system clipboard fails it falls back to OSC 52,
  which cannot report whether the terminal accepted it. macOS Terminal.app ignores OSC 52.
- The fallback has not been tried in a real terminal.
- `tableRows` computes the space for rows separately from `aboveTable`; if the facet summary
  ever wraps to two lines, click rows will be off by one.
- To test clicks by hand, run the app in tmux and send SGR sequences with
  `tmux send-keys -l $'\e[<0;X;YM'` (1-based X and Y), followed by the matching `m` release.
- The Branch column pushes the pull request board past 120 columns once Title is at its
  minimum; two tests resize to 200 for this reason.

## Next steps

- Review and merge pull request #1. If the width bothers anyone, lower the Branch cap or move
  the column later in the board.
