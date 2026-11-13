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
```

The repositories and the review team belong to the provider, so they live in the table named
after it. Without `review_team`, the `e` key is disabled and no team is highlighted or counted
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
| `f` | Filter by repository and by people or bots: `j`/`k` move, `space` toggles, `1` to `9` toggle the first nine options, `b` switches people and bots |
| `tab` / `shift+tab` | Switch boards, when plugins add more than one |
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
