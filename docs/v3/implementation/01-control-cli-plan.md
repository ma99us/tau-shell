# Implementation plan: Control CLI and local API (feature 01)

Spec: [../features/01-control-cli-and-api.md](../features/01-control-cli-and-api.md).
Status: work package 1 shipped; the rest wait on feature 02.

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
| 5 | `send`: refused unless the session is confirmed idle; `--insert-only` types without submitting; audited by source and session, never by text | Blocked on feature 02 |
| 6 | `wait --for idle --timeout`: needs a confirmed idle state | Blocked on feature 02 |
| 7 | Agent access: per-session token injected into the session environment, gated by `control.agents`, plus an agent skill | Open |

## Verification

- Package 1: unit tests cover the DACL read-back, the first-instance refusal, a pipe round trip, the audit bound, newest-first discovery, the CLI table and JSON output, and exit codes. A real-app smoke test confirmed `ctl list` works with the channel on, exits 4 when off and 3 when not running, and leaves no pipe after the app stops.
- Later packages: add a test that a refused `send` never reaches the session, and that audit entries carry no text.

## Exit codes

0 ok, 1 error, 2 usage, 3 TauShell not running, 4 channel disabled.
