# MVP Backlog

## Phase 1: local Windows CLI/TUI

- ConPTY host for PowerShell 7 on Windows 10.
- Session registry and human-edited, per-session YAML definitions; keep mutable runtime state separate.
- Start, interact with, stop, and explicitly restore TauShell-launched sessions.
- Harness-specific launch/resume support for Claude Code and GitHub Copilot CLI where supported; report unsupported or failed resume attempts.
- Confirm before application shutdown when sessions are running; attempt graceful interruption, then force-terminate remaining processes after a brief wait.
- Windows 10 terminal compatibility: UTF-8/code-page bootstrap, Unicode and box-drawing behavior, color preservation, and Nerd Font/emoji compatibility checks with a readable fallback.

## Later phases

- Workspaces and group restore.
- Notifications, mobile dashboard, remote interaction, and web UI.
- General plugin SDK, rich attention detection, multi-agent orchestration, distributed execution, and cloud sync.
