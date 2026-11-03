# 06 - Status Insights (Quota and Git Chips)

Status: **git chip built; quota chip partial (Copilot only)**. Rank 6. Sources: Orca usage tracking; user decision (light git).

**Built:** the toolbar branch chip now comes from `git status --porcelain=v2 --branch` (`presets.GitStatus`, 4 s limit, `--no-optional-locks`, polled with the existing 5 s toolbar refresh). It shows `branch*` when the tree is dirty and `+N -M` when ahead/behind; with no upstream the counts are omitted and the tooltip says so. A folder that is not a repository shows no chip. Tested by `TestParseGitStatus` and `gitChip.test.ts`. Not clicked through live. **Not built:** Claude quota (no reliable source found yet), per-harness budget fallback, chips on Home.

## Problem

TauShell already shows tokens and cost per session and per month from harness logs, and an
initial Copilot quota chip is implemented. It does not show quota information for other
harnesses or whether the working tree is dirty or out of sync, so users may still miss a
limit or uncommitted changes.

## User stories

- As a user, I want to see my remaining Copilot or Claude quota and the reset time, so that I
  can pace work before an agent stalls.
- As a user, I want a warning near a limit (Orca uses 80%), so that I can react.
- As a user, I want to see whether the session's repo is dirty or ahead/behind, so that I
  notice uncommitted agent changes at a glance.

## In scope

- **Quota chip** per supported harness: usage against the plan, reset time, warning
  threshold. The Copilot implementation uses `gh api /copilot_internal/user`, polls every
  five minutes with a five-minute cache, and can be disabled in configuration; see the
  [MVP backlog](../../v2/20-mvp-backlog.md#copilot-quota-chip-done). Extend to Claude only
  where a reliable source exists.
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

Keep `11-security-model.md`, `12-storage-layout.md`, README and the backlog current as
additional providers are implemented; Copilot quota support is already recorded there.
