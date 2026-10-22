# MVP Backlog

## Phase 1: local Windows desktop GUI (TUI removed)

- Completed migration: the Bubble Tea TUI and its TUI-only dependencies were removed. `list`, `start <session-id>`, and explicit `restore <session-id>` remain available as CLI operations; no TUI fallback is planned.
- In progress: a Go/Wails WebView2 host, bundled React/xterm.js launcher, local tabs, session creation, tool inventory, session lifecycle bridge, and shutdown confirmation are implemented. The host/window/terminal and both supported harnesses have not passed the end-to-end Windows 10 feasibility gate. See [`docs/v1/08-UI-Design.md`](../v1/08-UI-Design.md) and [`docs/v1/09-UI-Implementation-Plan.md`](../v1/09-UI-Implementation-Plan.md).
- Security hardening: the Windows host now cancels top-level WebView2 navigations outside the bundled `http://wails.localhost` origin before they load. Wails' bound-method origin validation remains a separate defense.
- Session-manager lifecycle coverage is implemented for concurrent independent fake sessions, input/output/resize, process exit, failed restore preserving the resume ID, graceful and forced stop, and cleanup. This does not cover a real hosted xterm or harness.
- Implemented basic executable and version detection for PowerShell 7, Claude Code, and Copilot CLI; Home shows the resolved executable path so PATH precedence is visible. Planned: minimum/tested ranges, required capability probes, out-of-range warnings, and confirmed setup flows. Do not silently pin or downgrade.
- Implemented schema: strict YAML accepts ordered harness `launch_args`; managed session/resume arguments cannot be overridden. The UI currently accepts additional ordered arguments while creating a session; generated per-harness templates and a full editable configuration preview remain outstanding.
- Planned setup: WebView2 and PowerShell 7 are TauShell host prerequisites; missing harnesses do not block Home. winget is optional; installation requires confirmation and falls back to official setup links.
- ConPTY host for PowerShell 7 on Windows 10, with automated Windows tests for output, exit codes, resizing, and child-process cleanup.
- Implemented per-run JSON diagnostics at `%TEMP%\TauShell\taushell.log`; startup truncates the file and the UI exposes its location. Host/lifecycle, Wails, frontend runtime, React render, and operation failures are logged with sensitive payload boundaries. Prompt/terminal contents, credentials, and environment values are intentionally excluded.
- Session registry and human-edited, per-session YAML definitions; keep mutable runtime state separate.
- Start, interact with, stop, and explicitly restore TauShell-launched sessions.
- Harness-specific launch/resume command construction for Claude Code and GitHub Copilot CLI uses explicit session IDs. A disposable Copilot CLI 1.0.100-1 provider-backed test confirmed conversation recovery via TauShell `restore`; Claude Code provider-backed recovery remains unverified.
- Confirm before application shutdown when sessions are running; keep the Wails close callback nonblocking while confirmation and stopping run asynchronously. Attempt graceful interruption, then force-terminate any remaining processes in TauShell's job after a brief wait. A production-window smoke with an inert fixture verified that No leaves the app and session running and Yes closes the app after the fixture process exits; the fixture's PowerShell process exited with code 1, so graceful-stop success and forced-termination reporting still need separate verification.
- Automated session-manager tests cover concurrent session I/O/resize, process exit, failed restore, graceful/forced stop, and inherited child cleanup. Shutdown tests also cover supervisor completion, idempotent close, rejecting starts after shutdown, retry after stop timeout or database-close failure, and keeping the window open when session checks or forced-stop reporting fail. Detached shell/broker launches such as PowerShell `Start-Process` can escape the ConPTY job; by design, those processes are unmanaged and are not subject to ancestry-based termination.
- Windows 10 terminal compatibility: CLI and hosted sessions temporarily use UTF-8 console code pages and restore prior settings; session hosting also enables/restores virtual-terminal output mode. ConPTY tests cover supplementary-plane emoji input, Unicode/emoji output bytes, ANSI output, resizing, and process cleanup. The desktop embedded-terminal path needs separate end-to-end verification; TauShell must not rewrite arbitrary harness output.

### Open Phase 1 backlog

- **Feasibility gate:** a checked-in deterministic fixture has verified
  Unicode/emoji, ANSI colors, cursor movement/erase, cursor-position query
  replies, split UTF-8 output, typed input, explicit exit, and live resize
  through the production WebView2, xterm.js, PowerShell 7, and ConPTY path.
  The fixture's explicit exit restored the preserved primary-screen marker.
  A post-start device-status response is now confirmed at the Go bridge and
  successfully written to ConPTY, but no corresponding input chunk reaches
  the fixture. A post-start cursor query produces a fixture-visible
  seven-byte response, although the bridge diagnostic did not identify its
  source. Payload-free production diagnostics confirm native pointer events
  reach the terminal container, but xterm's effective mouse-tracking mode
  remains `none`. A direct ConPTY probe showed Windows ConPTY consumes
  `CSI ? 1000 h` and `CSI ? 1006 h` before xterm receives them; directly
  enabling tracking in xterm caused SGR mouse input to reach the Go bridge.
  Do not force tracking unconditionally because it changes normal terminal
  selection/input behavior. **Decided next spike:** evaluate a bundled newer
  ConPTY (`conpty.dll` + `OpenConsole.exe`) against inbox ConPTY for mouse-mode
  passthrough, device-status/cursor-report delivery (hypothesis: inbox ConPTY
  answers these itself), and the per-cycle console-host handle; adopt only if
  mouse modes pass through. Then verify these behaviors and conduct
  user-authorized non-destructive Claude Code and Copilot CLI hosted-terminal
  walkthroughs using the user's existing sign-ins.
- **Host prerequisite:** registry-detection unit tests cover Phase 1; the
  missing-WebView2 native prompt and official download handoff are deferred to
  a manual Windows Sandbox/VM check (the current machine has WebView2).
- **Tool discovery/setup** and **session authoring** proceed in parallel with
  the feasibility gate (decided); they do not depend on the terminal path.
- **Tool discovery/setup:** declare minimum and tested version ranges, probe
  required CLI capabilities, add confirmed winget installs, and provide
  official vendor-link fallbacks.
- **Session authoring:** generate harness-specific editable YAML defaults,
  preview and validate effective discrete argv before saving, and offer to
  start after creation.
- **Workspace UX:** verify recent folder suggestions and keyboard-only tab
  navigation/overflow controls in the desktop host, persisted-tab restoration
  across app restarts, and background terminal behavior through the actual
  Wails/ConPTY path. A mocked-browser smoke verified Home/End selection,
  arrow-key wraparound, focus movement, and synchronized selected-tab state;
  a production-host smoke found and fixed the missing tab-strip flex layout,
  then verified Home/Right focus and selection switching between Home and one
  stopped fixture tab. With eight inert fixture tabs open, native left/right
  controls scrolled through the overflow. A mouse click seeded focus, so a
  complete keyboard-only walkthrough remains unverified. Basic accessible tab
  navigation and horizontal overflow controls are implemented in the frontend.
  The session form suggests up to five recently used folders, persisted in
  WebView local storage.
- **Shutdown/recovery:** a repeated native test verifies process, job, and
  input/output pipe handles are invalidated after ConPTY close; the opaque
  pseudoconsole is checked through successful close and cleared state. Wails-
  host close integration tests remain open. The comprehensive handle-leak
  item stays open: the residual per-cycle handle to the exited console host
  is tracked in the bundled-ConPTY spike. Broaden stale-runtime reconciliation beyond the tested
  abandoned-starting, recent-starting, missing/exited-PID, and
  live-other-window cases.
- **Harness compatibility:** complete provider-backed Claude Code and Copilot
  CLI hosted-terminal walkthroughs; the current Copilot restore evidence is
  CLI/provider-level only, not hosted xterm coverage.

The detailed step dependencies and acceptance criteria remain in
[`docs/v1/09-UI-Implementation-Plan.md`](../v1/09-UI-Implementation-Plan.md).

## Later phases

- Workspaces and group restore.
- Notifications, mobile dashboard, remote interaction, and remote/web dashboard.
- General plugin SDK, rich attention detection, multi-agent orchestration, distributed execution, and cloud sync.
