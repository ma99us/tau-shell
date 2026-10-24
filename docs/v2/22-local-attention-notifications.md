# Local Attention Notifications (Later Phase Pre-Plan)

Status: **not scheduled, not implemented.** Phase 1 excludes notifications (see
`AGENTS.md`, `docs/v2/16-roadmap.md`). This document pre-plans a small local
(desktop-only) slice so a later agent can pick it up. It narrows
[`07-notification-engine.md`](./07-notification-engine.md) and
[`08-attention-detection.md`](./08-attention-detection.md) to what needs no
remote service. Remote push (ntfy, Pushover, Web Push) stays out of scope here.

## Problem

When a harness finishes working and waits for the user, the only cue is its
terminal tab. A user working in another window or another TauShell tab gets no
signal. Session status changes only on process start, exit, or failure.

## Goal

Tell the user, locally and unobtrusively, that a **background or unfocused**
session likely needs input or has ended, without reading or rewriting harness
output.

## Proposed behavior (in order of increasing cost)

1. **Tab marker (frontend only).** A dot/ASCII marker on a session tab and its
   sidebar row when that session is not the active, focused tab and has become
   quiet after output. Clears when the tab is selected or the user types in it.
2. **Taskbar flash.** When TauShell is not the foreground window and a session
   is flagged, flash the taskbar button (Win32 `FlashWindowEx` with
   `FLASHW_TRAY`, flash until foreground). Stop when the window is activated.
3. **Windows toast (optional, last).** Only if 1-2 prove insufficient. Needs an
   app identity/AUMID decision and likely a packaging change; do not start here.
   No tray icon: the UI design excludes a tray-first model.

Keep TauShell-owned text ASCII. Never put terminal contents, prompts, paths, or
environment values in a notification or log.

## Attention signal (Phase-appropriate, heuristic)

Use only signals TauShell already has, per `08-attention-detection.md`:

- **Output silence:** no PTY output for N seconds after at least some output
  since the last user input. Suggested default N = 3 s, debounced; make it a
  named constant first, config later.
- **Process state:** exit/failure events already exist; map them to a distinct
  "ended" marker rather than "needs input".
- Not in the first slice: stable-screen-buffer analysis, cursor location,
  OSC sequences (OSC 9/777 notifications and OSC 133 prompt marks could be an
  adapter-capability-gated follow-up), plugin-specific patterns.

Known weakness: silence is not proof of "waiting for user" (a harness can
pause during a long tool call). Label markers neutrally (for example
"quiet"), not "needs your input", and do not claim detection accuracy.

## Suggested implementation map (verify against current code first)

- Output already reaches the UI as base64 `session-event` events with
  `kind: "output"` (see the event handler in `frontend/src/main.tsx` and
  `Event`/`publish` in `internal/sessionhost/manager_windows.go`). A per-session
  last-output / last-input timestamp can live in the frontend (marker) and, for
  flashing, in the Go manager so it works without UI timers.
- Add a host-side method (Windows-only file, `_windows.go`) that calls
  `FlashWindowEx` on the main window handle; expose it through a bound method
  or call it directly from the manager callback. Window focus state: use
  Wails window events or `GetForegroundWindow`; do not flash when focused.
- Keep the attention logic tool-independent in core; harness-specific cues
  (if any) belong behind adapters.

## Settings and safety

- Off-by-default is acceptable; if on by default, provide a visible toggle.
  A per-session mute is a later nicety.
- Rate-limit: at most one flash per session per quiet period; no repeats until
  new output followed by new silence.
- No sound in the first slice.

## Acceptance criteria

- With the terminal-fixture standing in for a harness, a background tab gets a
  marker after output + silence, and the marker clears on selection or typing.
- With TauShell minimized/unfocused, the taskbar button flashes once per
  attention event and stops on activation.
- Focused active tab never flashes or marks.
- Unit tests for the silence/debounce state machine (inject a fake clock) and a
  manual production-window smoke documented in the engineering handoff.
- No terminal contents appear in logs or notifications.

## Open questions for the owner before work starts

1. Is output silence acceptable as the first signal, and is 3 s the right
   default?
2. Flash only, or also a toast? (Toast has packaging/AUMID cost.)
3. On by default or opt-in?
4. Should "session ended/failed" also flash?

## Scope guard

Pull this into a phase only by explicit decision; update `AGENTS.md`,
`README.md`, and `16-roadmap.md` at that time, since they currently state
notifications are excluded.
