# Conversations: New conversation, History and Resume (plan)

Status: implemented 2026-11-25 (steps 2-7 done; step 1 spike done through the live tests; step 8 partial). See [`04-session-restoration`](04-session-restoration.md#conversations-the-harness-starts-itself) and the [handoff](21-engineering-handoff.md#conversations-new-conversation-history-in-harness-switches) for behavior and verification. Findings: Claude Code `/clear` and Copilot CLI `/new` create a new conversation ID; Copilot CLI `/clear` does not. Decision: tracking is suppressed while a session is closed to switch (`managedSession.switching`), because the old launch's final poll otherwise flipped the ID during a running-resume.

## Problem

- The toolbar Clear button sends the harness `/clear`. The harness then starts a new conversation, but TauShell keeps the ID it recorded at launch. The History drawer lists only IDs TauShell recorded (usage ledger plus the runtime row), so a cleared conversation never appears, and the next Resume continues the pre-clear conversation. The Clear button was hidden in the UI as an interim step.
- History cannot switch conversations while the session runs (Resume is disabled).
- History and the conversation controls are not grouped on the right toolbar.

## Requested behavior (from the user)

1. Replace Clear with a "New conversation" action that restarts the session and records the new conversation in History.
2. Resume in History is also enabled for a running session. Pressing it shows a warning that the current session will be closed (same style as the old Clear dialog), then switches.
3. The conversation being left stays in the History list, so the user can return to it later.
4. A regular session Resume always resumes the latest conversation that was active when the harness closed, for any reason.
5. Move the History button on the right toolbar next to the New conversation button.

## Decisions so far

- Plan document: `docs/v2/28-conversations-plan.md` (user choice).
- Detection (2026-11-25): TauShell detects conversation changes made inside the harness (/clear, /new) by scanning the harness's own logs for the newest conversation in the session folder, on stop and while running. Best effort per harness; where unreliable, that harness tracks only conversations started from the New conversation button.
- Mechanism (2026-11-25): New conversation = confirm dialog, graceful stop, fresh launch (existing StartFresh path), so the new ID is known from the start. No `/clear` injection.
- Assumed (no objection raised): Resume on a running session uses the same mechanism: confirm dialog stating the running session will be closed, graceful stop, then resume the chosen conversation. The outgoing conversation stays in History.

## Research

- None found yet; only established sources count. Per-harness behavior of /clear and /new (does a new log file with a new ID appear?) is unverified and is spike step 1.

## Code facts checked

- `internal/sessionhost/conversations_windows.go`: `Conversations` builds the list from the usage ledger plus the runtime `HarnessSessionID`; `ResumeConversation` refuses a running session and only accepts IDs from that list.
- `internal/sessionhost/usage_windows.go`: ledger ingest reads only the runtime ID's log files, so unseen IDs never enter the ledger.
- `internal/harness/adapter.go`: `DiscoversSessionID()` marks harnesses that pick their own ID.
- UI: `frontend/src/harnessActions.tsx` (toolbar; Clear removed), `frontend/src/conversationsDrawer.tsx`, `frontend/src/terminalPane.tsx` (side rail, `onResumeConversation`), `frontend/src/main.tsx` (launch "fresh").

## Steps

1. Spike, per harness (Claude Code, Copilot CLI, GSD, Codex, OpenCode; WSL only for Claude and Copilot): in a disposable folder, run /clear or /new and record whether a new conversation ID appears in the harness logs and how to find the newest one for a folder. Output: a table in this doc. Harnesses without a reliable signal are marked button-only.
2. Backend detection: add a per-adapter "latest conversation in folder" probe (capability-gated). On stop, and on a throttled poll while running, if the newest ID differs from the runtime ID, record it as the session's current conversation and add it to the ledger so History lists it. Never overwrite the ID on an ambiguous result (two sessions in one folder): leave unchanged and note it.
3. Backend resume of a running session: make `ResumeConversation` stop gracefully first (existing interrupt, short wait, forced termination, visible), then switch the ID and restore. Keep the revert-on-failure behavior. Outgoing conversation usage is ingested before the ID changes (already done).
4. Resume rule: a plain Resume restores the runtime ID, which step 2 keeps pointed at the latest active conversation.
5. Frontend: add a "New conversation" toolbar button with a confirm dialog (running: stop and relaunch; stopped: start fresh). Enable History's Resume on a running session with a confirm dialog warning the session will be closed. Move the History button next to New conversation in the side rail. Use theme tokens, ASCII text, an existing or new icon in both themes.
6. Tests: Go tests for detection (new ID appears, ambiguous case, throttle), running-resume (stop, switch, revert on failure), list ordering keeps the outgoing conversation. Frontend tests for the dialogs and the drawer enabled state. Run `gofmt -l cmd internal`, `go test ./...`, `npx tsc --noEmit`, `npm test`, `npm run check:tokens`.
7. Docs: `03-session-lifecycle`/`04-session-restoration`, `21-engineering-handoff`, README feature list; state which harnesses detect in-harness changes and which are button-only.
8. Live check: disposable session per installed harness (new conversation, switch while running, return to the earlier one, plain Resume after /clear). Report anything not verified.

## Done criteria

- New conversation appears in History for every harness, started from the button.
- After /clear or /new inside the harness, History lists the new conversation and plain Resume continues it, on harnesses the spike marked as detectable; the rest are documented as button-only.
- Resume works on a running session after a confirm dialog that warns it will be closed; the outgoing conversation remains listed and can be resumed again.
- History and New conversation sit together in the side rail.
- All checks in step 6 pass; the live check results (or gaps) are reported.

## Risks

- In-harness /clear detection depends on each harness's log layout and may differ by version; probes are capability-gated and never guess.
- Two sessions sharing one folder make "newest conversation" ambiguous.

## Open questions

- None blocking. Next: say "go" to start with step 1.
