# 06 - Status Insights (Quota and Git Chips)

Status: **future planning**. Rank 6. Sources: Orca usage tracking; user decision (light git).

## Problem

TauShell already shows tokens and cost per session and per month from harness logs. It does
not show **how close the user is to a plan limit** or when it resets, so an agent can stall
unexpectedly. The top bar shows the git branch but not whether the working tree is dirty or
out of sync.

## User stories

- As a user, I want to see my remaining Copilot or Claude quota and the reset time, so that I
  can pace work before an agent stalls.
- As a user, I want a warning near a limit (Orca uses 80%), so that I can react.
- As a user, I want to see whether the session's repo is dirty or ahead/behind, so that I
  notice uncommitted agent changes at a glance.

## In scope

- **Quota chip** per supported harness: usage against the plan, reset time, warning
  threshold. Starts from the Copilot research note in the
  [MVP backlog](../../v2/20-mvp-backlog.md#research-copilot-quota-chip-not-started)
  (`gh api /copilot_internal/user`, polled about every 15 minutes and on Refresh), then
  Claude where a reliable source exists.
- Clear "unavailable" and "not signed in" states.
- **Git chip**: branch (exists), dirty marker, ahead/behind counts, refreshed on a light
  poll or on focus.
- Opt-out for any network poll.

## Out of scope

- Account switching or hot-swapping logins.
- Estimating billing; the existing ledger already covers cost.
- Any git action (commit, push, diff, branch switching) and worktrees.

## Acceptance criteria

- Chips never show a number the source did not provide; unknown reads "unavailable".
- The quota poll is off or clearly disclosed in settings, and does nothing without a `gh`
  login; no credentials are logged or stored by TauShell.
- Git status calls do not block the UI and are bounded in time on large repos.
- A folder that is not a repository shows no git chip, without an error.
- Behaviour is documented per harness with the version tested.

## Dependencies

- None blocking. Quota sources are undocumented ([finding 2.5](../02-cross-cutting.md#25-undocumented-harness-formats)).

## Risks

- The Copilot endpoint is undocumented and usage-based billing may replace quotas.
- Claude may have no stable local or API source for plan limits.
- Extra outbound calls need to be visible in the security model.

## Spikes

- Which quota sources exist per harness and what they return.
- Cost of `git status` on large repos; poll interval and timeouts.

## Open questions

- Show chips in the top bar, Home, or both?
- Is a per-harness monthly budget the user sets a useful fallback when no source exists?

## Docs to update when implemented

`11-security-model.md`, `12-storage-layout.md`, README, backlog (close the research item).
