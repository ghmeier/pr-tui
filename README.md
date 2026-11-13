# pr-tui

A terminal board of open GitHub pull requests across a fixed set of repositories. It shows
each pull request's draft or review state, CI status, lines changed, and requested
reviewers, and lets you act on them without leaving the terminal.

## Install

Requires Go and an authenticated [`gh`](https://cli.github.com).

```
make install   # builds to ~/.local/bin/prs
make test
```

The owner, repositories, review team, and editor are constants at the top of `main.go`.

## Use

```
prs                  # interactive board
prs --once           # print the table and exit
prs --bots           # start on bot-authored pull requests
prs --repo gazebo    # start with only this repository selected (repeatable)
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
| `v` | Check out the branch with `wt switch` and open it, with its changed files, in VS Code |
| `d` | Toggle draft / ready for review |
| `e` | Request review from the team, marking a draft ready for review first |
| `m` | Enable or disable auto-merge (squash), after a confirmation |
| `f` | Filter by repository, or switch between people and bots |
| `r` | Refresh (also every 3 minutes) |
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
