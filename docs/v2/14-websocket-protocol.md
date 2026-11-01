# WebSocket Protocol

**Status:** Not implemented; future scope only. Phase 1 is a local Windows
desktop application with no WebSocket listener or remote clients.

The desktop UI receives local Wails runtime events. The current event contract
is described in [`13-api-specification.md`](13-api-specification.md); this
document does not define a deployed protocol. Authentication, authorization,
message schemas, reconnect behavior, and network exposure require a separate
design before any WebSocket implementation.
