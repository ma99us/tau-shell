# Desktop API Specification

## Phase 1 transport

The Windows desktop uses local Wails method bindings implemented by
`cmd/taushell/App`; the typed frontend contract is `frontend/src/bridge.ts`.
Bound operations cover session configuration and lifecycle, tools/installations,
skills, presets, usage, updates, dialogs, and native file/window actions.
Validation and side effects remain in the Go host; the renderer does not
construct provider commands or manage processes.

The UI subscribes to Wails runtime events, not WebSockets. `session-event`
carries state snapshots and base64-encoded terminal output from
`internal/sessionhost.Event`. Host-only events also request dialogs, focus a
session, report settings errors, and publish shutdown progress. Terminal bytes
are sent only on that session output event and are not written to diagnostics;
prompt text and environment values are not logged.

Separately, the experimental phone server is off by default and listens only
when enabled on a configured local address. It uses TLS 1.3, a pairing channel,
and per-device bearer authentication. Its current HTTPS routes include
`POST /v1/pair`, `GET /v1/sessions`, session `wait`, `screen`, and `presets`
reads, session `send`, `start`, and `answer` writes, and `GET /v1/presets`.
See the [phone security model](11-security-model.md#phone-server-experimental-off-by-default)
and [Android guide](../../android/README.md) for limits and operation.

This experimental phone API is not a general desktop REST/server mode.
WebSocket access remains future scope; see
[`14-websocket-protocol.md`](14-websocket-protocol.md).
