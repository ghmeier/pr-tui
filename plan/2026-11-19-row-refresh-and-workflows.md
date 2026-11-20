---
title: Single-row refresh and a workflows board
status: completed
tags: [pull-requests, host, workflows]
created: 2026-11-20
updated: 2026-11-20
next_step: Done; pull requests #2 to #6 merged. A release (v0.4.0) is the user's call.
---
# Single-row refresh and a workflows board

## Goal

1. After approving, merging, toggling draft, requesting the team, or changing auto-merge, only
   the affected pull request is reloaded, so its row shows the new state in about a second
   instead of waiting for the whole board. Several actions can run at once.
2. A new `workflows` board: one tab with side-by-side sections for deploys, in-progress CI,
   scheduled workflows, the merge queue, and starting a workflow ("run command"). Each section
   can combine several repositories.

The description view already exists (`p` on the board and in the diff view), so nothing is
needed there.

## Approach

### Single-row refresh (host and pull request board)

**Host (`internal/plugin`)**

- `Board.Reload func(row Row) (Row, error)`, optional. Runs on a background goroutine and
  returns the row's current version, or nil when the row no longer belongs on the board (a
  merged or closed pull request).
- `Effect.RefreshRow Row`: after `Command` succeeds, reload this row with `Board.Reload`
  instead of reloading the board. When the board has no `Reload`, the host falls back to a
  full reload. `Refresh` and `RefreshRow` are not both set.
- Replacing a row: find it in `board.rows` by key, replace it in place (or remove it when
  Reload returns nil), `applyFilters`, then run every column's `Load` with only that row
  and deliver the values as usual, so the CI and line-count cells update too. The cursor stays
  on the same row; when the row is removed it stays at the same index.
- A row reload that started before a full load whose rows are already applied is dropped,
  because the full load is newer. Record `loadGeneration` when the row reload starts and
  compare with `rowsGeneration` on arrival.
- Concurrent commands: `boardState.running int`, counted up when a command starts and down
  when it finishes. While more than one is running, the status line adds `(N running)` after
  the notice. When the last one finishes successfully, the notice clears, as a full load does
  today. A failure keeps showing `Failed: …`.

**Pull request board**

- `Provider.PullRequest(pullRequest PullRequest) (PullRequest, bool, error)`: the pull
  request's current version and whether it is still open.
- GitHub: one GraphQL query, `repository(owner, name) { pullRequest(number) { state …fields } }`.
  Split the existing node fields into a `fragment pullRequestNodeFields on PullRequest`
  shared by both queries, so the two cannot drift.
- `board.reload(row)` keeps the old row's `Viewer`.
- `toggleDraft`, `setAutoMerge`, `mergeNow`, `approve`, `requestTeamReview` set
  `RefreshRow: selection.Row` instead of `Refresh: true`. Saving a description and posting
  a comment are unchanged.
- `pullrequeststest`'s fake provider gains `PullRequest`.

### Workflows board

A new plugin `workflows` with a neutral `Provider` interface in `plugins/workflows` and a
GitHub provider in `plugins/workflows/github`. CLAUDE.md's boundary changes from "only
`plugins/pullrequests/github` may run gh" to "only the `github` provider packages may run
gh".

**Configuration**

```toml
plugins = ["pull_requests", "workflows"]

[workflows]
provider = "github"                 # default "github"
repositories = ["CoProcure/gazebo", "CoProcure/u2", "CoProcure/analytics"]   # required
refresh_interval = "30s"            # optional; this board's own interval, default the top-level one

[[workflows.sections]]
title = "Deploys"
kind = "runs"                       # runs (default), workflows, or merge_queue
workflows = ["Deploy CI", "deploy.yaml"]   # names or file names, matched in every repository; empty means all
limit = 20                          # runs per repository; default 20

[[workflows.sections]]
title = "CI"
workflows = ["PR CI"]
status = ["queued", "running"]      # queued, running, succeeded, failed, cancelled; empty means all

[[workflows.sections]]
title = "Scheduled"
kind = "workflows"                  # one row per workflow with its latest run
trigger = ["schedule"]              # schedule, push, pull_request, manual, merge_queue

[[workflows.sections]]
title = "Merge queue"
kind = "merge_queue"

[[workflows.sections]]
title = "Run command"
kind = "workflows"
workflows = ["Run Command", "Run Management Command in Environments"]
```

Any section may also set its own `repositories`, a subset or a different list. No sections
means one "Recent runs" section of every run. Each value is validated with an error naming
`[[workflows.sections]]`, the section's title, and the key.

**Rows and keys**

- Run row: repository, workflow, title (commit message or pull request title), branch, trigger,
  status mark, started, duration (elapsed while running). Key: the run's URL.
- Workflow row: repository, workflow, schedule (from the workflow's cron, as written), its
  latest run's status and age. Key: repository and workflow file.
- Merge queue row: repository, position, pull request number and title, author, state, time
  in queue, estimated time to merge. Key: the pull request URL.

Every section has a Repository facet, so repositories combine in one list and can be narrowed.
A section with `kind = "runs"` also has a Workflow facet built from its rows.

**Actions**

- `enter` on a run: a run screen with its jobs, each job's status and duration, and the steps
  of the job under the cursor with their durations. `o` opens it in the browser.
- `enter` on a workflow row: the run screen for its latest run.
- `x` on a workflow row that accepts manual runs: a form screen built from the workflow's
  declared inputs (text, choice, and boolean inputs, with their defaults), a branch field
  defaulting to the default branch, then a confirmation naming the workflow, repository, and
  inputs before it starts. `x` on a run row offers to re-run it.
- `o` on any row opens it in the browser.

**Host: sections side by side in one tab**

- `Board.Tab string`: boards with the same non-empty `Tab` are shown side by side in one
  tab titled by it, in registration order, splitting the width evenly. Each board keeps its
  own rows, cursor, filter menu, actions, and status line. `←`/`→` and `h`/`l` move the
  focus between them; the focused one has a highlighted title and gets every board key.
  These four keys join the reserved keys.
- `Board.RefreshInterval time.Duration`, optional, for the 30-second refresh above.
- `--once` prints each section as its own titled table.

**GitHub provider**

- Runs: `gh api repos/<repository>/actions/runs?per_page=<n>` with `status` and `event`
  parameters where one value is asked for, filtered further in Go. One request per
  repository per section.
- Workflows and their triggers and inputs: `gh api repos/<repository>/actions/workflows`, then
  the workflow file's contents for its `on:` block, parsed with `gopkg.in/yaml.v3` (a new
  dependency). Cached per refresh, since workflow files rarely change.
- Jobs: `gh run view <id> --json jobs -R <repository>`, fetched when the run screen opens.
- Merge queue: GraphQL `repository { defaultBranchRef { name } mergeQueue { entries … } }`,
  one query for every repository.
- Start: `gh workflow run <file> -R <repository> --ref <branch> -f key=value…`.
  Re-run: `gh run rerun <id> -R <repository>`.

## Steps

1. **`single-row-refresh`** from `main`: host `Board.Reload`, `Effect.RefreshRow`, the
   running count; `Provider.PullRequest` and the GitHub query; the five actions switched.
   Tests: host replace, remove, fallback, stale-drop, and running count with fake rows;
   provider command line and parsing; each action sets `RefreshRow`. Verified with the test
   suite and `--once`. Ships as a patch release. Done: pull request #2, merged.
2. **`workflows-board`** from `main` after step 1: the plugin, configuration, provider for
   runs and workflows, and the run screen, with each section as its own tab for now (`Tab` is
   not set). Starter file and README. Verified with `--once` against gazebo, u2, and
   analytics.
3. **`workflow-sections-side-by-side`**: host `Board.Tab` and `Board.RefreshInterval`,
   used by the workflows plugin so its sections share one tab.
   Step 2 done: pull request #3, merged. Step 3 done: pull request #4, merged. Step 4 done: pull request #5, merged. Step 5 done: pull request #6, merged.
4. **`merge-queue-section`**: the `merge_queue` kind.
5. **`run-workflow`**: the inputs form, starting a workflow, and re-running a run. Tested
   with fakes only; never start a real workflow while verifying.

Steps 2 and 3 can be one pull request or two; the user is fine with either.

## Risks

- **Stale overwrites.** A full load started before an action finishes can arrive after the
  row reload and show the old state until the next refresh. Acceptable; the scheduled refresh
  corrects it. Step 1's generation check covers the opposite order.
- **Rate limits.** Five sections × three repositories every 30 seconds is about 30 REST calls
  a minute, well inside the 5,000 an hour limit. Jobs are fetched only on demand.
- **Workflow inputs.** Parsing `on:` handles its string, list, and map forms. Step 5 is last,
  so a surprise there blocks nothing else.
- **Side-by-side width.** Five tables in one terminal width is tight. Columns already have
  `MaxWidth` and one flexible column. If it does not fit, sections can stack in rows; decide
  when step 3 is on screen.

## Notes and decisions

- 2026-11-19: The user prefers reloading only the changed row over optimistic updates. No
  combined keys for multi-action workflows, because they would confuse the interface.
- 2026-11-19: One tab with side-by-side sections; it may land in a later pull request than
  the plugin itself.
- 2026-11-19: Plugin named `workflows`, not `actions`, because `Action` already means a
  key on a board's rows.
- 2026-11-19: Step 1 notice rule: a failure notice stays until a load or a new effect replaces it; a success clears the notice only when no other command is running. Left alone: a full load that started while a command ran can still arrive after the row reload and briefly show the old state.
- 2026-11-19: The user allowed merging these pull requests directly. Pull request #2 merged.
- 2026-11-19: Workflow files are read in one GraphQL tree query per repository and parsed with gopkg.in/yaml.v3, cached for the life of the process.
- 2026-11-19: Step 2: a runs section that names workflows fetches each workflow's runs separately, so `limit` is per workflow per repository; otherwise one busy workflow hid the deploys.
- 2026-11-19: Step 3 layout: sections form a grid, at least 60 columns each, wrapping to more rows on a narrow terminal. One status and help line for the focused section. `[workflows] refresh_interval` sets the board's own interval.
- 2026-11-19: Step 3: fixed pane placement after a separator (it used the byte length of "│", three, instead of its width, one) with a test against the drawn screen. Run rows now lead with status, then repository, workflow, title, start time, and duration, because a 66-column pane cut the status off. Checked live in tmux at 200 columns.
- 2026-11-20: Step 5: the form heading and confirmation say "Start <workflow>", because "Run Run Command" read badly. Checked live by opening the form for gazebo's Run Command and cancelling; nothing was submitted.
