# Roadmap

## Phase 1 — Local Windows desktop session manager

- Go desktop host with a local WebView2 UI (host implementation to be validated).
- The current full-screen Bubble Tea CLI/TUI is a disposable prototype to
  replace and remove, not a supported parallel frontend or fallback.
- Home/setup screen with dependency detection and confirmed installation
  options.
- Session creation from a selected harness and working folder, with editable
  YAML and harness-specific argument defaults.
- Tabbed workspace with an embedded terminal per PowerShell 7/ConPTY session.
- Explicit start/restore, concurrent background sessions, truthful statuses,
  and confirmed shutdown.
- Keep `list`, `start`, and `restore` CLI commands for scripting/recovery.
- Validate supported tool versions and capabilities without silently pinning
  or downgrading.

See [UI design](../v1/08-UI-Design.md) and
[UI implementation plan](../v1/09-UI-Implementation-Plan.md).

## Later phases

- Capability-gated model selection, reusable prompt presets, and harness
  command widgets.
- Broader harness/plugin support and workspace management.
- Remote access, mobile control, notifications, web dashboards, multi-agent
  orchestration, and distributed agents remain later-scope capabilities and
  are not Phase 1 requirements.
