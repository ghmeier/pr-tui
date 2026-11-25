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
# text_command = ["kitty", "@", "launch", "--type=window", "--location=vsplit", "--cwd=current", "--wait-for-child-to-exit", "nvim"]

[pull_requests]
provider = "github"                # only "github" is supported; default "github"
clone_root = "~/git"               # default "~/git"; a clone lives at <clone_root>/<name>

[github]                           # the settings of the provider named above
repositories = ["acme/storefront", "acme/billing"]   # required; owners may differ
review_team = "engineering"        # optional

[agent_plans]                      # read only when "agent_plans" is in plugins
repositories = ["~/git/storefront", "~/git/billing"]   # required; checkouts that contain a .apl directory

[agent_sessions]                   # read only when "agent_sessions" is in plugins; every key is optional
provider = "claude"                # only "claude" is supported; default "claude"
relay_model = "haiku"              # the model that relays a message to a session; default "haiku"

[workflows]                        # read only when "workflows" is in plugins
repositories = ["acme/storefront", "acme/billing"]   # required; owner/name
```

The repositories and the review team belong to the provider, so they live in the table named
after it. Without `review_team`, the `t` key is disabled and no team is highlighted or counted
as waiting on you.

By default, writing a comment, a review, or a description hands the whole terminal to `$VISUAL` or
`$EDITOR`. Set `text_command` to run a command beside prs instead, so the diff stays visible
and usable while you write. The file's path is added as the last argument, and prs reads the file when the
command exits. With kitty, the example above opens a split next to prs. Turn on remote control over a socket
in `kitty.conf` and restart kitty:

```
allow_remote_control socket-only
listen_on unix:/tmp/kitty
```

Use a socket rather than `allow_remote_control yes` alone: without one, `kitty @` talks to kitty
through the terminal that prs is reading keys from. `--location=vsplit` splits only in kitty's
Splits layout (`enabled_layouts splits`). Other terminals can use a wrapper script that opens a
pane and waits for the editor to exit.

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
| `d` | Mark a draft ready for review, or convert a ready pull request back to draft; the help line shows which |
| `t` | Request review: opens a list of the review team and its members, which you narrow by typing and pick with enter. The team is left out once it is requested, and so are the author and anyone already requested. A draft is marked ready for review first. Hidden when nobody is left to ask |
| `m` | Merge now (squash, after a confirmation), or enable or disable auto-merge (squash) |
| `a` | Review (labelled comment on your own pull request): approve, request changes, or comment, written in your editor. It submits the pending comments you have collected in the diff view, after a confirmation. On your own pull request only a comment review is offered |
| `f` | Filter by repository and by people or bots: `j`/`k` move, `space` toggles, `1` to `9` toggle the first nine options, `b` switches people and bots |
| `tab` / `shift+tab` | Switch tabs, when plugins add more than one |
| `←` / `→` or `h` / `l` | Move between the boards that share a tab, such as the workflow sections |
| `r` | Refresh (also on the configured interval, every 3 minutes by default) |
| `q` | Quit |

Double-click any cell to copy its full text, including text the table cuts short. Clicking a
row selects it, and the mouse wheel moves the cursor. Because the app uses the mouse,
the terminal's own text selection needs its bypass modifier (shift in kitty and most
terminals, option in macOS Terminal and iTerm2), and it spans the whole screen. In the diff
view, dragging selects lines instead.
Columns whose values vary in length, such as branches, reviewers, and tags, are cut short;
double-click a cell to copy its full text.

Diff view keys:

| Key | Action |
| --- | --- |
| `j` / `k` | Previous / next file |
| `←` / `→` | Choose the old or new side of a split row for the comment; shown only while the file is split |
| `↑` / `↓`, mouse wheel | Move the line cursor by a line |
| `pgup` / `pgdn` | Move the line cursor by a page |
| `s` | Toggle split and unified layouts; a file that was added or deleted opens unified, and the key changes only that file |
| `c` | Write a comment in your editor on the line under the cursor and add it to the pending review. On a posted thread it replies at once, and on a pending comment it edits it (saving nothing removes it) |
| `d` | Mark a draft ready for review, or convert a ready pull request back to draft; the help line shows which |
| `t` | Request review from the team or one of its members, as on the board; hidden when nobody is left to ask |
| `m` | Merge now (squash, after a confirmation), or enable or disable auto-merge (squash) |
| `a` | Review (labelled comment on your own pull request): approve, request changes, or comment, written in your editor. It submits the pending comments you have collected in the diff view, after a confirmation. On your own pull request only a comment review is offered |
| `esc` | Back |

In the diff view, dragging the mouse over lines copies their code, without the markers and
line numbers, from the side of a split row where the drag started. Clicking a line moves the
cursor to it, and clicking a file in the list shows that file. For anything else, the
terminal's own selection works with its bypass key (shift in kitty and most terminals), but it
spans the whole screen.

Pending comments are kept in memory, so they are lost when `prs` quits. They are posted together when
you submit a review with `a`.

Description view keys:

| Key | Action |
| --- | --- |
| `↑` / `↓`, `space` / `b` | Scroll by a line or a page |
| `e` | Edit the title and the description in your editor: the title on the first line, a blank line, then the description. Saving changes either or both; an empty title is refused |
| `d` | Mark a draft ready for review, or convert a ready pull request back to draft; the help line shows which |
| `t` | Request review from the team or one of its members, as on the board; hidden when nobody is left to ask |
| `m` | Merge now (squash, after a confirmation), or enable or disable auto-merge (squash) |
| `a` | Review (labelled comment on your own pull request): approve, request changes, or comment, written in your editor. It submits the pending comments you have collected in the diff view, after a confirmation. On your own pull request only a comment review is offered |
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

## Agent sessions

The `agent_sessions` plugin finds the live Claude Code sessions (the ones `claude agents` lists)
that are working on each pull request, and sends them a message. It is off until you add
`"agent_sessions"` to `plugins`, it needs `claude` on your `PATH` and logged in, and it adds to the
pull request board when `pull_requests` is on.

The board gets a `Session` column with the name of the most recently active matching session, its status
(`busy` or `blocked`) when it is not idle, and `+N` when more match. A session matches a pull request when it
has mentioned the pull request's address (prs reads the last 2 MiB of the session's transcript under
`~/.claude/projects`), or when it is on the pull request's head branch in a checkout of the same repository.
A session in a worktree counts as a session of the main checkout.

`c` writes a message about the selected pull request in your editor, opened on a few lines naming the pull
request, its address, and its branch, and sends what you save to the session. With several matching
sessions it asks which one first. Saving the text unchanged sends nothing. Claude Code has no command to
send a message, so prs runs a one-shot `claude -p` that can only call `SendMessage`, on the model
`relay_model`, which takes a few seconds. A session is addressed by its name, so a send fails when two
live sessions share one; rename one with `/rename`.

## Workflows

The `workflows` plugin adds boards of CI/CD workflow runs across several repositories. It is
off until you add `"workflows"` to `plugins`, and it needs `gh` to be logged in, like the
pull request board. Every section is drawn side by side in one tab titled "Workflows", so you see
Deploys, CI, and the rest at once; with no sections, you get one section titled "Recent runs"
that lists every recent run of every repository. Use `←` / `→` or `h` / `l` to move between
the sections, and every other key applies to the section with the focus. Each section keeps its
own cursor and its own filters, and `r` reloads only the focused section. Sections sit in a
row when each can be at least 60 columns wide, and wrap onto further rows in a narrower terminal.

```toml
[workflows]
provider = "github"                # only "github" is supported; default "github"
repositories = ["acme/storefront", "acme/billing"]   # required; owner/name
# How often a runs or merge queue section refreshes while something in it is in progress. Otherwise
# sections refresh at the top-level refresh_interval, and so do tabs that are not on screen.
refresh_interval = "30s"           # a Go duration, at least 1s; default: the top-level refresh_interval

[[workflows.sections]]
title = "Deploys"                  # required, and different for each section
kind = "runs"                      # "runs" (default), "workflows", or "merge_queue"
workflows = ["Deploy CI", "deploy.yaml"]   # a workflow's name or its file name; default: all
status = ["queued", "running"]     # runs only: queued, running, succeeded, failed, cancelled
trigger = ["push"]                 # runs and workflows: push, pull_request, schedule, manual, merge_queue
repositories = ["acme/storefront"] # replaces the list above for this section
limit = 20                         # runs only: recent runs per workflow, or per repository; 1 to 100; default 20

[[workflows.sections]]
title = "Merge queue"
kind = "merge_queue"               # takes only title and repositories
```

A section of kind `runs` lists runs, newest first, with the status, repository, workflow, title,
how long ago it started, how long it took, branch, and trigger, so the parts that matter most
stay visible in a narrow pane. A section of kind `workflows`
lists the workflows themselves, with their cron schedules and the status of their latest run,
which fills in after the table appears. A section of kind `merge_queue` lists the pull requests in
each repository's merge queue for its default branch, repository by repository and front of the
queue first: whether their checks pass, the repository, the position, the pull request, its title,
its state, how long it has waited, GitHub's estimate of when it will merge (empty when GitHub has
none), and the author. It rejects `workflows`, `status`, `trigger`, and `limit`, and a repository
without a merge queue lists nothing. A section's filters combine: a run or workflow must
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

The checks mark of a queued pull request uses the same marks: 🟢 when its checks passed, 🔴 when
they failed, 🟡 while they run, and ⏳ when none have started. Its state reads `queued`,
`awaiting checks`, `mergeable`, `unmergeable`, or `locked`.

| Key | Action |
| --- | --- |
| `enter` | Open the run's jobs; on a workflow, the jobs of its latest run |
| `o` | Open the run, the workflow, or the queued pull request in the browser |
| `x` | On a workflow, fill in a form and start a manual run; on a finished run, run it again |
| `f` | Filter by repository and by workflow |

`x` on a workflow opens a form with the branch, which starts as the repository's default branch,
and then each input the workflow declares for manual runs, in the order the file declares them.
A workflow that cannot be started by hand says so instead. Starting a run never happens without
a confirmation: `enter` submits the form, and the board then asks you to confirm the branch and
the values before anything is started. The new run appears in a runs section when it next
reloads.

| Key in the form | Action |
| --- | --- |
| `↑` / `↓`, `tab` / `shift+tab` | Move between fields |
| `←` / `→` (also `h` / `l` outside text fields) | Cycle the options of a choice |
| `space` | Toggle a boolean |
| `enter` | Submit; a required field that is empty, or a number field that is not a number, is marked and nothing is submitted |
| `esc` | Cancel without starting anything |

`x` on a run that has finished offers to re-run its failed jobs (only when it failed) or all of its
jobs, and asks for a confirmation before doing either. A run that is still queued or running
only shows a notice.

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
