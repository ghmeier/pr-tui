# prs

A terminal board for the work you are waiting on, built from plugins. The first board lists
open pull requests across the repositories you configure. It shows each one's review state,
CI status, lines changed, and requested reviewers, and lets you read diffs, comment, and act
on them without leaving the terminal.

## Install

With [Homebrew](https://brew.sh), on macOS or Linux:

```
brew install --cask ghmeier/tap/prs
prs --init           # writes a starter configuration file to edit
```

This also installs [`gh`](https://cli.github.com), which the GitHub provider uses. Sign in
once with `gh auth login`. The `v` key also needs [worktrunk](https://worktrunk.dev) (`wt`);
everything else works without it.

Without Homebrew, download the archive for your machine from the
[releases page](https://github.com/ghmeier/pr-tui/releases), put `prs` on your `PATH`, and
install `gh` yourself.

## Configuration

`prs` reads a TOML file. Run `prs --init` to write a commented starter file and print its
path; it will not overwrite an existing file. The file is found at, in order: the
`--config <path>` flag, `$PRS_CONFIG`, `$XDG_CONFIG_HOME/prs/config.toml`, and
`~/.config/prs/config.toml`.

```toml
plugins = ["pull_requests"]        # the plugins to enable, in order; default ["pull_requests"]
refresh_interval = "3m"            # a Go duration, at least 1s; default 3m

[editor]
name = "VS Code"                   # shown in the help line; default "VS Code"
command = "code"                   # default "code"; a string is one executable, spaces included
# command = ["code", "--reuse-window"]   # or a list: the executable, then its arguments

[pull_requests]
provider = "github"                # only "github" is supported; default "github"
clone_root = "~/git"               # default "~/git"; a clone lives at <clone_root>/<name>

[github]                           # the settings of the provider named above
repositories = ["acme/storefront", "acme/billing"]   # required; owners may differ
review_team = "engineering"        # optional

[agent_plans]                      # read only when "agent_plans" is in plugins
repositories = ["~/git/storefront", "~/git/billing"]   # required; checkouts that contain a .apl directory

[workflows]                        # read only when "workflows" is in plugins
repositories = ["acme/storefront", "acme/billing"]   # required; owner/name
```

The repositories and the review team belong to the provider, so they live in the table named
after it. Without `review_team`, the `t` key is disabled and no team is highlighted or counted
as waiting on you.

A mistake in the file, such as an unknown key or a repository not written as `owner/name`,
is reported at startup with the file, the table, and the key.

## Use

```
prs                    # interactive board
prs --once             # print every board and exit
prs --bots             # start on bot-authored pull requests
prs --repo storefront  # start with only this repository selected, as a name or owner/name (repeatable)
prs --config <path>    # use a configuration file at this path
prs --init             # write a starter configuration file
prs --version          # print the version
```

The State column uses these marks:

| Mark | Meaning |
| --- | --- |
| 📝 | Draft |
| ✅ | Approved |
| ❌ | Changes requested |
| ✋ | Someone else's pull request with a review requested from you or the team |
| ♻️ | In the merge queue |
| ⏩ | Auto-merge enabled |
| 🟢 / 🔴 / 🟡 | CI passing / failing / running |

The board's Branch column shows each pull request's head branch.

Board keys:

| Key | Action |
| --- | --- |
| `enter` | Diff view |
| `p` | Read or edit the description |
| `o` | Open in the browser |
| `v` | Check out the branch with `wt switch` and open it, with its changed files, in the configured editor |
| `d` | Toggle draft / ready for review |
| `t` | Request review from the configured team, marking a draft ready for review first |
| `m` | Merge now (squash, after a confirmation), or enable or disable auto-merge (squash) |
| `a` | Approve someone else's pull request, after a confirmation |
| `f` | Filter by repository and by people or bots: `j`/`k` move, `space` toggles, `1` to `9` toggle the first nine options, `b` switches people and bots |
| `tab` / `shift+tab` | Switch boards, when plugins add more than one |
| `r` | Refresh (also on the configured interval, every 3 minutes by default) |
| `q` | Quit |

Double-click any cell to copy its full text, including text the table cuts short. Clicking a
row selects it, and the mouse wheel moves the cursor. Because the app captures the mouse, hold
Option (iTerm2) or Shift (most other terminals) to select text with the terminal instead.
Columns whose values vary in length, such as branches, reviewers, and tags, are cut short;
double-click a cell to copy its full text.

Diff view keys:

| Key | Action |
| --- | --- |
| `↑` / `↓` | Previous / next file |
| `j` / `k`, `space` / `b` | Move the line cursor by a line or a page |
| `s` | Toggle split and unified layouts |
| `c` | Comment on the line under the cursor, or reply to the thread under it (`ctrl+s` posts) |
| `tab` | Choose the old or new side of a split row for the comment |
| `d` | Toggle draft / ready for review |
| `t` | Request review from the configured team, as on the board |
| `m` | Merge now (squash, after a confirmation), or enable or disable auto-merge (squash) |
| `a` | Approve someone else's pull request, after a confirmation |
| `esc` | Back |

Description view keys:

| Key | Action |
| --- | --- |
| `↑` / `↓`, `space` / `b` | Scroll by a line or a page |
| `e` | Edit the description |
| `d` | Toggle draft / ready for review |
| `t` | Request review from the configured team, as on the board |
| `m` | Merge now (squash, after a confirmation), or enable or disable auto-merge (squash) |
| `a` | Approve someone else's pull request, after a confirmation |
| `esc` | Back |

## Agent plans

The `agent_plans` plugin adds a board of the `apl` documents
(plans, research notes, and handoffs) in your local checkouts, newest first, with the pull
request numbers each document mentions and, in a Tags column, its tags. It
needs `apl` on your `PATH` and is off until you add `"agent_plans"` to `plugins`. Each entry
of `repositories` must be a directory with a `.apl` directory inside, and two entries cannot
have the same directory name, because the name is how the board tells repositories apart. The filter menu (`f`) narrows by repository, type, and
status; it starts on plans only. `enter` reads the selected document as rendered markdown
(front matter left out), and `v` opens its file in your configured editor, from the board or
from the document. `s` changes the selected document's status, from the board or from the
document: it asks in the status line which of the statuses its type allows to set (press its
number, or `esc` to cancel) and runs `apl set <document> --status <status>` in the document's
checkout. That needs apl 0.9.0 or later; the rest of the board works with older versions. When
`pull_requests` is on too, its board gets a `Plan` column with the title
of the document linked to each pull request's branch with `apl link <document> --branch <name>`,
and `+N` when more documents are linked.

## Workflows

The `workflows` plugin adds boards of CI/CD workflow runs across several repositories. It is
off until you add `"workflows"` to `plugins`, and it needs `gh` to be logged in, like the
pull request board. Each `[[workflows.sections]]` entry is its own tab; with none, you get one
tab titled "Recent runs" that lists every recent run of every repository.

```toml
[workflows]
provider = "github"                # only "github" is supported; default "github"
repositories = ["acme/storefront", "acme/billing"]   # required; owner/name

[[workflows.sections]]
title = "Deploys"                  # required, and different for each section
kind = "runs"                      # "runs" (default) or "workflows"
workflows = ["Deploy CI", "deploy.yaml"]   # a workflow's name or its file name; default: all
status = ["queued", "running"]     # runs only: queued, running, succeeded, failed, cancelled
trigger = ["push"]                 # push, pull_request, schedule, manual, merge_queue
repositories = ["acme/storefront"] # replaces the list above for this section
limit = 20                         # runs only: recent runs per workflow, or per repository; 1 to 100; default 20
```

A section of kind `runs` lists runs, newest first, with the repository, workflow, title, branch,
trigger, status, how long ago it started, and how long it took. A section of kind `workflows`
lists the workflows themselves, with their cron schedules and the status of their latest run,
which fills in after the table appears. A section's filters combine: a run or workflow must
match the names, the statuses, and the triggers that the section lists.

The `limit` is the number of recent runs fetched for each workflow the section names, in each
repository. A section that names no workflows fetches that many runs for each repository as a
whole, before its other filters apply. A name that matches no workflow in a repository is
skipped for that repository.

Status marks:

| Mark | Meaning |
| --- | --- |
| 🟢 | succeeded |
| 🔴 | failed, timed out, or failed to start |
| 🟡 | running |
| ⏳ | queued or waiting |
| ⚪ | cancelled |
| ⏭️ | skipped |

| Key | Action |
| --- | --- |
| `enter` | Open the run's jobs; on a workflow, the jobs of its latest run |
| `o` | Open the run, or the workflow, in the browser |
| `f` | Filter by repository and by workflow |

On the jobs screen, `j`/`k` or the arrow keys move between jobs and show the steps of the
selected one, `o` opens the selected job in the browser, `r` reloads the jobs, and `esc` or
`q` goes back.

## Extending

A plugin is a Go package, compiled in, that registers boards and may add columns and keys to
another plugin's board. It runs only when its name is in the `plugins` list. A provider is a
data source behind a plugin: the pull request plugin reads pull requests through a
`Provider` interface, and GitHub is one implementation of it.

[docs/writing-a-plugin.md](docs/writing-a-plugin.md) builds a small plugin step by step and
covers writing a provider. Its example, `plugins/tickets`, ships disabled; add `"tickets"`
to `plugins` to see it. `CLAUDE.md` describes the package layout and the conventions.

## Development

Requires Go.

```
make install   # builds to ~/.local/bin/prs
make test
```

To release, push a version tag. The release workflow builds the binaries, publishes a
GitHub release, and updates the Homebrew tap:

```
git tag v0.2.0
git push origin v0.2.0
```

The workflow needs a repository secret on this repository named `HOMEBREW_TAP_TOKEN`: a
fine-grained personal access token with read and write access to the contents of
`ghmeier/homebrew-tap`.
