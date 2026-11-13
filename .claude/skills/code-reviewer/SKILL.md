---
name: code-reviewer
description: >
  Review a branch, a series of commits, or a pull request in prs and give important, actionable
  feedback. Use when asked to review changes, review a PR, or give feedback on local commits.
---

## How to review

Review is about understanding what the change is for and helping it get there, not running
a checklist. Match your effort to the stakes: a new host capability or a change to
configuration loading deserves close reading, and a wording fix deserves a glance.

Work from the foundation up, and stop at the level where you find problems. If the design
is wrong, do not also comment on names in code that will be rewritten.

**Architecture.** Form your own view of how the change should fit the architecture in
`CLAUDE.md` before reading the code, then compare.
- Is it in the right place? Generic board behavior belongs in the host, service knowledge in
  a provider, and wiring in `main.go`.
- Does it keep the boundaries? Look for a `gh` call or GitHub vocabulary outside the GitHub
  provider, the host or `internal/ui` learning about a specific plugin, and one plugin
  reaching into another beyond its exported row and domain types.
- Does a new extension point have a real use in the same change? Is an existing one enough?
- Is anything personal compiled in: a repository, organization, team, username, editor, or
  path?

**Functionality.** Read closely and ask how it could break.
- Concurrency: `Load` functions and fetches run in goroutines while cells render on the
  update loop. Is any state shared between them without a lock?
- Messages: does a screen act on a message that belongs to another screen or to an earlier
  selection? Is a result lost when it arrives after its screen closed?
- Refresh: does the cursor stay on the same row, and does background column data survive?
- Layout: narrow terminals, wide characters, an empty board, more rows than fit.
- Configuration: are unknown keys rejected, do errors name the table and key in plain
  language and the file once, and do defaults work with the key omitted?
- External commands: are arguments passed as a list and never through a shell, and is
  anything from a remote service sanitized before it is drawn in the terminal?
- Is a credential ever written to the file, a log, or an error?

**Maintainability.**
- Do tests cover the behavior at the right layer (host, plugin, provider), and do they
  follow the test conventions in `CLAUDE.md`? Flag tests that restate the implementation,
  duplicate another test's invariant, or call the network.
- Is the exported surface as small as it can be?
- Was code moved, or needlessly rewritten?

**Readability.** Lowest priority; `gofmt` and `go vet` handle formatting. Two things are
rules here and worth flagging in code the change adds or edits, with the rewrite included:
- Names that are abbreviated or need decoding.
- Comments that restate the code, or that mention a plan, ticket, brief, or step number.

## Feedback

State the severity of every point so the author never has to guess:

- **Blocking**: a bug, a broken boundary, a compiled-in preference, a leaked credential, or
  lost user input. Must be fixed.
- **Non-blocking**: a real improvement that should be made, at the author's judgment.

Every point names the file and line and gives a concrete alternative. Say only what the
author does not already know, and skip praise and summaries of the change.

## Workflow

### 1. Identify the changes

```bash
git log main..<branch> --oneline && git diff main...<branch> --stat   # a local branch
gh pr view <number> --json title,body,baseRefName,headRefName,files   # a pull request
gh pr diff <number>
```

### 2. Build context

Read the commit messages or the pull request description, and the plan in `.apl/` if one
covers the work. Read every changed file in full, not only the changed lines, and the tests
beside it.

### 3. Check that it works

In the branch's worktree, run `go vet ./... && gofmt -l . && go test ./...`. When the change
affects what a board shows, run `go run . --config <temporary file> --once`, which is
read-only. Do not press keys that change real data.

### 4. Deliver

**Local branch.** Return the review as your final output: blocking points first, then
non-blocking, each as `path/file.go:line` followed by the problem and the alternative. Do
not edit the code.

**Pull request.** Post each point as an inline comment on its line, starting with the
severity. Use a top-level comment only for design feedback that fits no line.

```bash
gh api repos/{owner}/{repo}/pulls/<number>/comments --method POST \
  -f body="**Blocking**: ..." -f commit_id="<head sha>" -f path="path/file.go" -F line=42
```
