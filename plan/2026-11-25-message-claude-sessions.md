---
title: Message Claude sessions from pull requests and plans
status: active
tags: [plugins, agent-sessions]
created: 2026-11-25
updated: 2026-11-25
next_step: Get the user's approval and the result of the relay test, then dispatch step 1.
---
# Message Claude sessions from pull requests and plans

## Goal

The pull request board and the agent plans board show which live Claude Code session is
working on each row. A key opens the editor, and the saved text is delivered to that session
as a cross-session message, so feedback on a pull request or plan reaches the agent without
copying names and comments into its chat.

## Approach

### What Claude Code offers

- `claude agents --json` lists live sessions, interactive and background: `sessionId`,
  `name`, `cwd` (launch directory), `kind`, `status` (`idle`/`busy`) or `state`
  (`blocked`), `startedAt`. Documented as the scripting interface.
- Each session's transcript is `~/.claude/projects/<directory slug>/<sessionId>.jsonl`. Every
  entry carries `cwd` and `gitBranch`. The slug rule is not documented, so the provider
  finds the file by globbing `~/.claude/projects/*/<sessionId>.jsonl`.
- There is no documented command to send a message. The user chose a relay: a one-shot
  `claude -p` whose only tool is SendMessage, addressed by session id. The undocumented
  socket in `/tmp/cc-socks` was rejected because an update could break it.

### Matching rows to sessions

The user's sessions mostly stay on `main` and work through worktrees, so branch matching
alone misses them. The provider reads the last 2 MiB of each live session's transcript and
collects:

- pull request URLs, `github.com/<owner>/<name>/pull/<number>`, as `{Repository, Number}`;
- apl document paths, `.apl/<type>/<file>.md`, as `<type>/<file>.md`;
- the `cwd` and `gitBranch` of the newest entry.

A pull request row matches a session when the session mentions its URL, or when the
session's branch equals the head branch and the session's repository name equals the pull
request's. A plan row matches when the session mentions the document's path and the
session's repository name equals the row's repository. The repository name is the base name
of the parent of `git rev-parse --git-common-dir` run in the session's directory, so a
worktree reports its main checkout's name. Bare `#123` is ignored because it is ambiguous
across repositories.

### Plugin `plugins/agentsessions`, name `agent_sessions`

Neutral types, no knowledge of Claude Code:

```go
type Status string // StatusIdle, StatusBusy, StatusBlocked, StatusUnknown

type PullRequestReference struct {
	Repository string // "owner/name"
	Number     int
}

type Session struct {
	ID           string // stable key
	Name         string
	Repository   string // base name of the checkout, "" when not in a git repository
	Branch       string
	Status       Status
	LastActive   time.Time
	PullRequests []PullRequestReference
	Documents    []string // relative to .apl, such as "plan/2026-11-25-x.md"
}

type Provider interface {
	Sessions() ([]Session, error)
	Send(session Session, message string) error
}
```

The provider registers itself with the plugin the way the GitHub providers do.

**Provider `plugins/agentsessions/claude`**: the only package that runs `claude` or reads
`~/.claude`. `Send` runs:

```
claude -p --model <relay_model> --no-session-persistence --name prs \
  --strict-mcp-config --disable-slash-commands \
  --tools=SendMessage,ListAgents --allowed-tools=SendMessage,ListAgents  < prompt
```

The prompt goes on standard input, because `--tools` and `--allowed-tools` take a list and
swallow a trailing prompt argument. It names the session id and the message between markers,
and asks for the word `sent` back; any other output is the error.

**Column "Session"** on the pull request board (when `host.Enabled(pullrequests.BoardName)`)
and on the agent plans board (when `host.Enabled(agentplans.BoardName)`). One fetch per load
calls `Sessions()` and delivers, per row key, the matching sessions, most recently active
first. The cell shows the first session's name and status, dimmed when busy, plus `+N`.

**Action `c` "message session"** on both boards. It reads the column's data from
`selection.Data("Session")`. No match: notice "No live Claude session mentions this".
One match: an `Edit` prefilled with a first line naming the row (`About pull request
owner/name#123 (title):` or `About plan <title> (.apl/<file>):`) and a blank line. Several:
a `Choice` of session names first. Saving runs `Send` as the effect's `Command` with the
notice "Sending to <name>…"; an empty text sends nothing.

### Configuration

```toml
[agent_sessions]
# Model that relays each message to a session. A small model keeps a send to a few seconds.
relay_model = "haiku"
```

Neutral default `haiku`; an empty string is an error naming `[agent_sessions] relay_model`.
Authentication: the relay uses the `claude` login. Nothing in the file.

### Host changes

None. Columns, actions, `Edit`, `Choice`, and `Command` cover it.

## Steps

1. **`agent-sessions-pull-requests`** from `main`. The plugin, the Claude provider (list,
   transcript scan, relay send), the Session column and the message action on the pull request
   board, configuration, starter file, README. Provider tests replace `shell.Run` and use a
   temporary home with canned transcripts. Verify: tests, `--once` showing a live session on
   a pull request it mentions, and one real send to a session the user names.
2. **`agent-sessions-plans`** from step 1. The Session column and message action on the agent
   plans board. Verify: tests and `--once` against `~/git/gazebo`.

## Risks

- **Permission-mode hold.** A session in a different permission mode from the sender may hold
  cross-session messages for approval. The relay runs in the default mode; the user's
  sessions run in auto mode. The relay test before step 1 shows whether messages arrive
  directly; if not, add `relay_permission_mode` to the table.
- Each send costs a few seconds and a small model call. Accepted by the user.
- Transcript format and location are undocumented. Only the provider knows them, and a
  missing transcript leaves a session listed with no references rather than failing the load.
- Reading 2 MiB per live session each refresh. With a handful of sessions this is a few
  megabytes; measured in step 1.

## Notes and decisions

- 2026-11-25: The user chose the `claude -p` relay over the local socket, transcript
  references over branch-only matching, and a column plus action over a separate board.
- 2026-11-25: Key `c` for "message session" is my choice; cheap to change.
- 2026-11-25: Relay test by the user: a message from a default-mode relay reached an
  auto-mode session directly, so no permission-mode setting is needed. SendMessage does not
  accept a session id as an address; it takes the session's name. `--tools` limits only
  built-in tools: the relay's model tried a Slack MCP tool, so the relay passes
  `--strict-mcp-config` (no MCP servers) and `--disable-slash-commands`. `--bare` is not used
  because it ignores the `claude` login. `--name prs` labels the sender.
- 2026-11-25: Auto mode blocked my own relay test from inside a Claude session, so the user
  runs it.
