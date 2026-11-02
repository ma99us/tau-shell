# Implementation plan: Control CLI and local API (feature 01)

Spec: [../features/01-control-cli-and-api.md](../features/01-control-cli-and-api.md).
Status: work packages 1, 2, 5 and 6 shipped (list, wait, guarded send, window-reported state). Start/restore/stop, read and agent access are open.

## Decisions

- Existing sessions only. Create and delete stay in the app.
- One pipe per instance (`taushell-ctl-<user-tag>-<pid>`); the CLI defaults to the newest, `--instance <pid>` selects.
- `read` returns current screen text only.
- Opt-in: `control.enabled` (default false). `control.agents` (default false) is reserved for agent access.

## Principles

- Same-user only: protected DACL, first-instance flag, remote clients rejected. The caller PID is a hint.
- No prompt text or terminal content in logs or in the audit trail.
- Windows pieces stay in `_windows.go` files; the protocol and handler interface are portable.
- Failures are visible: a failed pipe start is shown in the UI; the CLI explains disabled vs not running.

## Work packages

| # | Package | Status |
|---|---|---|
| 1 | `internal/control`: protocol, pipe server, per-instance discovery, audit log; `hello` and `list`; `control` settings; `taushell ctl list [--json] [--instance]`; audit exposed through the bound `GetControlStatus` and the `control-audit` event | Done |
| 2 | UI: Home section "Control channel: on" with the live audit list (hidden while the channel is off) | Done |
| 3 | `start`, `restore`, `stop` on existing sessions through the same manager methods the UI uses | Open |
| 4 | `read`: rendered screen rows from the terminal model | Open (needs a Go-side screen model or a UI round trip) |
| 5 | `send`: refused unless the session state is a confirmed harness cue (idle with no unsent text, or any non-waiting state with `--insert-only`); never interrupts a working turn, never types into a prompt; `--stdin` keeps text off the command line; audited by session and outcome, never by text. The window re-checks the live screen before typing; a refusal there shows in the UI but is not returned to the CLI (the CLI prints "handed to the TauShell window"). Exit 5 = refused | Done |
| 6 | `wait <session> --for idle\|input\|working --timeout`: server holds each request up to 8 s, the CLI repeats; matches only a confirmed state. Exit 6 = timeout | Done |
| 7 | Agent access: per-session token injected into the session environment, gated by `control.agents`, plus an agent skill | Open |

## Verification

- Package 1: unit tests cover the DACL read-back, the first-instance refusal, a pipe round trip, the audit bound, newest-first discovery, the CLI table and JSON output, and exit codes. A real-app smoke test confirmed `ctl list` works with the channel on, exits 4 when off and 3 when not running, and leaves no pipe after the app stops.
- Packages 5 and 6: unit tests cover the refusal matrix (not running, never reported, heuristic, waiting, working, just-idle, draft), delivery only on confirmed state, wait matching only confirmed states, and CLI exit codes. A real-app run confirmed the refusals for a stopped and an unknown session. A live send into a running Claude Code or Copilot CLI session has not been run yet.
- The audit trail records the session id and outcome only; the text never reaches the audit code (only `session` is read from the arguments).

## Exit codes

0 ok, 1 error, 2 usage, 3 TauShell not running or session not found, 4 channel disabled, 5 refused (state does not allow it), 6 wait timed out.

## Reliable state (feature 02, first slice)

The window reports each running session's state to Go about twice a second (`ReportSessionState`), tagged `cue` (title spinner, status row, prompt footer of Claude Code and Copilot CLI) or `heuristic` (output timing). `internal/agentstate` marks a state confirmed only when it is a fresh cue; idle must also hold 1.5 s. Reports older than 4 s become `unknown`, so a closed or hidden window never looks idle. Tools without cues (GSD, OpenCode, Codex) stay heuristic, so `send` and `wait` refuse them. Still open for feature 02: harness hooks and plugins, the inbox, tray, sound and per-session mute.
