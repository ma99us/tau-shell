# TauShell

TauShell is a PowerShell-first orchestrator for persistent sessions with terminal-native AI coding tools. It manages sessions rather than terminal tabs and aims to restore both the terminal environment and the AI harness conversation when the harness supports resuming it.

## Project status

TauShell is in the planning and documentation stage. The current planning baseline is **Phase 1: a local CLI/TUI**:

- Run AI tools in PowerShell 7 hosted through ConPTY.
- Target the Windows 10 development environment from the start, including UTF-8/code-page setup, Unicode and box-drawing compatibility, color preservation, and Nerd Font/emoji checks with readable fallbacks.
- Define sessions in human-edited YAML.
- Persist runtime/session state separately from configuration.
- Restore the PowerShell environment and, when supported, resume the intended AI harness session using tool-specific commands.
- Begin with built-in support for Claude Code and GitHub Copilot CLI; report unsupported or failed resume attempts clearly.
- Warn and require confirmation before closing while sessions are running; on confirmation, attempt graceful interruption before force-terminating sessions that remain after a brief wait.

Remote access, phone/mobile integration, web dashboards, and notifications are not part of Phase 1.

## Documentation

[`docs/v2/`](docs/v2/) is the active product and engineering documentation set. Start with:

- [Product vision](docs/v2/00-product-vision.md)
- [System architecture](docs/v2/01-system-architecture.md)
- [Session lifecycle](docs/v2/03-session-lifecycle.md)
- [Session restoration](docs/v2/04-session-restoration.md)
- [YAML schema](docs/v2/18-yaml-schema.md)
- [Workspaces](docs/v2/19-workspaces.md)
- [MVP backlog](docs/v2/20-mvp-backlog.md)
- [Engineering handoff](docs/v2/21-engineering-handoff.md)
- [PowerShell bootstrap](docs/v2/09-powershell-bootstrap.md)
- [Terminal rendering and compatibility](docs/v2/15-terminal-rendering.md)

The V2 documents are evolving specifications; where they still describe mobile or remote features as near-term requirements, use the Phase 1 scope above. V1 archives and V3 ideas are historical/reference material, not the current plan.

## Core runtime

```text
TauShell -> ConPTY -> PowerShell 7 -> AI harness
```

TauShell should preserve ordinary PowerShell behavior and avoid coupling its core session management to any one AI provider.
