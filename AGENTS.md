# Agent guidance

## Private user notes

- Never read, search, summarize, quote, modify, or otherwise inspect `docs/.todo` or `docs/.todo.md`.
- Treat these paths as private user-only notes: exclude them from version control and changes; do not stage, commit, or track their contents.

## What this is

TauShell is a local Windows desktop session manager: a Go host (`cmd/taushell`, `internal/`) with a React/xterm.js UI (`frontend/`) in WebView2. Each session is a PowerShell 7 process hosted through ConPTY, running an AI harness (Claude Code, GitHub Copilot CLI, GSD, OpenAI Codex CLI, OpenCode). Session intent is human-edited YAML; runtime state (resume IDs, restore outcomes) lives separately in SQLite. See [`README.md`](README.md) for features and usage.

## Scope

- Phase 1 is the current scope. [`docs/v2/`](docs/v2/) is the active documentation; where its roadmap or backlog describes later work, this section wins.
- Out of scope: remote access, web dashboards, and phone integration beyond the experimental, off-by-default phone server. Local attention alerts (taskbar flash, Windows toast) are in scope.
- Do not treat [`docs/v1/`](docs/v1/) (historical) or [`docs/v3/`](docs/v3/) (future-only planning) as requirements unless the user asks.
- The CLI keeps `list`, `start`, `restore` and `ctl`. There is no TUI and no TUI fallback.
- Target Windows 10 and later. Keep TauShell-owned UI text ASCII; never rewrite provider output.
- Detect WebView2, PowerShell 7, harnesses, versions and declared tool dependencies. Missing host prerequisites use first-run setup; missing harnesses do not block Home. Offer confirmed installs (winget, vendor links as fallback); never install or elevate silently.
- Use installed tool versions; keep per-adapter tested ranges and capability probes; warn outside them; never silently pin, downgrade or auto-update.
- Closing TauShell with running sessions needs confirmation, then graceful interruption, a short wait, then forced termination. Closing a running session's tab needs the same confirmation. A graceful stop does not guarantee resume.

## Architecture rules

- Keep PowerShell first-class; do not replace it with a custom shell.
- Model persistent sessions, not terminal tabs or windows.
- Keep core session management tool-independent; harness launch/resume belongs in manifests/adapters.
- Restoring the PowerShell process is not resuming the harness conversation. Claim a resume only when the harness confirms it; surface unsupported and failed resumes.
- Make interruption and forced termination visible.
- YAML is intent; keep changing runtime metadata out of it.
- Keep session-scoped environment handling; never put secrets in configuration or logs.
- Keep managed session/resume arguments under adapter control; user harness arguments are discrete argv values, not shell strings. Harness-dependent widgets are capability-gated.
- Prove terminal encoding and rendering end to end (ConPTY, xterm.js); process-level UTF-8 alone is not proof.
- Git branch and status data come from the Go host, not from frontend parsing.

## UI conventions

- Sessions appear in the same order on Home, the sidebar and the tabs. The sidebar lists only sessions with an open tab; Home lists all.
- Home is a house icon. Session icons show initials of the session name on the session color; harness marks are Simple Icons credited in `THIRD_PARTY_NOTICES.md`.
- Session color tints the tab, row, top bar and terminal edge; text color comes from the `sessionColor.ts` contrast rules (4.5:1). Use theme tokens, not hard-coded colors ([`docs/v2/25-ui-design-system.md`](docs/v2/25-ui-design-system.md)).
- No `target="_blank"`; open external links through the bound Go opener restricted to known URLs.

## Build and verify

- Frontend: `npm ci && npm run build` in `frontend/` (set `$env:npm_config_allow_scripts=''` on `EALLOWSCRIPTS`). Type check: `npx tsc --noEmit`. Unit tests: `npm test`.
- Go: `go test ./...`, then `go build -tags production -ldflags "-H=windowsgui" -o taushell.exe ./cmd/taushell`. Build the frontend first; the embedded `cmd/taushell/frontend/dist`, `taushell.exe`, ConPTY files and icon resources are gitignored.
- Stop a running `taushell.exe` by PID before rebuilding.
- Opt-in live harness check: `TAUSHELL_LIVE=gsd|opencode|codex go test ./internal/sessionhost -run TestLiveHarness -v`. The `./internal/sessionhost` suite takes about 2 minutes.
- Regenerate the harness golden fixture with `$env:UPDATE_GOLDEN=1; go test ./internal/harness/manifest`.
- Real-harness probes use disposable directories and sessions, never print credentials, and clean up transcripts and temp files.

## Implementation expectations

- Inspect existing code and follow its patterns. Keep changes focused; update directly related docs.
- Keep documentation claims aligned with tested behavior; say what was not verified.
- Run the smallest relevant check and report checks that could not run.
- Do not swallow errors or report success when launch or restore failed.
- Line endings are LF everywhere; never write CRLF. From PowerShell use `[IO.File]::WriteAllText` with `` `n ``, not `Set-Content`/`Out-File`. `gofmt -l cmd internal` must print nothing (CI enforces it).

## Read documentation selectively

Read only what the task needs:

| Task | Read |
|---|---|
| Product goals, scope | [`00-product-vision`](docs/v2/00-product-vision.md), [`16-roadmap`](docs/v2/16-roadmap.md), [`20-mvp-backlog`](docs/v2/20-mvp-backlog.md) |
| Architecture, runtime | [`01-system-architecture`](docs/v2/01-system-architecture.md), [`02-conpty-integration`](docs/v2/02-conpty-integration.md), [`09-powershell-bootstrap`](docs/v2/09-powershell-bootstrap.md) |
| Terminal rendering, clipboard, encoding | [`15-terminal-rendering`](docs/v2/15-terminal-rendering.md) |
| Session lifecycle, restore, storage | [`03-session-lifecycle`](docs/v2/03-session-lifecycle.md), [`04-session-restoration`](docs/v2/04-session-restoration.md), [`12-storage-layout`](docs/v2/12-storage-layout.md) |
| YAML, environment, prompt presets | [`18-yaml-schema`](docs/v2/18-yaml-schema.md), [`10-environment-management`](docs/v2/10-environment-management.md) |
| Harness adapters, new harness | [`05-plugin-system`](docs/v2/05-plugin-system.md), [`23-adding-a-harness`](docs/v2/23-adding-a-harness.md), [`24-claude-code-verification-plan`](docs/v2/24-claude-code-verification-plan.md) |
| Attention cues, alerts | [`08-attention-detection`](docs/v2/08-attention-detection.md), [`22-local-attention-notifications`](docs/v2/22-local-attention-notifications.md) |
| UI, theme, workspaces | [`25-ui-design-system`](docs/v2/25-ui-design-system.md), [`19-workspaces`](docs/v2/19-workspaces.md) |
| Security-sensitive work (files drawer, bridge, phone server) | [`11-security-model`](docs/v2/11-security-model.md) |
| Phone, API, protocol (only when in scope) | [`06-mobile-ui`](docs/v2/06-mobile-ui.md), [`13-api-specification`](docs/v2/13-api-specification.md), [`14-websocket-protocol`](docs/v2/14-websocket-protocol.md), [`07-notification-engine`](docs/v2/07-notification-engine.md) |
| Contributing, status, known gaps | [`17-contributor-guide`](docs/v2/17-contributor-guide.md), [`21-engineering-handoff`](docs/v2/21-engineering-handoff.md), [`26-architecture-audit-handoff`](docs/v2/26-architecture-audit-handoff.md) |
| Local environment | [`LOCAL-ENVIRONMENT.md`](LOCAL-ENVIRONMENT.md) |

Prototypes in the ignored `spikes/` folder are references only; their findings are summarized in `02-conpty-integration` and `12-storage-layout`.
