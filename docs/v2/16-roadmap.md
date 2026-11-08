# Roadmap

## Phase 1 — Local Windows desktop session manager

- Implemented: Go/Wails desktop host with a local WebView2 UI. The Bubble Tea
  TUI has been removed; the desktop app is the full local desktop frontend.
  See the [engineering handoff](21-engineering-handoff.md) for verification
  limits.
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
- Codex has a local rate-limit reader and curated model suggestions alongside
  its native picker and custom model IDs. Live quota/auth compatibility and
  suggested model availability remain unverified; account-aware catalog and
  additional verified commands remain planned. See the
  [Codex usage and toolbar handoff](27-codex-quota-handoff.md).

See the current [UI design system](25-ui-design-system.md). The earlier
[V1 UI design](../v1/08-UI-Design.md) and
[implementation plan](../v1/09-UI-Implementation-Plan.md) are historical.

## Later phases

Future-only V3 planning (research, gap analysis, ranked feature specs) lives in
[`docs/v3/`](../v3/README.md); it is not a current requirement.

- **Deferred architecture spike (not Phase 1 scope):** evaluate launching a
  harness directly under ConPTY as an alternative to the current PowerShell
  host. Do not change the PowerShell-first design before the spike's Windows
  process, `.cmd` resolution, environment, encoding, lifecycle, and UX criteria
  pass. See the [backlog](20-mvp-backlog.md#deferred-alternative-spike-direct-conpty-harness-launch)
  and [ConPTY notes](02-conpty-integration.md#deferred-alternative-launch-the-harness-directly).
- A general plugin SDK, third-party adapter loading, and broader
  workspace/group management. Automatic project grouping and eligible group
  resume/stop already exist; see [Workspaces](19-workspaces.md).
- Remote access, mobile control, remote push notifications, web dashboards,
  multi-agent orchestration, and distributed agents remain later-scope
  capabilities and are not Phase 1 requirements. An experimental phone server
  and Android companion already exist, off by default; see the
  [phone security model](11-security-model.md#phone-server-experimental-off-by-default).
  Local attention alerts (taskbar flash and Windows toast) are Phase 1; see
  [`22-local-attention-notifications.md`](./22-local-attention-notifications.md).
