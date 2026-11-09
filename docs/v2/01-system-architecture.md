# System Architecture

## Phase 1 local desktop architecture

```text
Windows desktop host (Go + WebView2)
  -> local UI (React)
  -> narrow validated UI bridge
  -> session manager and built-in harness adapters
  -> ConPTY -> PowerShell 7 -> AI harness CLI        (default runtime)
  -> ConPTY -> wsl.exe --exec -> AI harness CLI      (experimental WSL runtime)
```

The desktop UI is the sole full desktop frontend; the Bubble Tea CLI/TUI has
been removed. `list`, `start`, `restore`, and `ctl` remain available as
standalone CLI operations. The Windows host uses Wails v2 with bundled React
and xterm.js assets. The UI host, session manager, and harness adapters are distinct: the
renderer does not own process lifecycle or construct provider-specific
commands. The desktop renderer calls local typed Wails bindings and subscribes
to Wails runtime events. Separately, an experimental, off-by-default phone
server exposes an HTTPS API on one configured local address; it is not a
WebSocket listener or a general server mode. See the
[API overview](13-api-specification.md) and
[phone security model](11-security-model.md#phone-server-experimental-off-by-default).
The Windows 10 production terminal path has passed the recorded fixture and
hosted-harness checks, subject to the limits in the
[backlog](20-mvp-backlog.md#open-phase-1-backlog).

Each live session has an independent PowerShell (or WSL) ConPTY process tree. xterm.js
is mounted per open tab and receives ConPTY output as base64-encoded bytes;
input (including binary terminal input) and measured resize are sent only to
the session represented by that terminal. The app stores open tab IDs and
selection in WebView local storage. The desktop opens Home by default; opt-in
per-session/global settings can restore eligible sessions on startup. Live sessions
continue running when their tab is not selected. A running session always has a tab and cannot be closed without being stopped. The xterm instance stays mounted so it continues consuming output and
can reopen the same screen. A browser-only mocked-bridge smoke exercised
background output, split UTF-8/emoji, ANSI, alternate screen, terminal queries,
mouse reporting, resize, and input. That browser-only check does not exercise
the production path. A separate production-window fixture verified rendering,
typed input, resize, cursor and device-status reports, and bundled-ConPTY
mouse reports, with the backend-specific limits documented in the
[ConPTY notes](02-conpty-integration.md#repeatable-hosted-terminal-fixture).
Output delivery under sustained load still requires end-to-end testing.

The desktop app checks the Evergreen WebView2 Runtime before creating its
window and directs the user to Microsoft's official installer when it is
absent. Home detects PowerShell 7, WSL and its distros, supported harnesses, Node.js/npm and Git,
including versions and declared capability probes. Missing PowerShell has a
confirmed setup flow, which is skipped when WSL has a usable distro; optional Node.js and Git have confirmed per-user
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
- **Runtime (`internal/runtime`):** the per-session seam that answers "where does
  this session live". `powershellRuntime` is the default; `WSL` launches
  `wsl.exe -d <distro> --cd <dir> --exec ...` with no shell, forwards the session
  environment through `WSLENV`, tags the launch with `TAUSHELL_LAUNCH` and cleans
  up leftover Linux processes after exit, because the Windows job object only owns
  `wsl.exe`. It also provides WSL readiness (`Detect`), distro inventory, path
  mapping, install helpers and the update check. Harness state readers take a
  per-session home (`harness.*In`), git runs inside the distro, and the files
  drawer reads the distro over `\\wsl.localhost`. Never start a stopped distro
  just to read state. Design, status and backlog: [`docs/wsl/`](../wsl/).
- **Embedded terminal:** VT screen interpretation and display; not a chat
  transcript renderer or agent-message parser.

Local taskbar-flash and Windows-toast attention notifications are in Phase 1;
see [`22-local-attention-notifications.md`](22-local-attention-notifications.md).
General remote access, mobile UI, web dashboards/server mode, general
third-party plugins, and distributed execution are outside Phase 1. The
experimental opt-in phone server and Android companion already exist; their
existence does not expand that scope.
