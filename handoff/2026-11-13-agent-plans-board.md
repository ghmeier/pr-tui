---
title: Agent plans board
status: closed
tags: [plugins, agent-plans]
created: 2026-11-13
updated: 2026-11-13
---
# Agent plans board

Closes out `plan/2026-11-13-agent-plans-board.md`, which has the design and dated decisions.
Everything is on `main` at `697768b`; nothing was pushed.

## Done

- **`agent_plans` plugin** (`plugins/agentplans`). A board of the `apl` docs in the checkouts
  listed in `[agent_plans] repositories`, with Repository, Type, Title, Status, Updated, and
  Pull requests columns. Filters: Repositories, Type (starts on plan), Status.
- **Pull request numbers** are `#123` and `/pull/123` mentions in each doc's text.
- **Enter** shows the doc as rendered markdown without its front matter; **v** opens the file
  in the configured editor, from the board or the screen.
- **Plan column** on the pull request board: the title of the doc that `apl link` connected to
  the pull request's head branch, matched by the checkout's directory name.
- **Host:** `Facet.OptionsFromRows` builds filter options from loaded rows.
  `shell.RunIn` runs a command in a directory. `ui.RenderMarkdown` is shared by both
  markdown screens.

## Key files

| File | What it is |
| --- | --- |
| `plugins/agentplans/load.go` | `apl ls --json` parsing, body reading, pull request number extraction |
| `plugins/agentplans/board.go` | Columns and facets |
| `plugins/agentplans/documentscreen.go` | The markdown screen on enter |
| `plugins/agentplans/pullrequestcolumn.go` | The Plan column on the pull request board |
| `internal/plugin/facets.go` | `refreshOptionsFromRows` |

## Gotchas

- `apl` has no repository flag; it acts on its working directory, hence `shell.RunIn`.
- A checkout's name is its directory's base name. It is the filter value and how pull
  requests are matched, so two checkouts with the same base name are rejected.
- Every refresh runs `apl ls --json` once per checkout for the board, and once more per
  checkout that has open pull requests for the Plan column. About 0.36s for gazebo's 368 docs.
- `#123` in any doc counts, including examples in prose (this plan shows `#123`).
- The document screen repeats the description screen's scrolling code.

## Next steps

- A key on the pull request board that opens the linked plan in the document screen.
- A shared scrollable markdown screen in the host, if a third screen needs one.
- A table form of `repositories` for a checkout whose directory differs from the repository
  name.
