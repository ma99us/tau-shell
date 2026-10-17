# System Architecture

## Phase 1 local desktop architecture

```text
Windows desktop host (Go + WebView2)
  -> local UI (React candidate)
  -> narrow validated UI bridge
  -> session manager and built-in harness adapters
  -> ConPTY -> PowerShell 7 -> AI harness CLI
```

The desktop UI is the sole interactive experience; the Bubble Tea CLI/TUI has
been removed. `list`, `start`, and `restore` remain available as standalone
CLI operations. The current Windows host uses Wails v2 with bundled React and
xterm.js assets. The UI host, session manager, and harness adapters are
distinct: the renderer does not own process lifecycle or construct
provider-specific commands. The host and terminal stack are implemented but
remain provisional until the Windows 10 feasibility gate passes with both
supported harnesses.

Each live session has an independent PowerShell/ConPTY process tree. xterm.js
is mounted per open tab and receives ConPTY output as base64-encoded bytes;
input (including binary terminal input) and measured resize are sent only to
the session represented by that terminal. The app stores open tab IDs and
selection in WebView local storage; it does not start sessions automatically
on startup. Full-screen harness behavior, terminal query/input modes, mouse
support, resize fidelity, and output delivery under load still require
end-to-end testing.

The desktop app checks the Evergreen WebView2 Runtime before creating its
window and can direct the user to Microsoft's download page when the runtime
is absent. Home displays detected PowerShell 7 and harness executable/version
information. Automated setup, version support ranges, capability probes, and
confirmed installation flows are not implemented yet.

## Boundaries

- **Desktop host:** Wails window lifecycle, embedded local web assets, runtime
  prerequisite check, and a small typed set of bound UI operations. Explicit
  external-navigation restrictions and bridge behavior across navigation
  origins have not yet been verified.
- **Session manager:** session configuration, start/restore/stop, process state,
  runtime persistence, and shutdown.
- **Harness adapters:** executable/version detection, capability probes, and
  harness-specific launch/resume arguments.
- **ConPTY/PowerShell:** ordinary interactive PowerShell 7 hosting, UTF-8
  bootstrap, input/output, resizing, and process-tree cleanup.
- **Embedded terminal:** VT screen interpretation and display; not a chat
  transcript renderer or agent-message parser.

Remote access, mobile UI, web dashboards/server mode, notifications, general
third-party plugins, and distributed execution are outside Phase 1.
