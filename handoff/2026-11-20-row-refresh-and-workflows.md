---
title: Single-row refresh and the workflows tab
status: closed
tags: [pull-requests, host, workflows]
branch: main
created: 2026-11-20
updated: 2026-11-20
---
# Single-row refresh and the workflows tab

Continues [the plan](../plan/2026-11-19-row-refresh-and-workflows.md). All five steps merged: pull requests #2 to #6 on ghmeier/pr-tui. Not yet released; the last tag is v0.3.1.

## What was built

- **Single-row refresh (#2).** `Board.Reload` and `Effect.RefreshRow` in `internal/plugin`. Pull request actions (approve, merge, draft, team, auto-merge) reload only their row through `Provider.PullRequest`. The status line shows "(N running)" while several commands run. A failure notice stays until a load or new effect replaces it.
- **Workflows plugin (#3).** `plugins/workflows` (neutral types, `Provider`) and `plugins/workflows/github`. `[workflows]` holds the repositories, and each `[[workflows.sections]]` entry is a pane. Kinds are `runs`, `workflows`, and `merge_queue`. `enter` opens a run screen with jobs, steps, and durations.
- **Shared tabs (#4).** `Board.Tab` groups boards into one tab of panes in a grid, each at least 60 columns wide. `←`/`→` and `h`/`l` move the focus. `Board.RefreshInterval` gives a board its own timer, set for workflows by `[workflows] refresh_interval`.
- **Merge queue (#5).** `Provider.MergeQueue`, one GraphQL query per repository.
- **Start and re-run (#6).** `x` on a workflow opens a form of its manual-run inputs, and `x` on a run offers a re-run. Both always confirm first.

## Key files

- `internal/plugin/panes.go`: pane geometry, shared by drawing and the mouse.
- `internal/plugin/model.go`: row reload, running count, and the per-interval timers.
- `plugins/workflows/settings.go`: section kinds, and the table of which keys apply to which kind.
- `plugins/workflows/github/workflows.go`: workflow files read in one GraphQL tree query and parsed with yaml.v3 (`onNode`). Cached for the life of the process.
- `plugins/workflows/formscreen.go`: the run form. Each field kind is handled in `formField`.

## Gotchas

- When a section names workflows, it fetches each workflow's runs separately, so `limit` is per workflow per repository. A single repository-wide fetch let a busy workflow push the deploys out.
- Workflow files are cached until restart, so new inputs or schedules need a restart.
- Use `lipgloss.Width`, not `len`, for anything drawn. "│" is 3 bytes but 1 column; this caused a bug in pane placement.
- A full load that started while an action ran can arrive after the row reload and show the old state until the next refresh. This was accepted.
- The GitHub check-state mapping is duplicated in the two GitHub provider packages, because they map to different neutral types.
- Never start or re-run a real workflow while verifying. To check the form by hand, open it in tmux and press `esc`.

## Next steps

- Tag a release, v0.4.0, when the user wants it.
- Add workflows sections to the user's own `~/.config/prs/config.toml`.
- Possible follow-ups:
  - an elapsed-time tick for running runs between refreshes;
  - a narrower Workflow column in runs panes;
  - choices for `environment` inputs from the repository's environments.
