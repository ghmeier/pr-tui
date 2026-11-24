---
title: Request review from the team or one of its members
status: active
tags: [pull-requests, host]
next_step: Waiting for the user's approval of the plan, then dispatch the request-reviewer branch.
created: 2026-11-24
updated: 2026-11-24
---
# Request review from the team or one of its members

## Goal

On the pull request board and its screens, `t` is labelled "request" and opens a list of the
configured review team followed by the team's current members. The list scrolls, typing narrows
it, and enter requests review from the team or the person under the cursor. It replaces the
"request <team>" shortcut, which could only ask the whole team.

## Approach

### Host: a choice shown as a menu

`plugin.Choice` gains one field:

```go
// Menu shows the options in a list above the board, or at the bottom of a screen, instead of
// in the status line. It suits a choice that can have more options than number keys: the list
// scrolls, typing narrows it to the options that contain the typed text, ignoring case, the
// arrows move the cursor, enter picks the option under it, backspace removes a typed character,
// and esc cancels.
Menu bool
```

- While a menu choice is open, every key except ctrl+c goes to it, q included, on the board and
  over a screen.
- Drawn with `ui.MenuStyle` like the filter menu: the question, the typed text, at most 10
  options around the cursor with the cursor marker, a count such as "3 of 14" when the list is
  cut, and a help line (`↑↓ move  enter pick  esc cancel`). `Current` is bold, as in the
  status-line choice.
- On the board it goes in `aboveLines`, as the filter menu does. Over a screen the screen is
  drawn shorter by the menu's height and the menu goes below it.
- Enter with no option left does nothing. The status-line choice is unchanged.

### Pull request plugin

- `Row` gains `TeamMembers []string`: the review team's members in the organization that owns
  the pull request's repository, sorted. `load` fetches them once per distinct repository owner,
  concurrently with `PullRequests`; a failure leaves the rows without members rather than
  failing the board. `reload` copies them from the previous row, as it does `Viewer`.
- Provider interface: `RequestTeamReview(pullRequest)` becomes
  `RequestReview(pullRequest PullRequest, reviewer Reviewer) error`, with
  `type Reviewer struct { Login string; Team bool }`, and a new
  `TeamMembers(repository Repository) ([]string, error)` that returns nil when no team is
  configured.
- The `t` action stays only when a review team is configured. Its `LabelFor` is "request" when
  anyone is left to ask and empty otherwise. The options are the team as "<team> (team)" unless
  it is already requested, then each member who is not the author and not already requested.
- Picking an option keeps today's behavior: a draft is marked ready first, then review is
  requested, with a notice that names who, and the row is refreshed.

### GitHub provider

- Members: `gh api --paginate orgs/<owner>/teams/<review_team>/members --jq '.[].login'`.
- Request: `POST repos/<owner>/<name>/pulls/<number>/requested_reviewers` with
  `team_reviewers[]=<slug>` for the team and `reviewers[]=<login>` for a person.

No configuration change: `review_team` in `[github]` keeps its meaning. The starter file comment
and the README rows for `t` are reworded.

## Steps

1. **`request-reviewer`** from main: the host menu choice with its tests in `internal/plugin`,
   then the provider and plugin changes that use it, README and starter file. Verified with
   `go vet`, `gofmt`, `go test ./...`, and a read-only `--once` run.

## Risks

- The menu is the uncertain part (key routing over screens, drawing over a screen), so the
  implementer builds and tests it first.
- Listing team members needs the `read:org` scope, which gh's default login has. Without it the
  menu still offers the team.

## Notes and decisions

- 2026-11-24: The user chose the team plus its members over a members-only display, and a
  scrolling, filterable list over cutting the list at nine options.
