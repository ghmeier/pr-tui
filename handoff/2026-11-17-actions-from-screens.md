---
title: Pull request actions from screens, approval, and plan status
type: handoff
status: closed
tags: [plugins]
created: 2026-11-17
updated: 2026-11-17
---
# Pull request actions from screens, approval, and plan status

Closes out `plan/2026-11-17-actions-from-screens.md`, which has the design and dated decisions.
prs is on `main` at `09ecf35`, merged locally and not pushed. The apl side is agent-plan#9,
released as v0.9.0.

## Done

- **Screens run board keys.** The diff and description screens take `d` (draft or ready),
  `t` (request the review team), `m` (auto-merge), and `a` (approve). They run the board's own
  action on the board's current copy of the row, so a second `d` after a refresh sends the
  opposite change.
- **Approve.** `a` asks first, then runs `gh pr review <url> --approve`. On the viewer's own
  pull request it only says it cannot.
- **Team review moved from `e` to `t`** everywhere, so `e` stays edit on the description
  screen. The example tickets plugin's key moved from `t` to `i`.
- **Plan status.** `s` on the Agent plans board and document screen offers the doc type's
  statuses from `apl types --json` as a numbered choice, and runs
  `apl set <file> --status <status>` in the checkout.
- **apl.** `apl set <doc> --status <status>` checks the status against the type, sets
  `updated:`, and changes no other line.

## Key files

| File | What it is |
| --- | --- |
| `internal/plugin/plugin.go` | `Selection.RunAction`, `Choice`, `Effect.Choice` |
| `internal/plugin/model.go` | `screenBoards`, `actionMessage`, choice mode, `asking()`, `handleAnswerKey` |
| `internal/plugin/view.go` | `viewTopScreen` draws the board's status line beneath a screen; `Choice.line` |
| `plugins/pullrequests/actions.go` | `approve`, and `screenActions`, which the screens' keys and help lines come from |
| `plugins/agentplans/status.go` | `chooseStatus`, `setStatus` |
| `plugins/agentplans/load.go` | `statusesByType`, one `apl types --json` per checkout per load |

## Gotchas

- A screen gives up its last line to the board's status line while a question or choice is
  pending or the board has a notice, so it moves up a line then. A stale board notice, such as
  "Refresh failed", shows beneath screens until the next load.
- While a question or choice is pending over a screen, every key but ctrl+c goes to it,
  including `q`. On the board, `q` still quits.
- A `Choice` hides options past the ninth.
- prs recognises an apl older than 0.9.0 by "unknown command" in the error text of `apl set`.
- `TestChoiceStatusLineMarksOnlyTheCurrentOption` forces a colour profile, which is why
  `termenv` is a direct dependency in `go.mod`.
- agent-plan's `test/reading.test.ts` can time out at the default 5 seconds under load, on
  `main` too. Run with `--timeout 30000`.

## Next steps

- Nothing was pushed in prs. Push `main` when ready.
- A key on the pull request board that opens the linked plan in the document screen, from the
  previous handoff, would now also allow setting its status from there.
