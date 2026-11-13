---
title: Configuration file and plugin system
status: closed
tags: [plugins, configuration]
branch: main
created: 2026-11-13
updated: 2026-11-13
---
# Configuration file and plugin system

Closes out the plan at `plan/2026-11-13-configuration-and-plugins.md`, which has the full
design reasoning and the dated decisions.

## Context

`prs` was a single-file terminal board of open GitHub pull requests with one person's
preferences compiled in. It is now configured from a file, built from plugins, public, and
installable with Homebrew. Released as `v0.1.0`.

## Done

- **Configuration.** Everything a person can vary is in `~/.config/prs/config.toml`. The top
  level holds `plugins`, `refresh_interval`, and `[editor]`; every other table belongs to
  one plugin or provider, which decodes it strictly. `prs --init` writes a starter.
- **Plugin host** (`internal/plugin`). A plugin registers boards, columns, actions, screens,
  and facets. A column or action can target another plugin's board. The host owns the
  table, cursor, filter menu, refresh, status line, confirmation, screen stack, and tabs.
- **Pull request plugin** (`plugins/pullrequests`) running on the host, with GitHub behind a
  `Provider` interface (`plugins/pullrequests/github`).
- **Example plugin** (`plugins/tickets`) and the guide that builds it
  (`docs/writing-a-plugin.md`). A test fails when the guide's code blocks differ from the
  package.
- **Distribution.** A pushed `v*` tag runs GoReleaser, publishes a GitHub release, and
  updates the cask in `ghmeier/homebrew-tap`. Install is `brew install --cask ghmeier/tap/prs`.
- **Working agreements.** `CLAUDE.md`, the `project-lead`, `implementer`, and
  `code-reviewer` skills, and the `implementer`, `refactorer` (Sonnet), and `code-reviewer`
  (Opus) agents.

## Key files

| File | What it is |
| --- | --- |
| `internal/plugin/plugin.go` | The whole plugin API, with where each function runs |
| `internal/plugin/host.go` | Setup, validation of contributions and configuration tables |
| `internal/plugin/model.go`, `board.go`, `view.go` | The root model, per-board state, and rendering |
| `internal/plugin/plugintest` | Helpers for driving the host in a plugin's tests |
| `internal/config/config.go` | File loading, strict sections, the starter file |
| `plugins/pullrequests/provider.go`, `types.go` | The `Provider` interface and neutral types |
| `plugins/pullrequests/diffscreen.go`, `descriptionscreen.go` | The two screens |
| `plugins/tickets` | The smallest complete plugin; copy from here |
| `.goreleaser.yaml`, `.github/workflows/release.yml` | The release pipeline |

## Next steps

Nothing is in progress. These were deferred until a real second plugin (for example
Shortcut) needs them. Every plugin is compiled in this repository, so changing the API is a
compile error fixed in place.

- Facet options built from loaded rows, and facets that read column data.
- An effect that refreshes a board other than its own.
- `selection.Data` finds a column by header text, so two plugins using the same header
  collide silently. Consider a column identifier.
- A cell cannot tell "still loading" from "loaded, nothing found".
- A second pull request provider needs two GitHub-shaped spots moved behind the provider:
  the auto-merge question says "(squash)", and `v` runs `wt switch pr:<number>`.
- The release has only been installed on macOS on Apple silicon. Linux, Intel, and
  `brew upgrade` are untested.
- The binary is not signed or notarized. The cask clears the quarantine attribute instead.

## Gotchas

- A bare `go build` in the repository writes a `pr-tui` binary. Use `make install`.
- The `HOMEBREW_TAP_TOKEN` secret must be on `ghmeier/pr-tui`, where the workflow runs, not
  on the tap repository.
- `Board.Load`, column fetches, and `Effect.Command` run on goroutines; `Cell`,
  `Column.Load`, `Facet.Value`, and `Action.Run` run on the update loop. Put what a cell
  needs on the row.
- A screen receives messages meant for other screens and must ignore them.
- A confirmation run from a screen can only be answered after the screen closes.
- Never press keys that change a pull request (`d`, `e`, `m`, commenting, saving a
  description) while verifying against real data. `--once` is read-only. The interactive
  board can be driven for verification with `tmux send-keys` and `tmux capture-pane`.
- A machine with both the Homebrew install and a source build has two `prs` binaries;
  `/opt/homebrew/bin` usually comes first on the `PATH`.
