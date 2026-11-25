---
title: Message Claude sessions from pull requests and plans
status: active
tags: [plugins, agent-sessions]
created: 2026-11-25
updated: 2026-11-25
next_step: Check one real send from the app, mark #20 ready; then the agent plans board step from diff-review-keys.
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
2. **`agent-sessions-diff-comments`** from step 1. On the diff screen `c` stays comment. After the
   editor is saved, a numbered choice asks where the comment goes: `1` post it to the pull
   request now (`Provider.PostComment` with the line target), `2` add it to the pending review
   (today's behaviour), `3` send it to the agent session (only offered when a session matches).
   A reply on a posted thread offers only post and send. The message carries the pull request
   metadata plus the file, line, and side.

   The diff screen belongs to the pull request plugin and must not know about agent sessions,
   so the host gains actions that screens offer with a value instead of a key:
   - `Action.Takes string`: the kind of value the action works on, such as "comment". Such an
     action has no key on the board and is not in the help line.
   - `Action.Applies func(selection Selection) bool`: optional; whether it is offered for this
     selection, so the agent option appears only when the Session data has a match.
   - `Selection.Offers(kind string) []string`: the labels of the board's applicable actions
     taking that kind, in the order they were added.
   - `Selection.RunOffer(label string, value any) tea.Cmd` and `Selection.Value any`, which
     the action's `Run` reads.
   `plugins/pullrequests` exports `Comment{PullRequest, Target CommentTarget, Body}` as
   the "comment" value. The agent sessions plugin adds an action with `Takes: "comment"` and
   label "send to agent session" that sends the body with the metadata, picking a session first
   when several match. Verify with host tests for offers and a plugin test for each option.
3. **`agent-sessions-plans`** from step 2. The Session column and message action on the agent
   plans board. Verify: tests and `--once` against `~/git/gazebo`.

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
- 2026-11-25: Step 1 on `agent-sessions-pull-requests`: `6132e01`, refactor `142203f`. Verified: vet,
  gofmt, 976 tests, and `--once` against the real repositories shows gazebo-30 on #21493 and
  #21488 and gazebo-56 on #21489. `Sessions()` takes about 0.2 s for five sessions. The relay
  accepts "sent" ignoring case and trailing punctuation. Left alone: the provider recognises
  `github.com/.../pull/N` URLs in transcripts, a text heuristic like the agent plans board's;
  the repository in a URL is compared with exact case; the relay's `--name prs` is fixed.
- 2026-11-25: The user wants a live session shown as an emoji in the State column instead of a
  Session column. The host gains `Column.Within`: a column whose cell is drawn at the end of
  another column's cell on the same board. The agent sessions column joins
  `pullrequests.StateColumn` with 🤖 (my choice of emoji; cheap to change). Step 2 joins the
  agent plans board's Status column the same way.
- 2026-11-25: `Column.Within` and the 🤖 mark: `7c1e6cf`, refactor `447e8bd`. Verified: 984 tests,
  `--once` shows 🤖 after the CI mark on the three matched pull requests. Validation runs after
  every plugin's columns are attached, so a column added by another plugin is checked. Left
  alone: a long joined mark can push the host column's own text past its `MaxWidth`.
- 2026-11-25: The user wants the diff screen's comment key kept, with a choice after saving:
  post to the pull request, add to the review, or send to the agent. Step 3 rewritten for that;
  it replaces the separate diff-screen key.
- 2026-11-25: The user approved the comment choice and its host change. It becomes step 2 and the
  agent plans board step 3. Comments have no line ranges, so the value carries one line.
- 2026-11-25: Step 2 on `agent-sessions-diff-comments`: `0439a73`, refactor `dccbc3c`, 1021 tests. The
  host also gained `Effect.Then` (a screen's own command started with an effect, so posting from
  the choice reaches the diff screen) and `Choice.Cancel` (esc keeps the comment's text).
- 2026-11-25: Code review of step 2 found offers checked against stale data and comments lost on the
  offer path; fixed in `4649817` (1031 tests). The user also asked for `c comment` and `a review`
  everywhere (the label always "review"; `a` kept on the screens for consistency), the pending count on the review label, and a confirmation before leaving the
  diff screen with pending comments: branch `diff-review-keys` from step 2.- 2026-11-25: `diff-review-keys`: `46f9c7a`, refactor `1cd7862`, 1042 tests. Review label always
  "review" with "(N pending)"; esc or q on the diff screen asks while comments are pending.
- 2026-11-25: Pull requests opened, stacked: #20 (step 1, draft until a real send is checked), #21
  (step 2), #22 (review label and leave confirmation). The user approved 2 and 3.
- 2026-11-25: Auto mode blocked my own relay test from inside a Claude session, so the user
  runs it.
