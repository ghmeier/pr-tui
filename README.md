# pr-tui

A terminal board of open GitHub pull requests across the repositories you configure. It shows
each pull request's draft or review state, CI status, lines changed, and requested
reviewers, and lets you act on them without leaving the terminal.

## Install

Requires Go and an authenticated [`gh`](https://cli.github.com).

```
make install   # builds to ~/.local/bin/prs
make test
```

## Configuration

`prs` reads a TOML file. Run `prs --init` to write a commented starter file and print its
path; it will not overwrite an existing file. The file is found at, in order: the
`--config <path>` flag, `$PRS_CONFIG`, `$XDG_CONFIG_HOME/prs/config.toml`, and
`~/.config/prs/config.toml`.

```toml
plugins = ["pull_requests"]        # only "pull_requests" exists today
refresh_interval = "3m"            # a Go duration, at least 1s; default 3m

[editor]
name = "VS Code"                   # shown in the help line; default "VS Code"
command = "code"                   # default "code"; a string is one executable, spaces included
# command = ["code", "--reuse-window"]   # or a list: the executable, then its arguments

[pull_requests]
provider = "github"                # only "github" is supported; default "github"
repositories = ["CoProcure/gazebo", "CoProcure/u2"]   # required; owners may differ
review_team = "engineering"        # optional
clone_root = "~/git"               # default "~/git"; a clone lives at <clone_root>/<name>
```

Without `review_team`, the `e` key is disabled and no team is highlighted or counted as
waiting on you. Mistakes in the file, such as a repository not written as `owner/name` or an
unknown key, are reported with the file and the key at startup.

## Use

```
prs                  # interactive board
prs --once           # print the table and exit
prs --bots           # start on bot-authored pull requests
prs --repo gazebo    # start with only this repository selected, as a name or owner/name (repeatable)
prs --config <path>  # use a configuration file at this path
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

Board keys:

| Key | Action |
| --- | --- |
| `enter` | Diff view |
| `p` | Read or edit the description |
| `o` | Open in the browser |
| `v` | Check out the branch with `wt switch` and open it, with its changed files, in the configured editor |
| `d` | Toggle draft / ready for review |
| `e` | Request review from the configured team, marking a draft ready for review first |
| `m` | Enable or disable auto-merge (squash), after a confirmation |
| `f` | Open the filter menu: `up`/`down` (or `k`/`j`) move over the repositories and `space` toggles the one under the cursor; `1` to `9` toggle the first nine directly; `b` switches between people and bots |
| `r` | Refresh (also on the configured interval, every 3 minutes by default) |
| `q` | Quit |

Diff view keys:

| Key | Action |
| --- | --- |
| `↑` / `↓` | Previous / next file |
| `j` / `k`, `space` / `b` | Move the line cursor by a line or a page |
| `s` | Toggle split and unified layouts |
| `c` | Comment on the line under the cursor, or reply to the thread under it (`ctrl+s` posts) |
| `tab` | Choose the old or new side of a split row for the comment |
| `esc` | Back |
