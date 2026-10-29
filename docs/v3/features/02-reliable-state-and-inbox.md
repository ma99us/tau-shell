# 02 - Reliable Agent State and Notification Inbox

Status: **future planning**. Rank 2. Sources: V3 ideas #6 and #8; Orca notifications.

## Problem

Session state comes from screen and title cues (`frontend/src/attention.ts`): Copilot's
"Working ... esc" row and selection footer, Claude Code's title spinner. These are
harness-specific heuristics, and Claude Code's "needs input" cue for tool permissions was
never captured. "Quiet" does not prove the harness is waiting. Alerts are fire-and-forget:
a toast or flash is easy to miss, there is no list of what needs attention, no sound, no
per-session mute and no tray icon
([`22-local-attention-notifications.md`](../../v2/22-local-attention-notifications.md)).

Anything that acts on state automatically ([03](03-prompt-dispatch.md),
[01](01-control-cli-and-api.md) `wait`, [04](04-mobile-companion.md) pushes) needs a more
trustworthy signal.

## User stories

- As a user, I want to know when an agent finished or needs me, without watching the
  terminal, so that I can work elsewhere.
- As a user, I want an inbox of items that need attention that survives me missing a toast,
  so that nothing is lost.
- As a user, I want to silence noisy sessions and pick a sound, so that alerts stay useful.
- As a user, I want state labels (working, waiting for input, waiting for approval, done,
  failed) I can trust, so that automation never types into the wrong moment.

## In scope

- Prefer **signals the harness reports** (hooks, plugin events, structured logs, title or
  OSC events) over screen scraping, per adapter and capability-probed; keep the current
  cues as fallback.
- A defined state set (see V3 ideas #6), shown consistently in Home, sidebar and tabs.
- A notification inbox: unread items per session, mark unread, click to open the session,
  badge count, optional tray icon.
- Optional sound per category; per-session mute.
- A confidence marker so consumers can tell "confirmed idle" from "probably idle".

## Out of scope

- Remote push delivery (see [04](04-mobile-companion.md)).
- Whole-screen comparison or output-silence detection as a primary signal (rejected in the
  Phase 1 design).
- Rewriting provider output.

## Acceptance criteria

- For each supported harness, the doc table of signals states which are harness-reported
  and which are heuristic, verified live against a named harness version.
- A permission or selection prompt is reported as "waiting" for every harness whose prompt
  has been captured; an unknown prompt is never reported as idle with high confidence.
- Missing a toast leaves an unread inbox entry; clicking it opens the right session.
- Muting a session suppresses its toast, flash and sound but keeps its inbox entries.
- State changes are not logged with terminal content.
- Switching a harness's signal source does not change the UI vocabulary.

## Dependencies

- Start gate: Claude Code cue verification is a precondition.
- [Finding 2.3](../02-cross-cutting.md#23-idle-detection-is-a-heuristic),
  [finding 2.5](../02-cross-cutting.md#25-undocumented-harness-formats).
- Feeds [01](01-control-cli-and-api.md), [03](03-prompt-dispatch.md), [04](04-mobile-companion.md),
  [10](10-declarative-adapters.md) (cue schema).

## Risks

- Hooks and plugins differ per harness, are versioned and may change without notice.
- Copilot sends no BEL/OSC 9/777/133, so some harnesses may stay heuristic.
- A wrong "idle" is worse than "unknown"; confidence must be honest.

## Spikes

- Inventory per harness: hooks, plugin events, logs, OSC, title changes.
- Whether hook-based signals can be loaded without editing the user's own harness config
  (as skills use `--plugin-dir`).
- Tray icon and toast behaviour on Windows 10 with Focus Assist.

## Open questions

- Which states beyond the current five are worth surfacing?
- Should the inbox persist across restarts?
- Is a sound on by default acceptable?

## Docs to update when implemented

`08-attention-detection.md`, `22-local-attention-notifications.md`, `07-notification-engine.md`,
`18-yaml-schema.md`, README, handoff.
