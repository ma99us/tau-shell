# 07 - Conversation History Browser

Status: **future planning**. Rank 7. Sources: Orca (conversation names in tabs); user value from existing log parsing.

## Problem

One TauShell session can span several harness conversations (Start fresh rotates the
harness ID). TauShell already reads these logs for usage and model
(`SessionUsage.conversations`), but the user cannot see, search or choose a past
conversation. Resume always targets the latest saved ID.

## User stories

- As a user, I want to see past conversations of a session with title and dates, so that I
  recall what each was about.
- As a user, I want to search conversation titles, so that I can find one quickly.
- As a user, I want to resume a chosen conversation where the harness supports it.

## In scope

- Per session list of conversations: title (when the harness provides one), first and last
  activity, model(s), token and cost totals from the existing ledger.
- Search by title.
- Resume a selected conversation through the adapter, with the usual restore truthfulness.
- Show the harness conversation ID with a copy action.

## Out of scope

- Showing or searching full conversation content.
- Editing, deleting or exporting harness logs.
- Merging conversations across sessions.

## Acceptance criteria

- The list is built only from logs the harness itself wrote; unreadable or unknown formats
  show "unavailable" for that harness version.
- Resuming a selected conversation reports success only when the harness confirms; failure
  stays visible ([finding 2.7](../02-cross-cutting.md#27-restore-truthfulness)).
- Selecting a conversation does not change the saved latest-resume ID unless the user
  confirms.
- Logs are read locally; no content is copied into TauShell's state beyond titles and
  totals already kept.
- Deleted harness logs are handled without errors.

## Dependencies

- [Finding 2.5](../02-cross-cutting.md#25-undocumented-harness-formats). Benefits from
  [10](10-declarative-adapters.md) (declared log paths).

## Risks

- Log formats and locations differ per harness and version.
- Titles may be missing, so lists may look anonymous.
- Resuming an older conversation may fail in ways the harness does not report.

## Spikes

- Which harnesses expose titles and how (Copilot `events.jsonl`, Claude `projects/*/<id>.jsonl`).
- Per-harness support for resuming by ID versus "most recent".

## Open questions

- Is title-only enough, or is a first-prompt excerpt needed (privacy implications)?
- Where should the list live: session page, Home tile, or a dialog?

## Docs to update when implemented

`04-session-restoration.md`, `12-storage-layout.md`, README, adapter docs.
