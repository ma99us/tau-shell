# Roadmap

## Phase 1 — Local Windows desktop session manager

- Implemented: Go/Wails desktop host with a local WebView2 UI. The Bubble Tea
  TUI has been removed; the desktop app is the only interactive frontend. See
  the [engineering handoff](21-engineering-handoff.md) for verification limits.
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
- Implemented: capability-gated harness widgets, reusable prompt presets and
  command actions. Availability and verification vary by harness; see the
  [MVP backlog](20-mvp-backlog.md).

See [UI design](../v1/08-UI-Design.md) and
[UI implementation plan](../v1/09-UI-Implementation-Plan.md).

## Later phases

Future-only V3 planning (research, gap analysis, ranked feature specs) lives in
[`docs/v3/`](../v3/README.md); it is not a current requirement.

- **Deferred architecture spike (not Phase 1 scope):** evaluate launching a
  harness directly under ConPTY as an alternative to the current PowerShell
  host. Do not change the PowerShell-first design before the spike's Windows
  process, `.cmd` resolution, environment, encoding, lifecycle, and UX criteria
  pass. See the [backlog](20-mvp-backlog.md#deferred-alternative-spike-direct-conpty-harness-launch)
  and [ConPTY notes](02-conpty-integration.md#deferred-alternative-launch-the-harness-directly).
- A general plugin SDK, third-party adapter loading, and workspace/group
  management.
- Remote access, mobile control, remote push notifications, web dashboards,
  multi-agent orchestration, and distributed agents remain later-scope
  capabilities and are not Phase 1 requirements. Local attention alerts
  (taskbar flash and Windows toast) are Phase 1; see
  [`22-local-attention-notifications.md`](./22-local-attention-notifications.md).
