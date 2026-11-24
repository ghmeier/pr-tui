---
title: Reviewer menu and the kitty editor wait
status: completed
tags: [pull-requests, host]
created: 2026-11-24
updated: 2026-11-24
---
# Reviewer menu and the kitty editor wait

Plan: [2026-11-24-request-reviewer-menu](../plan/2026-11-24-request-reviewer-menu.md)

## What was built

- Pull request #19, branch `request-reviewer`: `t` opens a menu of the review team and its members.
  - `internal/plugin/menu.go` adds `Choice.Menu`, a scrolling list that narrows as you type.
  - `plugins/pullrequests/plugin.go` loads `Row.TeamMembers` once per repository owner, alongside each load.
  - `plugins/pullrequests/actions.go` builds the options.
  - `plugins/pullrequests/github/actions.go` adds `TeamMembers` and `RequestReview`.
- The stuck "Writing in your editor…" notice was a kitty 0.43.1 bug in `--wait-for-child-to-exit`, fixed by upgrading kitty to 0.51.1. Pull request #18, a polling workaround, was closed.

## Gotchas

- A menu choice takes every key except ctrl+c, q included. Over a screen, the screen is drawn shorter to make room for the menu.
- A failed member lookup is silent, and the list then offers only the team.

## Next steps

- Consider showing why members are missing when the lookup fails.
