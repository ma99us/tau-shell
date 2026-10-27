# TauShell

TauShell is a PowerShell-first orchestrator for persistent sessions with
terminal-native AI coding tools. It is a Windows desktop app (Go + Wails
WebView2 host, React/xterm.js UI) where each session is a PowerShell 7 process
hosted through ConPTY, running a harness such as Claude Code or GitHub Copilot
CLI. When the harness supports explicit session restore, TauShell resumes the
conversation.

```text
TauShell -> ConPTY -> PowerShell 7 -> AI harness
```

TauShell keeps ordinary PowerShell behavior, never rewrites harness output, and
keeps core session management independent of any one AI provider.

## Status

Phase 1 is under way and the desktop app is an early implementation, not a
verified release. Remote access, phone/mobile integration, web dashboards and
notifications are not part of Phase 1. The old Bubble Tea TUI has been removed
and no TUI fallback is planned.

**Implemented**

- PowerShell 7 hosted through ConPTY (bundled ConPTY preferred, inbox fallback),
  with UTF-8/code-page setup.
- Human-edited YAML session definitions; runtime state (resume IDs, restore
  outcomes) in a separate SQLite store.
- Built-in Claude Code and Copilot CLI adapters, tool/version detection with
  tested ranges, and confirmed winget installs with vendor-page fallback.
- Home page with session tiles, tabs, sidebar, session create/rename/recolor/
  delete, explicit Start / Restore / Start Fresh, per-session state indicators.
- Close confirmation, then graceful interruption, then bounded force
  termination, with any forced stop reported.

**Verified on the Windows 10 development machine**

- Unicode/emoji, ANSI, cursor movement, terminal queries, typed input and live
  resize through WebView2, xterm.js, PowerShell 7 and ConPTY.
- Both harnesses render end to end in the hosted terminal.
- Copilot CLI restore of a session with real turns: the model recalled earlier
  context after Stop and Restore.
- Claude Code 2.1.311: graceful `/exit`, trust-folder prompt, title cues and the
  resume gate. Checks that need a model reply are blocked until a working
  credential is available.
- Top-level navigation outside `http://wails.localhost` is blocked.

**Still open:** Claude Code model-dependent checks, frame-navigation and
bridge-denial tests, an automated Wails-host close test, and a live winget
install run. See [`docs/v2/21-engineering-handoff.md`](docs/v2/21-engineering-handoff.md)
and [`docs/v2/24-claude-code-verification-plan.md`](docs/v2/24-claude-code-verification-plan.md).
Planned UI work (drag-and-drop reorder, "Open session", duplicate session) is
listed in [`docs/v2/20-mvp-backlog.md`](docs/v2/20-mvp-backlog.md).

## Using TauShell

```powershell
taushell                      # open the desktop app (never auto-starts sessions)
taushell list
taushell start <session-id>
taushell restore <session-id>
```

- Sessions are shown in the same order on Home, in the sidebar and in the tabs.
  Home is the house icon. Session icons come from the project folder name
  (initials, tinted with the session color when one is set).
- Right-click a session in the sidebar or its tab to rename or recolor it. New
  sessions are named after the project folder.
- Running sessions have a toolbar: **Prompts** (presets from
  `%USERPROFILE%\.tau_shell\prompts.yaml`, see the
  [schema](docs/v2/18-yaml-schema.md#prompt-presets)), **Commands**, **Model**,
  **Compact** and **Interrupt**. Ctrl+Shift+P opens a command palette;
  Ctrl+1..9 selects a tab; Ctrl+Tab cycles. Text is submitted only when the
  harness looks idle, otherwise it is just inserted. The UI reports "sent", not
  "applied". Verified on Copilot CLI; Claude Code widgets are not yet verified.
- Home has **Resume All**, which restores, one after another, every stopped
  session that has a saved harness session (sessions with a resume warning are
  skipped; use Start fresh on those). Failures are reported per session.
- Closing a tab keeps a live session running in the background with its
  terminal buffer; sessions are not auto-launched after a restart.
- The terminal has no scrollback of its own; scrolling is left to the harness.
  Ctrl+C copies when text is selected (otherwise it goes to the harness),
  Ctrl+V and Shift+Insert paste, Ctrl+Shift+C always copies.
- Closing the app asks before interrupting live sessions.

Create one YAML file per session at
`%USERPROFILE%\.tau_shell\sessions\<session-id>.yaml` (or use **Create
session**); see the [schema](docs/v2/18-yaml-schema.md):

```yaml
id: backend-agent
name: Backend Agent
tool: claude-code
working_dir: C:/Projects/App
powershell:
  utf8: true
```

Install PowerShell 7 and the harness CLI (`claude` or `copilot`) on `PATH`.
`start` creates a harness session ID and `restore` reuses it. TauShell closes
the managed PowerShell process when its harness exits.

## Build

Requires Windows, Go 1.26+, Node.js and Python 3 (icon generation).

```powershell
Push-Location frontend
npm ci
npm run build
Pop-Location
go test ./...
python build\windows\gen_icon.py
go build -tags production -ldflags "-H=windowsgui" -o taushell.exe ./cmd/taushell
.\scripts\fetch-conpty.ps1
```

- `-H=windowsgui` is required for direct `go build`; without it a console
  window can cover the app.
- If `npm ci` fails with `EALLOWSCRIPTS` because of a user-level `allow-scripts`
  npm setting, set `$env:npm_config_allow_scripts=''` for that shell.
- `gen_icon.py` writes the gitignored app icon and `.syso` resource.
- `fetch-conpty.ps1` downloads the pinned MIT-licensed
  `Microsoft.Windows.Console.ConPTY` package, verifies its SHA-256 and
  Authenticode signatures, and copies `conpty.dll` and `OpenConsole.exe` next to
  `taushell.exe` (`-Destination` for another folder). Windows 10's inbox ConPTY
  consumes mouse-mode requests and status replies, so without these two files
  TauShell falls back to inbox ConPTY with those limitations. The diagnostic log
  records which backend each session used.
- The built frontend is embedded from `cmd/taushell/frontend/dist`.

## Diagnostics

Each process writes a structured JSON log to `%TEMP%\TauShell\taushell.log`
(path shown on Home), replaced on the next launch. It records startup, session
lifecycle, prerequisite detection, bridge errors and frontend failures. Prompt
text, terminal I/O, environment values and credentials are excluded, and
credential-like strings in errors are redacted.

## Documentation

[`docs/v2/`](docs/v2/) is the active documentation set; apply the Phase 1 scope
above where it still describes later remote/mobile work. Start with:

- [Product vision](docs/v2/00-product-vision.md) and [MVP backlog](docs/v2/20-mvp-backlog.md)
- [System architecture](docs/v2/01-system-architecture.md), [ConPTY integration](docs/v2/02-conpty-integration.md), [PowerShell bootstrap](docs/v2/09-powershell-bootstrap.md)
- [Session lifecycle](docs/v2/03-session-lifecycle.md), [restoration](docs/v2/04-session-restoration.md), [storage layout](docs/v2/12-storage-layout.md)
- [Harness adapters](docs/v2/05-plugin-system.md) and [adding a harness](docs/v2/23-adding-a-harness.md)
- [YAML schema](docs/v2/18-yaml-schema.md), [workspaces](docs/v2/19-workspaces.md)
- [Terminal rendering](docs/v2/15-terminal-rendering.md)
- [Claude Code verification plan](docs/v2/24-claude-code-verification-plan.md)
- [Contributor guide](docs/v2/17-contributor-guide.md) and [engineering handoff](docs/v2/21-engineering-handoff.md)
- Desktop UI: [design](docs/v1/08-UI-Design.md), [implementation plan](docs/v1/09-UI-Implementation-Plan.md) (other V1 documents are historical)

The ignored [`spikes/`](spikes/) directory holds earlier prototypes (ConPTY,
session config); they are references, not the application architecture.
Third-party icons and components are credited in
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
