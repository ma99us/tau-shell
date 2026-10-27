# Local Attention Notifications

Status: **implemented in Phase 1** (owner decision; scope pulled in from the
earlier pre-plan). Covered by Go unit tests (decision logic, settings, body
text) and a type-checked build. The owner confirmed on the dev machine that the
background notification works; the click-to-open path and the taskbar flash
alone were not separately checked, and automation could not drive the window.
Remote push (ntfy, Pushover, Web Push) remains out of scope. See also
[`07-notification-engine.md`](./07-notification-engine.md) and
[`08-attention-detection.md`](./08-attention-detection.md).

## Behavior

- Triggers (frontend edge detector in `frontend/src/attention.ts`, fed by the
  harness cues already used for the state icons; no PTY-silence heuristic):
  1. the harness starts **waiting for an answer** (`input`);
  2. the harness **finishes** (working to not-working, after at least 3 s of
     work, and not into an input prompt).
  Process exit/failure is not a trigger. Each transition fires once.
- The UI calls `NotifyAttention(sessionId, kind)`. Go skips everything while
  TauShell is the foreground window, otherwise flashes the taskbar button
  (`FlashWindowEx`, `FLASHW_TRAY|FLASHW_TIMERNOFG`, stops on activation) and/or
  sends a Windows toast, per settings. In-app markers cover the foreground case.
- Toast text is ASCII with the session name only. Clicking a toast raises
  TauShell and opens that session's tab (`focus-session` event).
- Settings: `~/.tau_shell/config.yaml`, `notifications.toast` and
  `notifications.taskbar_flash`, both on by default; file only, no settings UI
  ("Edit settings..." in the command palette opens the file). See
  [`18-yaml-schema.md`](./18-yaml-schema.md).
- Security notes (HKCU toast registration, no content in alerts):
  [`11-security-model.md`](./11-security-model.md).

## Decisions (answers to the original open questions)

1. First signal: harness cues (`input` / working-to-idle), not output silence;
   3 s minimum work for "finished".
2. Flash **and** toast.
3. On by default; disable in the settings file.
4. Session ended/failed does not alert.

## Known limits

- Toasts can be suppressed by Focus Assist or per-app Windows settings.
- Claude Code cues are unverified past the title spinner (provider 401), so
  its alerts are unverified end to end.
- No sound, no per-session mute, no tray icon.
- Needs a live smoke test: background TauShell, let a Copilot session finish,
  confirm the flash and the toast, click the toast, confirm the tab opens.
