# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`prs` is a terminal board for monitoring work, written in Go with
[Bubble Tea](https://github.com/charmbracelet/bubbletea). Its first board lists open pull
requests across configured repositories and lets you read diffs, comment, and act on them.
Everything a person can vary lives in a configuration file, and features are plugins.

## Commands

```bash
make test                          # go test ./...
make install                       # builds to ~/.local/bin/prs
go vet ./... && gofmt -l .         # both must be clean before a commit
go test ./internal/plugin/ -run TestName   # one package or one test while iterating
go run . --config <file> --once    # print every board and exit; read-only
```

Never run a bare `go build ./...` or `go build .` in the repository: it writes a `pr-tui`
binary into the working tree. Use `make install`, or pass `-o` with a path outside the
repository.

## Architecture

```
main.go                        flags, configuration, wiring; nothing else
internal/config                the TOML file: top-level settings, strict per-table decoding, --init
internal/plugin                the host: plugin registry, boards, columns, actions, screens, facets, tabs
internal/ui                    styles, segments, and text helpers shared by everything
internal/shell                 runs external commands; tests replace shell.Run
plugins/pullrequests           the pull request board, its diff and description screens, and the Provider interface
plugins/pullrequests/github    the GitHub provider; the only package that runs gh
plugins/pullrequests/pullrequeststest  a small fake provider for tests of plugins that extend the pull request board
plugins/tickets                the example plugin from the guide; ships disabled, built from main.go
docs                           the guide to writing a plugin; its code blocks are plugins/tickets
```

A plugin is a package that registers itself from `init()` and is switched on by the
`plugins` list in the configuration file. In `Setup(host)` it reads its own configuration
table and registers contributions. A plugin is not the same thing as a tab, because a
contribution can target a board that another plugin owns:

| Contribution | What it is |
| --- | --- |
| Board | A tab. The plugin supplies how to load rows, the columns, actions, and filter facets. The host owns the table, cursor, filter menu, refresh, status line, confirmation prompt, and tabs. |
| Column | A column on any board, with an optional background loader that delivers values keyed by row. |
| Action | A key on a board's rows. It returns an `Effect` that describes what to do (notice, confirmation, command, refresh, screen) and the host carries it out. |
| Screen | A full-screen view that an action opens, such as the diff view. A screen keeps the `Selection` it was opened with and calls `selection.Run(effect)` to have the host do something for that board: set a notice, ask, run a command, refresh, or open another screen. |
| Facet | A way of narrowing a board's rows, shown in the filter menu. |

Where plugin code runs matters. `Board.Load`, each column `Fetch`, and `Effect.Command` run
on background goroutines. `Column.Load`, `Cell`, `Facet.Value`, and `Action.Run` run on the
update loop and must return quickly. State shared between the two kinds needs a lock; prefer
putting what a cell needs on the row. `Selection.Data` is a copy and is safe anywhere.

A plugin that adds a column or action to another plugin's board checks
`host.Enabled("<plugin name>")` first, so it still starts when that plugin is switched off.
Rows are matched across plugins through the owning plugin's exported row type, for example
`row.(pullrequests.Row).PullRequest.HeadBranch`.

The guide in `docs/writing-a-plugin.md` quotes `plugins/tickets` verbatim; when you change that
package, update the matching blocks in the guide by hand.

A provider is a data source behind a plugin. The pull request plugin only talks to its
`Provider` interface, and a provider registers with the plugin that consumes it.

### Boundaries to keep

- Only `plugins/pullrequests/github` may run `gh` or know GitHub's API shapes and vocabulary.
  `plugins/pullrequests` uses its own neutral types for things like CI state.
- `internal/plugin` and `internal/ui` know nothing about pull requests or any other plugin.
- A plugin may import another plugin's package only for its exported row and domain types,
  in order to add a column or an action to that plugin's board.
- Anything generic about a board (table layout, cursor, filtering, refresh) belongs in the
  host, so that every board gets it.

## Configuration

The file is `~/.config/prs/config.toml`, or the path given by `--config`, `$PRS_CONFIG`, or
`$XDG_CONFIG_HOME`. The top level holds `plugins`, `refresh_interval`, and `[editor]`.
Every other table belongs to exactly one plugin or provider, which decodes it strictly
through `host.Section`. An unknown key or table is an error that names the file once.

- Never compile in a personal preference: a repository, organization, team, username,
  editor, or path. Add a setting, give it a neutral default, and document it in the starter
  file in `internal/config` and in the README.
- Credentials never go in the file or the code. Providers use the login of the tool they
  call, such as `gh`.
- A new setting gets a validation error in plain language that names the table and key, and
  a test for the accepted and rejected forms.

## Conventions

**Names.** Spelled out and descriptive, matching the surrounding code: `pullRequest`,
`repository`, `configurationPath`. No abbreviations and no invented shorthand.

**Comments.** Plain sentences that say why, not what. Delete a comment that restates the
code; if code needs explaining, rename or restructure it. Never mention a plan, a ticket, a
brief, or a step number in code.

**Changes.** Move code rather than rewrite it when restructuring. Export only what another
package uses. Prefer extending the host or an existing helper over adding a parallel one.

**Tests.**
- Each test states one invariant and is named by subject and expected behavior, such as
  `TestFilterMenuTogglesTheRepositoryUnderTheCursor`.
- Arrange, act, and assert are separated by blank lines.
- Pair the positive case with the negative one.
- Assert against the fixture's own values, never a re-typed literal.
- Shared setup goes in a small helper, or in a test-support package (`configtest`,
  `plugintest`) when several packages need it.
- Host behavior is tested in `internal/plugin` with fake plugins and rows. Plugin tests cover
  only what is specific to the plugin, driving the real host with a fake provider. Provider
  tests replace `shell.Run` and assert the exact command lines and the parsing of canned
  responses.
- Tests never call the network or the real `gh`.

**Verifying against real data.** `go run . --config <temporary file> --once` is read-only
and safe. Do not press keys that change real pull requests (`d`, `e`, `m`, posting a
comment, saving a description) while verifying; cover those with tests.

## Working here

- Plans, research, and handoffs are written with `apl` (the `agent-plan:apl` skill) into
  `.apl/`, named `<type>/<YYYY-MM-DD>-<slug>`.
- Worktrees are managed with [worktrunk](https://worktrunk.dev): `wt switch --create <branch>`.
- Larger work follows the `project-lead` skill, which delegates to the `implementer`,
  `refactorer`, and `code-reviewer` agents.
- Commit messages and pull request descriptions are prose a reviewer skims: what changed
  and why, in plain language.
