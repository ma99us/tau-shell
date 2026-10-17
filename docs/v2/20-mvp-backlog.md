# MVP Backlog

## Phase 1: local Windows desktop GUI (TUI removed)

- Completed migration: the Bubble Tea TUI and its TUI-only dependencies were removed. `list`, `start <session-id>`, and explicit `restore <session-id>` remain available as CLI operations; no TUI fallback is planned.
- In progress: a Go/Wails WebView2 host, bundled React/xterm.js launcher, local tabs, session creation, tool inventory, session lifecycle bridge, and shutdown confirmation are implemented. The host/window/terminal and both supported harnesses have not passed the end-to-end Windows 10 feasibility gate. See [`docs/v1/08-UI-Design.md`](../v1/08-UI-Design.md) and [`docs/v1/09-UI-Implementation-Plan.md`](../v1/09-UI-Implementation-Plan.md).
- Planned: detect supported harness versions and required capabilities; use tested ranges, warn outside them, and avoid silent pinning or downgrades.
- Implemented schema: strict YAML accepts ordered harness `launch_args`; managed session/resume arguments cannot be overridden. The UI currently accepts additional ordered arguments while creating a session; generated per-harness templates and a full editable configuration preview remain outstanding.
- Planned setup: WebView2 and PowerShell 7 are TauShell host prerequisites; missing harnesses do not block Home. winget is optional; installation requires confirmation and falls back to official setup links.
- ConPTY host for PowerShell 7 on Windows 10, with automated Windows tests for output, exit codes, resizing, and child-process cleanup.
- Session registry and human-edited, per-session YAML definitions; keep mutable runtime state separate.
- Start, interact with, stop, and explicitly restore TauShell-launched sessions.
- Harness-specific launch/resume command construction for Claude Code and GitHub Copilot CLI uses explicit session IDs. A disposable Copilot CLI 1.0.100-1 provider-backed test confirmed conversation recovery via TauShell `restore`; Claude Code provider-backed recovery remains unverified.
- Confirm before application shutdown when sessions are running; attempt graceful interruption, then force-terminate remaining processes after a brief wait.
- Windows 10 terminal compatibility: CLI and hosted sessions temporarily use UTF-8 console code pages and restore prior settings; session hosting also enables/restores virtual-terminal output mode. ConPTY tests cover supplementary-plane emoji input, Unicode/emoji output bytes, ANSI output, resizing, and process cleanup. The desktop embedded-terminal path needs separate end-to-end verification; TauShell must not rewrite arbitrary harness output.

## Later phases

- Workspaces and group restore.
- Notifications, mobile dashboard, remote interaction, and remote/web dashboard.
- General plugin SDK, rich attention detection, multi-agent orchestration, distributed execution, and cloud sync.
