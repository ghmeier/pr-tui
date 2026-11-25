---
title: Message Claude sessions from pull requests and plans
status: active
tags: [plugins, agent-sessions]
created: 2026-11-25
updated: 2026-11-25
next_step: Step 1 (6132e01) is in the refactor pass; then verify and one real send.
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
claude -p --safe-mode --model <relay_model> --effort low --no-session-persistence --name prs \
  --strict-mcp-config --tools=SendMessage --allowed-tools=SendMessage \
  --output-format json --system-prompt <relay instructions>  < prompt
```

The system prompt replaces Claude Code's default one, so a send costs little input:

```
You relay one message. Call SendMessage once, with to set to the Recipient and message set to
everything after Message:, unchanged. Then reply with only the word sent, or the error
SendMessage returned.
```

Standard input carries `Recipient: <name>`, a line `Message:`, and the text. It goes on
standard input because `--tools` and `--allowed-tools` take a list and swallow a trailing
prompt argument. `Send` parses the JSON result: it succeeds when `is_error` is false and
`result` is `sent`, and otherwise fails with `result` as the error. Parsing JSON keeps
warnings such as Claude Code's Code Mode notice from reading as failures. `--safe-mode` drops
CLAUDE.md, skills, plugins, hooks, MCP servers, and agents but keeps the `claude` login,
which `--bare` does not.

**Column "Session"** on the pull request board (when `host.Enabled(pullrequests.BoardName)`)
and on the agent plans board (when `host.Enabled(agentplans.BoardName)`). One fetch per load
calls `Sessions()` and delivers, per row key, the matching sessions, most recently active
first. The cell shows the first session's name and status, dimmed when busy, plus `+N`.

**Action `c` "message session"** on both boards. It reads the column's data from
`selection.Data("Session")`. No match: notice "No live Claude session mentions this".
One match: an `Edit` prefilled with the row's metadata and a blank line. For a pull request:
`About pull request owner/name#123: <title>`, `URL: <url>`, `Branch: <head branch>`. For a
plan: `About plan: <title>`, `File: .apl/<file>`, `Repository: <name>`, `Status: <status>`. Several:
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

3. **`agent-sessions-diff-lines`** from step 2. Message a session from the diff screen with the
   file and line (or selected lines) under the cursor in the prefill. Screens run board
   actions through `Selection.RunAction`, which carries only the row, so the host gains a way
   for a screen to pass what it is showing with the action (for example
   `Selection.RunActionWith(key, context any)` and `Selection.Context`), and
   `plugins/pullrequests` exports a neutral `DiffPosition{Path, Line, Side, EndLine}` that the
   agent sessions action reads. `c` is already comment on the diff screen, so this needs
   another key there. Verify with host tests for the context and a plugin test for the prefill.

## Risks

- **Permission-mode hold.** A session in a different permission mode from the sender may hold
  cross-session messages for approval. The relay runs in the default mode; the user's
  sessions run in auto mode. The relay test before step 1 shows whether messages arrive
  directly; if not, add `relay_permission_mode` to the table.
- Addressing is by name, because ListAgents' references are not derived from session ids.
  When two live sessions share a name, `Send` fails with "Two Claude sessions are named <name>;
  rename one with /rename" without running the relay. A background session that `claude
  agents --json` lists may not be reachable; SendMessage's error is shown as the failure.
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
- 2026-11-25: The user asked for the strictest relay. Tested with `--safe-mode`, `--effort low`, a
  one-sentence `--system-prompt`, and JSON output: the message arrived, from the name `prs`.
- 2026-11-25: Strict relay baseline: 2.9 s end to end, about 6,600 input tokens (mostly the
  cache), 122 output tokens, $0.0008 per send on Haiku. The user approved the plan.
- 2026-11-25: The user asked for metadata in the prefill. Step 1 got the pull request's number,
  title, URL, and branch. File and line need the diff screen, added as step 3.
- 2026-11-25: Auto mode blocked my own relay test from inside a Claude session, so the user
  runs it.
