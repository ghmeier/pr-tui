---
title: Agent plans board
status: draft
tags: [plugins, agent-plans]
created: 2026-11-13
updated: 2026-11-13
next_step: Get approval, then dispatch step 1 (agent-plans-board).
---
# Agent plans board

## Goal

A second tab lists the `apl` docs (plans, research, handoffs) of configured local
checkouts, filterable by repository, type, and status, with the pull request numbers each
doc mentions. Enter opens the doc as rendered markdown. The pull request board gains a Plan
column showing the doc linked (with `apl link`) to each pull request's branch.

## Approach

### Data source

One `apl ls --json` per configured checkout, run with the checkout as working directory.
It returns, per doc: `file` (relative to `.apl/`), `type`, `meta.title`, `meta.status`
(may be absent), `meta.tags`, `meta.created`, `meta.updated`, and `links` (each with
`branch`). `apl links --json` returns `{"links":[{"branch","docs":[file...],...}]}`.
The doc body is read from `<checkout>/.apl/<file>` directly; running `apl show` per doc
would be one Node process per doc (gazebo has 368).

Pull request numbers come from the body: `#123` (not preceded by a word character, `&`,
or `/`) and `/pull/123`. Distinct, newest (largest) first. The cell shows up to three and
then `+N`.

### Plugin `plugins/agentplans`, name `agent_plans`

```toml
[agent_plans]
# Local checkouts whose apl docs to list. Each must contain a .apl directory.
repositories = ["~/git/gazebo", "~/git/prs"]
```

- No neutral default: an empty or missing list is an error naming `[agent_plans] repositories`.
- Each path is home-expanded (`config.ExpandHome`), must exist and contain `.apl`.
- The repository's name is the path's base name. Two paths with the same base name are an
  error, because the name is the filter value and the key for matching pull requests.
- Authentication: none. `apl` uses the local checkout and its own git remote.

**Board** `agent_plans` (exported `BoardName`), title "Agent plans", row noun "docs".
Row type exported as `agentplans.Row{Repository string; Doc Doc}`; key is
`<repository name>/<file>`, stable across refreshes. Rows from all checkouts are sorted by
updated date, newest first, then by file.

Columns: Repository, Type, Title (flexible), Status, Updated, Pull requests.

Actions: `enter` view (document screen), `v` open in the configured editor
(`host.Editor().Command` with the file's absolute path).

Facets: Repositories (fixed, from configuration, all selected), Type (options from rows,
starts on `plan`), Status (options from rows, all selected; a doc without a status shows as
`none`).

**Column on the pull request board** when `host.Enabled(pullrequests.BoardName)`:
header "Plan". Its `Load` reads repository name and `HeadBranch` off each
`pullrequests.Row`; the fetch runs `apl links --json` once per configured checkout whose
base name equals `PullRequest.Repository.Name`, and delivers the linked doc's title (more
than one doc: the first title plus ` +N`).

### Host and shared changes

- **Facet options from loaded rows** (`internal/plugin`). New field
  `Facet.OptionsFromRows bool`: after each load, the options are the facet's fixed
  `Options` followed by every other distinct `Value` of the loaded rows, sorted, labelled
  by value. A newly seen option starts selected when `Selected` is empty, and unselected
  when `Selected` names values. Choices the user made survive refreshes. Rejected with
  `Single` by validation. Used by Type and Status.
- **`shell.RunIn(directory string, arguments ...string)`**, a replaceable variable like
  `shell.Run`, because `apl` has no flag for the repository and works on its working
  directory.
- **`ui.RenderMarkdown`**, moved from `plugins/pullrequests/descriptionscreen.go`, used by
  both the description screen and the new document screen.

The document screen stays in the plugin (scroll, page, home/end, `v`, esc). It repeats the
description screen's scrolling, about forty lines.

Counterpoint: a host-provided document screen would remove that repetition, but the
description screen's editing flow does not fit a generic one, so there would still be only
one user.

## Steps

1. **`agent-plans-board`** from `main`. `shell.RunIn`, `Facet.OptionsFromRows` with host
   tests, the plugin with configuration, board, columns, facets, and pull request
   references, starter file and README. Verify: tests, and `go run . --config <tmp> --once`
   against `~/git/gazebo` and `~/git/prs`.
2. **`agent-plans-document-screen`** from step 1. Move `renderMarkdown` to `ui`, add the
   document screen on `enter` and the editor action on `v`. Verify with tests and a tmux
   session.
3. **`agent-plans-pull-request-column`** from step 2. The Plan column on the pull request
   board. Verify with a fake provider test and by linking a doc to a branch with an open
   pull request in `prs`, then unlinking it.
4. Guide: mention `OptionsFromRows` in `docs/writing-a-plugin.md` where facets are covered.
   Folded into step 1 if the guide has a facets section.

## Risks

- `apl ls --json` is a Node process per checkout on each refresh, plus reading every doc
  body (368 files in gazebo). Step 1 measures the load time against gazebo.
- `apl` not installed: the load error says so, through `exec.ErrNotFound`.
- `#123` references can be issue numbers or another repository's pull requests. Accepted.
- Base-name matching fails for a checkout whose directory differs from the repository name.
  A table form of the setting can be added later if that comes up.

## Notes and decisions

- 2026-11-13: The user chose its own list of checkout paths, body references plus branch
  links, all doc types with a Type filter, and an in-app markdown view on enter.
