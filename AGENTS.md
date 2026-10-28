# Agent guidance

## Private user notes

- Never read, search, summarize, quote, modify, or otherwise inspect `docs/.todo` or `docs/.todo.md`.
- Treat these paths as private user-only notes: exclude them from version control and changes; do not stage, commit, or track their contents.

## Source of truth and scope

- Use [`docs/v2/`](docs/v2/) as the active product and engineering documentation. Do not treat the V1 ZIP archives or V3 ideas summary as current requirements unless the user explicitly asks to revisit them.
- Current agreed scope is Phase 1: a local Windows desktop GUI (Go host with WebView2 UI; host/terminal dependencies require a Windows 10 feasibility spike); PowerShell 7 hosted through ConPTY; human-edited YAML session definitions; runtime state stored separately; harness-specific restore where supported: Claude Code and GitHub Copilot CLI, plus GSD, OpenAI Codex CLI and OpenCode (newer: GSD and OpenCode verified live for launch, resume ID, graceful stop and restore; model replies, usage and Codex are unproven; see the engineering handoff). Opt-in live harness check: `TAUSHELL_LIVE=gsd|opencode|codex go test ./internal/sessionhost -run TestLiveHarness -v`. The current Bubble Tea TUI is a disposable prototype and must be replaced/removed, not kept as a second frontend or fallback. Keep `list`, `start`, and `restore` CLI commands available.
- Detect WebView2, PowerShell 7, supported harnesses, versions, and declared tool dependencies. Missing host prerequisites use a first-run setup path; missing harnesses do not block Home. Offer confirmed installs via known winget packages, with official vendor links as fallback; do not silently install or elevate.
- Use installed tool versions, maintain per-adapter tested ranges and capability probes, warn outside tested ranges, and do not silently pin, downgrade, or auto-update. Generated session YAML may include editable ordered harness CLI arguments with harness-specific defaults.
- Target Windows 10 from Phase 1, including UTF-8/code-page setup and automated Unicode/emoji byte-path and ANSI-output checks. Keep TauShell-owned controls ASCII; visual glyph/color compatibility depends on the user's terminal and fonts, and provider output must not be rewritten.
- If TauShell is closed while sessions are running, warn and require confirmation. On confirmation, attempt graceful interruption, wait briefly, then force-terminate remaining processes. A graceful stop may improve harness resume reliability but does not guarantee it.
- Phase 1 excludes remote access, phones/mobile integration, and web dashboards. Local attention alerts (taskbar flash and Windows toast when a session needs an answer or finishes, configurable in `~/.tau_shell/config.yaml`, both on by default) are in Phase 1; see [`docs/v2/22-local-attention-notifications.md`](docs/v2/22-local-attention-notifications.md).
- V2 is still authoritative overall, but its roadmap/backlog includes capabilities later than Phase 1. Apply the explicit scope above where those conflict.
- Phase 1 implementation is underway in `cmd/` (host) and `internal/` (sessions, adapters, ConPTY, tools) with the React/xterm.js UI in `frontend/`. Treat the V2 documents as specifications and keep implementation claims aligned with tested behavior.
- Build: `npm ci && npm run build` in `frontend/` (set `$env:npm_config_allow_scripts=''` if npm fails with `EALLOWSCRIPTS`), then `go test ./...` and `go build -tags production -ldflags "-H=windowsgui" -o taushell.exe ./cmd/taushell` (see the README). Stop a running `taushell.exe` by PID before rebuilding. Built outputs (`taushell.exe`, ConPTY files, app icon) are gitignored; the embedded `cmd/taushell/frontend/dist` is tracked.
- UI conventions: sessions appear in the same order on Home, the sidebar and the tabs; Home is a house icon; session icons come from the project folder (initials on the session color), not the harness; harness marks are official Simple Icons credited in `THIRD_PARTY_NOTICES.md`. Do not open external links with `target="_blank"`; use a bound Go opener restricted to known URLs.
- Real-harness probes (Claude Code, Copilot CLI) must use disposable directories and sessions, never print credentials, and clean up transcripts and temp files afterwards.
- Earlier bounded prototypes live in ignored [`spikes/`](spikes/): ConPTY findings are summarized in [`docs/v2/02-conpty-integration.md`](docs/v2/02-conpty-integration.md), and YAML/runtime-state findings in [`docs/v2/12-storage-layout.md`](docs/v2/12-storage-layout.md). Use them as references, not as the application architecture.

## Architecture principles

- Keep PowerShell first-class; do not replace it with a custom shell.
- Model persistent sessions, not terminal tabs or windows.
- Keep core session management tool-independent. Harness-specific launch/resume behavior belongs behind tool-specific adapters.
- Distinguish restoring the PowerShell process/environment from resuming the AI harness conversation. Do not claim a conversation was restored unless the harness confirms or supports that outcome; surface unsupported and failed resumes.
- Make shutdown interruption and any forced termination visible; do not silently kill active sessions.
- Treat hand-edited YAML as session intent/configuration. Keep changing runtime metadata, such as resume identifiers and restore outcomes, separate from YAML.
- Preserve session-scoped environment handling. Do not put secret values in configuration or logs.
- Keep TauShell-managed session/resume arguments under adapter control; represent user harness arguments as discrete argv values, not shell strings. Prompt presets/model/command widgets are later work and must be capability-gated.
- Validate terminal encoding/rendering end to end through ConPTY and the selected embedded terminal component; process-level UTF-8 settings alone do not prove correct display or input behavior.

## Read documentation selectively

Read only the references relevant to the task:

- Product goals or scope: [`docs/v2/00-product-vision.md`](docs/v2/00-product-vision.md), [`docs/v2/16-roadmap.md`](docs/v2/16-roadmap.md), [`docs/v2/20-mvp-backlog.md`](docs/v2/20-mvp-backlog.md).
- Architecture or runtime: [`docs/v2/01-system-architecture.md`](docs/v2/01-system-architecture.md), [`docs/v2/02-conpty-integration.md`](docs/v2/02-conpty-integration.md), [`docs/v2/09-powershell-bootstrap.md`](docs/v2/09-powershell-bootstrap.md).
- Windows terminal compatibility: [`docs/v2/15-terminal-rendering.md`](docs/v2/15-terminal-rendering.md).
- Session lifecycle or recovery: [`docs/v2/03-session-lifecycle.md`](docs/v2/03-session-lifecycle.md), [`docs/v2/04-session-restoration.md`](docs/v2/04-session-restoration.md), [`docs/v2/12-storage-layout.md`](docs/v2/12-storage-layout.md).
- YAML or environment: [`docs/v2/18-yaml-schema.md`](docs/v2/18-yaml-schema.md), [`docs/v2/10-environment-management.md`](docs/v2/10-environment-management.md).
- Harness adapters/plugins: [`docs/v2/05-plugin-system.md`](docs/v2/05-plugin-system.md), [`docs/v2/23-adding-a-harness.md`](docs/v2/23-adding-a-harness.md); Claude Code verification status: [`docs/v2/24-claude-code-verification-plan.md`](docs/v2/24-claude-code-verification-plan.md).
- Attention/state cues: [`docs/v2/08-attention-detection.md`](docs/v2/08-attention-detection.md); local attention alerts (flash/toast, Phase 1): [`docs/v2/22-local-attention-notifications.md`](docs/v2/22-local-attention-notifications.md).
- Desktop UI: [`docs/v1/08-UI-Design.md`](docs/v1/08-UI-Design.md), [`docs/v1/09-UI-Implementation-Plan.md`](docs/v1/09-UI-Implementation-Plan.md).
- Security-sensitive work: [`docs/v2/11-security-model.md`](docs/v2/11-security-model.md).
- Mobile, remote access, notifications, terminal rendering, or protocol work: read the corresponding V2 document only when that work is in scope.
- Contributor workflow or handoff: [`docs/v2/17-contributor-guide.md`](docs/v2/17-contributor-guide.md), [`docs/v2/21-engineering-handoff.md`](docs/v2/21-engineering-handoff.md).

## Implementation expectations

- Inspect existing files and follow established patterns before changing code.
- Keep changes focused and update directly related documentation.
- Validate changes with the smallest relevant available check; report checks that could not be run.
- Do not silently swallow errors or report success when launch or restore failed.
