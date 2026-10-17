# MVP Backlog

## Phase 1: local Windows desktop GUI (TUI removed)

- Completed migration: the Bubble Tea TUI and its TUI-only dependencies were removed. `list`, `start <session-id>`, and explicit `restore <session-id>` remain available as CLI operations; no TUI fallback is planned.
- In progress: a Go/Wails WebView2 host, bundled React/xterm.js launcher, local tabs, session creation, tool inventory, session lifecycle bridge, and shutdown confirmation are implemented. The host/window/terminal and both supported harnesses have not passed the end-to-end Windows 10 feasibility gate. See [`docs/v1/08-UI-Design.md`](../v1/08-UI-Design.md) and [`docs/v1/09-UI-Implementation-Plan.md`](../v1/09-UI-Implementation-Plan.md).
- Security hardening: the Windows host now cancels top-level WebView2 navigations outside the bundled `http://wails.localhost` origin before they load. Wails' bound-method origin validation remains a separate defense.
- Session-manager lifecycle coverage is implemented for concurrent independent fake sessions, input/output/resize, process exit, failed restore preserving the resume ID, graceful and forced stop, and cleanup. This does not cover a real hosted xterm or harness.
- Planned: detect supported harness versions and required capabilities; use tested ranges, warn outside them, and avoid silent pinning or downgrades.
- Implemented schema: strict YAML accepts ordered harness `launch_args`; managed session/resume arguments cannot be overridden. The UI currently accepts additional ordered arguments while creating a session; generated per-harness templates and a full editable configuration preview remain outstanding.
- Planned setup: WebView2 and PowerShell 7 are TauShell host prerequisites; missing harnesses do not block Home. winget is optional; installation requires confirmation and falls back to official setup links.
- ConPTY host for PowerShell 7 on Windows 10, with automated Windows tests for output, exit codes, resizing, and child-process cleanup.
- Session registry and human-edited, per-session YAML definitions; keep mutable runtime state separate.
- Start, interact with, stop, and explicitly restore TauShell-launched sessions.
- Harness-specific launch/resume command construction for Claude Code and GitHub Copilot CLI uses explicit session IDs. A disposable Copilot CLI 1.0.100-1 provider-backed test confirmed conversation recovery via TauShell `restore`; Claude Code provider-backed recovery remains unverified.
- Confirm before application shutdown when sessions are running; attempt graceful interruption, then force-terminate any remaining processes in TauShell's job after a brief wait.
- Automated session-manager tests cover concurrent session I/O/resize, process exit, failed restore, graceful/forced stop, and inherited child cleanup. Shutdown tests also cover supervisor completion, idempotent close, rejecting starts after shutdown, retry after stop timeout or database-close failure, and keeping the window open when session checks or forced-stop reporting fail. Detached shell/broker launches such as PowerShell `Start-Process` can escape the ConPTY job; by design, those processes are unmanaged and are not subject to ancestry-based termination.
- Windows 10 terminal compatibility: CLI and hosted sessions temporarily use UTF-8 console code pages and restore prior settings; session hosting also enables/restores virtual-terminal output mode. ConPTY tests cover supplementary-plane emoji input, Unicode/emoji output bytes, ANSI output, resizing, and process cleanup. The desktop embedded-terminal path needs separate end-to-end verification; TauShell must not rewrite arbitrary harness output.

### Open Phase 1 backlog

- **Feasibility gate:** validate xterm rendering/input over the actual WebView2
  and ConPTY path, including split UTF-8, ANSI/cursor/alternate-screen,
  terminal queries, resize, and mouse modes. Then do non-destructive
  Claude Code and Copilot CLI hosted-terminal walkthroughs.
- **Host prerequisite:** exercise the missing-WebView2 native prompt and
  official download handoff in an isolated environment; the current machine
  has WebView2 installed.
- **Tool discovery/setup:** declare minimum and tested version ranges, probe
  required CLI capabilities, add confirmed winget installs, and provide
  official vendor-link fallbacks.
- **Session authoring:** generate harness-specific editable YAML defaults,
  preview and validate effective discrete argv before saving, and offer to
  start after creation.
- **Workspace UX:** add recent folders, keyboard tab navigation and overflow
  handling; verify persisted-tab restoration across app restarts and confirm
  background terminal behavior through the actual Wails/ConPTY path.
- **Shutdown/recovery:** add native ConPTY handle-leak checks and UI-level
  shutdown tests; broaden stale-runtime reconciliation coverage beyond the
  tested abandoned-starting, missing-PID, and live-other-window cases.
- **Harness compatibility:** complete provider-backed Claude Code and Copilot
  CLI hosted-terminal walkthroughs; the current Copilot restore evidence is
  CLI/provider-level only, not hosted xterm coverage.

The detailed step dependencies and acceptance criteria remain in
[`docs/v1/09-UI-Implementation-Plan.md`](../v1/09-UI-Implementation-Plan.md).

## Later phases

- Workspaces and group restore.
- Notifications, mobile dashboard, remote interaction, and remote/web dashboard.
- General plugin SDK, rich attention detection, multi-agent orchestration, distributed execution, and cloud sync.
