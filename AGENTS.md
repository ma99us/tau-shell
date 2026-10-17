# Agent guidance

## Source of truth and scope

- Use [`docs/v2/`](docs/v2/) as the active product and engineering documentation. Do not treat the V1 ZIP archives or V3 ideas summary as current requirements unless the user explicitly asks to revisit them.
- Current agreed scope is Phase 1: a local CLI/TUI; PowerShell 7 hosted through ConPTY; human-edited YAML session definitions; runtime state stored separately; harness-specific restore where supported, initially Claude Code and GitHub Copilot CLI.
- Target Windows 10 from Phase 1, including UTF-8/code-page setup and tested Unicode, color, box-drawing, emoji, and Nerd Font behavior; use readable fallbacks rather than changing machine-wide settings.
- If TauShell is closed while sessions are running, warn and require confirmation. On confirmation, attempt graceful interruption, wait briefly, then force-terminate remaining processes. A graceful stop may improve harness resume reliability but does not guarantee it.
- Phase 1 excludes remote access, phones/mobile integration, web dashboards, and notifications.
- V2 is still authoritative overall, but its roadmap/backlog includes capabilities later than Phase 1. Apply the explicit scope above where those conflict.
- The project is currently in planning. Do not invent implementation details or expand scope; identify unresolved behavior and update relevant documentation when asked.

## Architecture principles

- Keep PowerShell first-class; do not replace it with a custom shell.
- Model persistent sessions, not terminal tabs or windows.
- Keep core session management tool-independent. Harness-specific launch/resume behavior belongs behind tool-specific adapters.
- Distinguish restoring the PowerShell process/environment from resuming the AI harness conversation. Do not claim a conversation was restored unless the harness confirms or supports that outcome; surface unsupported and failed resumes.
- Make shutdown interruption and any forced termination visible; do not silently kill active sessions.
- Treat hand-edited YAML as session intent/configuration. Keep changing runtime metadata, such as resume identifiers and restore outcomes, separate from YAML.
- Preserve session-scoped environment handling. Do not put secret values in configuration or logs.
- Validate terminal encoding/rendering end to end through ConPTY; process-level UTF-8 settings alone do not prove correct display or input behavior.

## Read documentation selectively

Read only the references relevant to the task:

- Product goals or scope: [`docs/v2/00-product-vision.md`](docs/v2/00-product-vision.md), [`docs/v2/16-roadmap.md`](docs/v2/16-roadmap.md), [`docs/v2/20-mvp-backlog.md`](docs/v2/20-mvp-backlog.md).
- Architecture or runtime: [`docs/v2/01-system-architecture.md`](docs/v2/01-system-architecture.md), [`docs/v2/02-conpty-integration.md`](docs/v2/02-conpty-integration.md), [`docs/v2/09-powershell-bootstrap.md`](docs/v2/09-powershell-bootstrap.md).
- Windows terminal compatibility: [`docs/v2/15-terminal-rendering.md`](docs/v2/15-terminal-rendering.md).
- Session lifecycle or recovery: [`docs/v2/03-session-lifecycle.md`](docs/v2/03-session-lifecycle.md), [`docs/v2/04-session-restoration.md`](docs/v2/04-session-restoration.md), [`docs/v2/12-storage-layout.md`](docs/v2/12-storage-layout.md).
- YAML or environment: [`docs/v2/18-yaml-schema.md`](docs/v2/18-yaml-schema.md), [`docs/v2/10-environment-management.md`](docs/v2/10-environment-management.md).
- Harness adapters/plugins: [`docs/v2/05-plugin-system.md`](docs/v2/05-plugin-system.md).
- Security-sensitive work: [`docs/v2/11-security-model.md`](docs/v2/11-security-model.md).
- Mobile, remote access, notifications, terminal rendering, or protocol work: read the corresponding V2 document only when that work is in scope.
- Contributor workflow or handoff: [`docs/v2/17-contributor-guide.md`](docs/v2/17-contributor-guide.md), [`docs/v2/21-engineering-handoff.md`](docs/v2/21-engineering-handoff.md).

## Implementation expectations

- Inspect existing files and follow established patterns before changing code.
- Keep changes focused and update directly related documentation.
- Validate changes with the smallest relevant available check; report checks that could not be run.
- Do not silently swallow errors or report success when launch or restore failed.
