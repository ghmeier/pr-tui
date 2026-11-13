---
title: Configuration file and plugin system
status: active
tags: [plugins, configuration]
created: 2026-11-13
updated: 2026-11-13
next_step: Review, refactor, and merge step 2b (plugin-host branch), then write the plugin author guide
---
# Configuration file and plugin system

## Goal

`prs` started as a single-file terminal board of open GitHub pull requests with one person's
preferences compiled in. Make it something teammates can configure and extend:

1. No hard-coded preferences. Repositories, review team, editor, clone location, and refresh
   interval come from a configuration file.
2. A plugin system, so people can opt in to modules and build new ones. The motivating
   examples are a Shortcut plugin, replacing GitHub with GitLab, and combinations such as
   showing Shortcut tickets in the pull request table beside their pull requests.

No pull requests are opened for this work. Each step is built on a local branch, verified
locally, and fast-forwarded into `main`.

## Approach

### Plugins are compiled in

A plugin is a Go package in this repository (or a fork) that registers itself by name. The
configuration file's `plugins` list decides which ones run. External executables were
rejected because the diff and comment views are too interactive to drive across a process
boundary.

### A plugin contributes pieces, and pieces can target another plugin's board

A plugin is not the same thing as a tab. In `Setup(host)` a plugin reads its own
configuration section and registers contributions:

| Contribution | What it is | Example |
| --- | --- | --- |
| Board | A tab the host renders. The plugin supplies how to load rows, the columns, the actions, and the filter facets. The host owns the table, cursor, filter menu, refresh, status line, and tabs. | Pull requests; later, Shortcut stories |
| Column | A column on any board, added by any plugin, with an optional background loader keyed by row. | "Lines" and CI state today; later, a Shortcut "Ticket" column on the pull request board |
| Action | A key on a board's rows. It returns a described effect: a notice, an optional confirmation, a command to run, whether to refresh afterwards, or a screen to open. Any plugin can add one to any board. | Toggle draft, auto-merge, open in editor |
| Screen | A full-screen view that an action opens. | Diff view, description view |

The existing "Lines" column and the CI part of "State" already load late and merge by URL,
so they are rebuilt on the Column contribution. That proves the extension point with real
use before a second plugin exists.

Filters become generic facets (a title, options, and a function from row to value, either
multi-select or single-select). The filter menu is one cursor list over all facets. The
pull request board has a Repositories facet and an Authors facet, and `b` stays as a
shortcut to switch between people and bots.

With one board enabled the tool looks as it does today. A tab bar appears with two or more.

### Providers

The pull request plugin talks to a `Provider` interface and contains no GitHub vocabulary.
GitHub is the first implementation, in `plugins/pullrequests/github`, and it is the only
package that runs `gh`. A provider registers with the plugin that consumes it
(`pullrequests.RegisterProvider`). There is no host-level service lookup, because nothing
else needs one yet.

### Configuration

The file is TOML at `~/.config/prs/config.toml` (overridable with `--config`, `$PRS_CONFIG`,
or `$XDG_CONFIG_HOME`). `prs --init` writes a commented starter and never overwrites. The
top level holds `plugins`, `refresh_interval`, and `[editor]`. Every other table is a
section owned and strictly decoded by one plugin or provider, so a GitLab provider can have
a `[gitlab]` section with different fields.

```toml
plugins = ["pull_requests"]
refresh_interval = "3m"

[editor]
name = "VS Code"
command = "code"                 # or a list: ["code", "--reuse-window"]

[pull_requests]
provider = "github"
clone_root = "~/git"

[github]
repositories = ["CoProcure/gazebo", "CoProcure/u2"]
review_team = "engineering"      # optional; without it the "e" key is disabled and hidden
```

The viewer's username comes from the provider, not the file.

### Package layout after step 2

```
main.go                          flags, configuration, wiring
internal/config                  top-level settings, strict per-section decoding, --init
internal/ui                      styles, segments, table helpers
internal/shell                   runs external commands; tests replace it
internal/plugin (step 2b)        registry, host, Board, Column, Action, Screen
plugins/pullrequests             domain types, Provider interface, board, diff, description, comments
plugins/pullrequests/github      GitHub provider
```

## Steps

Steps run in sequence, because each one touches most of the code. Every step gets an
implementer, a refactor pass, and then verification with `go vet`, `go test`, and a
`prs --once` run against real configuration. Subagents run on Sonnet.

### 1. Configuration file — done, merged to `main` at `4593118`

- TOML configuration replaces the constants and the `repositories` global.
- Repositories are `owner/name` and may have different owners.
- `--init`, `--config`, strict validation with errors that name the file and key.
- The editor command accepts a string or a list with arguments.
- The filter menu is a cursor list (`j`/`k`, `space`) with `1`–`9` as shortcuts, so there is
  no limit on the number of repositories.
- 113 tests pass.

### 2a. Package split and Provider — done, merged to `main` at `ebde709`

- Split package `main` into the layout above (without `internal/plugin`).
- Move every `gh` call behind `Provider`. Replace GitHub's raw strings for CI state, review
  decision, and comment side with neutral types.
- Move repositories and the review team from `[pull_requests]` to `[github]`.
- Redistribute tests: the GitHub package asserts `gh` argument lists, and the pull request
  package uses a fake provider.
- No visible change other than the configuration shape.
- A file in the old shape gets an error saying `repositories` and `review_team` moved to
  the provider's table.
- 192 tests pass across the packages.

### 2b. Plugin host — in progress on `plugin-host`

- `internal/plugin`: registry, `Setup(host)`, and the Board, Column, Action, Screen, and
  facet types. Plugins may register command-line flags.
- A root model that hosts boards as tabs, owns the generic table, cursor, filter menu,
  refresh tick, status line, and confirmation prompt, and routes messages to screens.
- Rebuild the pull request plugin on it: one board, its columns (with loaders for line
  counts and CI state), its actions, and the diff, description, and comment screens.
- `--once` prints each enabled board.
- Acceptance test: a second plugin defined in a test adds a column and an action to the
  pull request board and registers a second board, without the pull request package
  knowing about it.

### 3. Plugin author guide — not started

- A "write a plugin" section in the README covering a board, a column on someone else's
  board, an action, and a provider.
- Keep the example honest by compiling it in a test.

### 4. Remove what is specific to one setup — not started

The repository goes public once it is generalized, so nothing in it should name the
original setup.

- Test fixtures and the README use the real organization and repository names
  (`CoProcure/gazebo`, `u2`, `analytics`, `infrastructure`) and the VS Code Insiders path.
  Replace them with neutral names. Wait for step 2b to merge first, because it moves the
  tests.
- `v` reports a clear message when `wt` is not installed.
- Decide what to do about history before the repository is made public: the early commits
  contain the hard-coded organization, repository names, and team, and the `apl` branch
  holds this plan, which names them too. Squashing `main` to one commit and not publishing
  the `apl` branch would remove both.

### 5. Distribution without Go — not started

- `prs --version`.
- GoReleaser in a GitHub Action, run on a version tag, publishing macOS (Apple silicon and
  Intel) and Linux binaries to GitHub Releases.
- A Homebrew tap that GoReleaser updates, with `gh` declared as a dependency, so the
  install is `brew install ghmeier/tap/prs` followed by `prs --init`.
- People who only enable existing plugins need no Go. Writing a plugin still means building
  from source.

## Risks

- **Step 2b is the uncertain one.** Moving the diff, description, and comment views onto a
  generic screen stack changes how asynchronous messages are routed. Results that arrive
  after a screen closes (for example a diff that finishes loading) must not be lost or
  delivered to the wrong screen. The diff cache needs an owner that outlives the screen.
- **The generic table must keep today's behaviour**: the flexible title column, the cursor
  staying on the same row across refreshes, and background column data surviving a refresh.
- **Test redistribution in 2a and 2b** moves about 1,300 lines of tests. Every invariant
  covered today must still be covered; test counts per package are checked at each step.
- **One review team for all repositories.** `review_team` applies to every repository,
  including ones under a different owner where the team may not exist.

## Notes and decisions

**2026-11-13**

- The auto-merge work that was uncommitted at the start was committed to `main` first
  (`fd257c4`), so branches could start from it.
- `review_team` empty disables and hides the `e` key (approved).
- Editor command must take arguments, and repository filters must not be capped at nine
  (requested during step 1; both done).
- Refresh intervals shorter than one second are rejected.
- An explicit `command = ""` under `[editor]` is an error; only an omitted key falls back to
  the default `code`.
- GitHub settings get their own `[github]` section instead of living under
  `[pull_requests]`, so each provider owns its own fields.
- The installed `~/.local/bin/prs` is not rebuilt by this work. Run `make install` from
  `main` to pick up merged steps.
