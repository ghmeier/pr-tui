---
title: Pull request actions from screens, approval, and plan status
type: plan
status: active
tags: [plugins]
created: 2026-11-17
updated: 2026-11-17
next_step: Step 2, plan-status from main, after apl set is merged and released
---
# Pull request actions from screens, approval, and plan status

## Goal

- From the diff and description screens, mark a pull request ready or draft (`d`), request
  the review team (`t`), and toggle auto-merge (`m`), as on the board.
- Approve a pull request someone else wrote with `a`, from the board and both screens, after
  a confirmation.
- Change an agent plan's status with `s` from the Agent plans board and its document screen,
  picking from the statuses `apl types` lists for the doc's type.

## Approach

### Why the host has to change

1. A screen holds a copy of the row it was opened with. After `d` and a refresh, the copy
   still says draft, so a second `d` would send the same change again. The action has to run
   on the row as the board has it now.
2. Confirmation questions and effect notices go to the board's status line, which is hidden
   while a screen is open. `m` and `a` ask first, so the question has to show over a screen.

### Host (`internal/plugin`)

- `Selection.RunAction(key string) tea.Cmd`: has the host run the board's action with that
  key, on the board's current row with the selection's key, falling back to the selection's
  own row when that row is gone. The effect is applied as if the key had been pressed on the
  board, including its question. An unknown key does nothing.
- The model records which board each screen was opened for. While a question or a choice is
  pending, or the top screen's board has a notice, the top screen is drawn one line shorter
  with that board's status line beneath it. While a question or choice is pending, keys go to
  it rather than to the screen. The comment on `applyEffect` that says questions wait for
  screens to close is removed.
- `Effect.Choice *Choice` (step 2):

  ```go
  // Choice asks for one of a few options before anything else happens. The status line shows
  // the question and the options numbered from 1; a number picks one and esc cancels.
  type Choice struct {
      Question string
      // Options are at most nine, one for each number key.
      Options []string
      // Current is marked among the options, or empty.
      Current string
      // Pick returns what choosing an option does. It runs on the update loop.
      Pick func(option string) Effect
  }
  ```

  The effect Pick returns is applied as any other, so it can ask a question too. Options past
  the ninth are not shown.

### Pull requests (`plugins/pullrequests`, `plugins/pullrequests/github`)

- Team review moves from `e` to `t` on the board. The description screen keeps `e` for edit.
- New board action `a`, label `approve`. On a pull request whose author is the viewer it
  returns only a notice, "You cannot approve your own pull request." Otherwise it confirms
  "Approve #123 in owner/name?" and runs `Provider.Approve(pr)`, then refreshes.
- `Provider.Approve(pullRequest PullRequest) error`. GitHub: `gh pr review <url> --approve`.
- The diff and description screens pass `d`, `t` (when a team is configured), `m`, and `a` to
  `selection.RunAction`, and list them in their help lines.

### Agent plans (`plugins/agentplans`)

- `Row.Statuses []string`: the statuses `apl types --json` lists for the doc's type in its
  checkout, read once per checkout on each load. Empty when the type is unknown.
- Picking a status runs `apl set <file> --status <status>` in the checkout with
  `shell.RunIn`. When apl answers that it has no `set` command, the failure reads "changing a
  status needs apl 0.9.0 or later".
- Board action `s`, label `status`: a Choice of `row.Statuses` with `Current` the doc's
  status. A doc whose type has no statuses gets a notice instead. Picking writes the file and
  refreshes the board.
- The document screen passes `s` to `selection.RunAction` and lists it in its help line.

No configuration changes.

## Steps

0. **`apl set`**, in `~/git/agent-plan`, branch `set-status` from `main`.
   `apl set <doc> --status <status>` sets the doc's status and stamps `updated:` (and
   `created:` when missing) with `setFrontmatterFields` and `stampTimestamps`, so no other line
   changes. It takes the store lock and resolves the doc with `resolveDocPath`, as `link`
   does. A status that the doc's type does not list is an error that names the allowed
   statuses. With no field option it is an error saying what can be set. README and the
   plugin skill say to use it instead of editing `status:` by hand. Verified with
   `bun test`, `bun run typecheck`, `bun run lint`. Lands as a pull request the user merges;
   then I push the `v0.9.0` tag.
1. **`screen-actions`**, from `main`. Done: merged locally at c116860. Host: `RunAction`, screen boards, status line and
   questions beneath screens. Pull requests: `a` approve with `Provider.Approve`, `e` to `t`,
   and the screens' `d`, `t`, `m`, `a`. README key list updated.
   Verified by host tests with fake boards and screens, plugin tests with the fake provider,
   a GitHub provider test for the exact `gh pr review` command, and a `--once` run.
2. **`plan-status`**, from step 1, after step 0 is merged. Host: `Effect.Choice`. Agent plans:
   `Row.Statuses`, the `s` action on the board and document screen running `apl set`.
   Verified by host tests for choosing, cancelling, and a choice over a screen; plugin tests
   that replace `shell.Run` and assert the exact `apl set` command; a `--once` run.

## Risks

- Drawing a screen one line shorter while a notice shows makes the screen jump by a line.
  Acceptable; if it reads badly, keep the line always reserved instead.
- A board notice that is stale (for example "Refresh failed") shows under every screen of
  that board until the next load. That is also where it belongs.
- Approving a draft may be refused by GitHub; the error shows as "Failed: ".

## Notes and decisions

- 2026-11-17: Team review is `t` everywhere, so `e` stays edit on the description screen.
  Approve is `a` with a confirmation. Status uses a numbered choice in the status line, and
  sets `updated:` to today.
- 2026-11-17: The user merges the agent-plan pull request; I push the release tag after.
- 2026-11-17: `apl` has no command that sets a status, so step 0 adds `apl set <doc>
  --status <s>`, named to take other fields later. apl commit already stamps `updated:`; set
  stamps it as well so the board's Updated column moves right away. Statuses differ by checkout (gazebo plans use active, merged, done, abandoned), so they come
  from `apl types --json` in each checkout.
- 2026-11-17: The approve action stays in the help line on the viewer's own pull requests,
  because actions are fixed per board; pressing it there only explains why it does nothing.
- 2026-11-17: Step 1 built on `screen-actions` (d7e22a8, refactor c116860), verified with vet,
  gofmt, 450 tests, and a `--once` run. The example tickets plugin used `t` on the pull
  request board, so its key moved to `i`, with the guide updated. The screens' help lines and
  keys come from the board's own actions (`screenActions`), so labels cannot drift. A stale
  board notice such as "Refresh failed" shows beneath screens until the next load, as expected.
