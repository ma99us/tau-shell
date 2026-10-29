# 03 - Prompt Dispatch (Queue and Broadcast)

Status: **future planning**. Rank 3. Sources: Orca parallel agents and automations (adapted); user decision.

## Problem

Prompt presets exist, but a prompt is sent now or not at all. A user who thinks of the next
instruction while a harness is still working must remember it, and sending the same prompt
to several sessions means pasting it repeatedly.

## User stories

- As a user, I want to queue more prompts while a session is working, so that they go out
  one by one without me watching.
- As a user, I want one prompt sent to several chosen sessions, so that I can compare
  approaches or give a shared instruction.
- As a user, I want the queue to **never interrupt** a running turn, so that I do not
  corrupt work in progress.
- As a user, I want to see, reorder and cancel queued items, so that I stay in control.

## In scope

- A per-session queue of prompts (typed or from presets). The next item is sent only when
  the previous turn is done and the harness is **confirmed idle**.
- Broadcast: send a prompt (or enqueue it) to a user-selected set of sessions.
- Visible queue state: position, target, status (waiting, sent, skipped, failed).
- Optional side-by-side view of several sessions for comparison.
- Warning when selected sessions share a working directory.

## Out of scope

- Time-based schedules and recurring prompts (user chose "queue only").
- Interrupting, cancelling or editing a running turn.
- Git worktrees to isolate parallel agents.
- Auto-answering permission or selection prompts.

## Acceptance criteria

- An item is never typed while the session is working or waiting for an answer.
- With only heuristic idle state, items wait for a manual "send next"; automatic sending is
  enabled only for harnesses with confirmed-idle signals ([feature 02](02-reliable-state-and-inbox.md)).
- If a session ends, fails or is stopped, its queue pauses and reports it; items are not
  lost silently.
- Closing TauShell with a non-empty queue warns and states what happens to the items.
- Broadcast reports per session whether the prompt was sent, queued or refused.
- Each automatic send is recorded in the audit trail ([finding 2.2](../02-cross-cutting.md#22-programmatic-input-reaches-agents-that-run-without-permission-checks)); prompt text is not logged.
- A session with a resume warning or no running process is skipped with a reason.

## Dependencies

- [Feature 02](02-reliable-state-and-inbox.md) (hard dependency for automatic sending).
- [Feature 01](01-control-cli-and-api.md) for scripted dispatch (optional).

## Risks

- A wrong idle signal can type into a prompt or a permission dialog.
- Unattended queues can run up cost; consider a visible count and a per-day cap.
- Several agents editing one folder can conflict.
- Whether the queue persists across restarts is a safety question: a stale prompt may no
  longer make sense.

## Spikes

- Confirm per harness that text sent right after a turn ends is accepted and not swallowed.
- Side-by-side layout with several xterm instances and sizing.

## Open questions

- Persist the queue across restarts, or clear it?
- Should a queued prompt expire?
- Is a per-day cap needed in V3, or only a visible counter?

## Docs to update when implemented

`08-UI-Design.md` (widgets), README, `18-yaml-schema.md` if presets gain queue behaviour.
