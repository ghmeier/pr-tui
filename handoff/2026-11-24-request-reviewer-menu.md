---
title: Reviewer menu and the kitty editor wait
status: active
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
- Pull request #18, branch `kitty-text-command`: the README and starter-file example `text_command` no longer relies on `--wait-for-child-to-exit`, which never returns in kitty 0.43.

## Gotchas

- A menu choice takes every key except ctrl+c, q included. Over a screen, the screen is drawn shorter to make room for the menu.
- A failed member lookup is silent, and the list then offers only the team.
- A prs that was already running when the config changed keeps the old text command. The `kitten @ launch` from that run is still waiting.

## Next steps

- Try the menu by hand without submitting a request, then merge #18 and #19.
- Consider showing why members are missing when the lookup fails.
