---
title: Code review, one editing pattern, and diff fixes
status: closed
tags: [pull-requests, host]
created: 2026-11-24
updated: 2026-11-24
---
# Code review, one editing pattern, and diff fixes

Plan: `.apl/plan/2026-11-24-review-and-diff-fixes.md`. All four steps merged as pull requests #9 to #12.

## What was built
- **Host `Edit` effect** (`internal/plugin/edit.go`): opens `$VISUAL`/`$EDITOR`/vi on a temp file, hands the trimmed
  text to `Done`, and re-enables mouse cell motion afterwards. Bubble Tea's `ExecProcess` turns mouse reporting off and
  never back on, which was the "double-click to copy stops working" bug. Tests replace `plugin.RunEditor`.
- **`plugin.CommandFinished`**: delivered to open screens after an effect's command finishes.
- **Diff wrapping**: `ui.WrapSegments`; diff lines render as parts; `viewRow.part`; the cursor skips continuation
  parts; rows are cached by file, layout, width, and a data version (plus the pending-review store version).
- **Fresh diffs**: the cache is keyed by URL and always revalidated on open; the replacement keeps file and line.
- **One-sided files** (new/deleted) are shown unified; `s` on one overrides only that file (`showsSplit`).
- **Pending review** (`plugins/pullrequests/pending.go`, `review.go`): drafts per pull request behind a mutex;
  `c` writes through the editor; `a` is "review"; `Provider.SubmitReview` replaced `Approve`.
- Workflow run form submits with `enter`.

## Gotchas
- Anything that hands the terminal to another program must go through `Effect.Edit` (or re-enable the mouse itself).
- Pending reviews are in memory only; quitting drops them (documented in the README).
- The review payload has no `commit_id` on purpose; `PostComment` still sends one for single comments.
- Outdated drafts are drawn at the top of their file (or the first file) so `c` can still reach them.

## Next steps
- Not verified by hand in a terminal: the mouse fix in kitty, wrapped diffs on a real pull request, and the review
  flow end to end. Try them on a pull request where a test review is harmless.
- Possible follow-ups: confirm before quitting with pending comments; a release tag when the user wants one.
