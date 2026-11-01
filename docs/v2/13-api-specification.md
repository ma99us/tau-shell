# Desktop API Specification

## Phase 1 transport

Phase 1 exposes no REST server or network API. The Windows desktop uses local
Wails method bindings implemented by `cmd/taushell/App`; the typed frontend
contract is `frontend/src/bridge.ts`. Bound operations cover session
configuration and lifecycle, tools/installations, skills, presets, usage,
updates, dialogs, and native file/window actions. Validation and side effects
remain in the Go host; the renderer does not construct provider commands or
manage processes.

The UI subscribes to Wails runtime events, not WebSockets. `session-event`
carries state snapshots and base64-encoded terminal output from
`internal/sessionhost.Event`. Host-only events also request dialogs, focus a
session, report settings errors, and publish shutdown progress. Terminal bytes
are sent only on that session output event and are not written to diagnostics;
prompt text and environment values are not logged.

Future REST/WebSocket access is outside Phase 1 and has no current endpoint,
authentication, or wire-format contract. See
[`14-websocket-protocol.md`](14-websocket-protocol.md) for its status.
