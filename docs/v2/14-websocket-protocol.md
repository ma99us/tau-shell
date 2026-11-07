# WebSocket Protocol

**Status:** A WebSocket protocol and listener are not implemented; they remain
future scope. The local Windows desktop uses Wails bindings and events.

The desktop UI receives local Wails runtime events. The current event contract
is described in [`13-api-specification.md`](13-api-specification.md); this
document does not define a deployed WebSocket protocol. Authentication,
authorization, message schemas, reconnect behavior, and network exposure
require a separate design before any WebSocket implementation.

The experimental, off-by-default phone server is a separate HTTPS API with
paired Android clients, not a WebSocket implementation. Its current boundaries
are described in the [API overview](13-api-specification.md) and
[phone security model](11-security-model.md#phone-server-experimental-off-by-default).
