# System Architecture

## Phase 1 local desktop architecture

```text
Windows desktop host (Go + WebView2)
  -> local UI (React)
  -> narrow validated UI bridge
  -> session manager and built-in harness adapters
  -> ConPTY -> PowerShell 7 -> AI harness CLI
```

The desktop UI is the sole interactive experience; the Bubble Tea CLI/TUI has
been removed. `list`, `start`, and `restore` remain available as standalone
CLI operations. The current Windows host uses Wails v2 with bundled React and
xterm.js assets. The UI host, session manager, and harness adapters are
distinct: the renderer does not own process lifecycle or construct
provider-specific commands. The renderer calls local typed Wails bindings and
subscribes to Wails runtime events; Phase 1 has no REST server or WebSocket
listener. The host and terminal stack remain provisional until the Windows 10
feasibility gate passes with supported harnesses.

Each live session has an independent PowerShell/ConPTY process tree. xterm.js
is mounted per open tab and receives ConPTY output as base64-encoded bytes;
input (including binary terminal input) and measured resize are sent only to
the session represented by that terminal. The app stores open tab IDs and
selection in WebView local storage. The desktop opens Home by default; opt-in
per-session/global settings can restore eligible sessions on startup. Live sessions
continue running when their tab is not selected. A running session always has a tab and cannot be closed without being stopped. The xterm instance stays mounted so it continues consuming output and
can reopen the same screen. A browser-only mocked-bridge smoke exercised
background output, split UTF-8/emoji, ANSI, alternate screen, terminal queries,
mouse reporting, resize, and input. It does not verify those behaviors through
the production Wails/WebView2/ConPTY path, and output delivery under load still
requires end-to-end testing.

The desktop app checks the Evergreen WebView2 Runtime before creating its
window and directs the user to Microsoft's official installer when it is
absent. Home detects PowerShell 7, supported harnesses, Node.js/npm and Git,
including versions and declared capability probes. Missing PowerShell has a
confirmed setup flow; optional Node.js and Git have confirmed per-user
installers and never gate Home. Harnesses with winget or npm install routes
also require confirmation and are re-detected afterwards.

## Boundaries

- **Desktop host:** Wails window lifecycle, embedded local web assets, runtime
  prerequisite check, and a small typed set of bound UI operations. The
  top-level external-navigation restriction has policy tests and a temporary
  production WebView2 cancellation smoke; cross-origin bridge denial has not
  been separately exercised.
- **Session manager:** session configuration, start/restore/stop, process state,
  runtime persistence, and shutdown.
- **Harness adapters:** executable/version detection, capability probes, and
  harness-specific launch/resume arguments.
- **ConPTY/PowerShell:** ordinary interactive PowerShell 7 hosting, UTF-8
  bootstrap, input/output, resizing, and process-tree cleanup.
- **Embedded terminal:** VT screen interpretation and display; not a chat
  transcript renderer or agent-message parser.

Local taskbar-flash and Windows-toast attention notifications are in Phase 1;
see [`22-local-attention-notifications.md`](22-local-attention-notifications.md).
Remote access, mobile UI, web dashboards/server mode, general third-party
plugins, and distributed execution are outside Phase 1.
