# TauShell

[![Release](https://img.shields.io/github/v/release/ma99us/tau-shell?include_prereleases)](https://github.com/ma99us/tau-shell/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Platform: Windows 10+](https://img.shields.io/badge/platform-Windows%2010%2B-0078D4)

A Windows desktop manager for persistent AI coding sessions. Each session is a
PowerShell 7 process hosted through ConPTY, running a terminal-native harness
such as Claude Code or GitHub Copilot CLI. TauShell keeps ordinary PowerShell
behavior, never rewrites harness output, and resumes the harness conversation
when the harness supports it.

![TauShell with several sessions, a session-colored tab and top bar, and the Files drawer](docs/screenshots/taushell-main.png)

```text
TauShell -> ConPTY -> PowerShell 7 -> AI harness
```

## Features

- **Persistent sessions.** One YAML file per session; resume IDs and restore
  outcomes are kept apart in a SQLite store. Explicit Start, Restore and Start
  Fresh, plus Resume All and opt-in auto resume.
- **Harnesses.** Claude Code, GitHub Copilot CLI, GSD, OpenAI Codex CLI and
  OpenCode, as declarative YAML manifests. Add your own in
  `~/.tau_shell/harnesses/`.
- **Wrong-session safety.** Per-session colors tint the tab, sidebar row,
  top bar and terminal edge, with text chosen for contrast.
- **Attention cues.** State indicators, taskbar flash and Windows toast when a
  session needs an answer or finishes.
- **Project context.** Top bar with path, git branch, `*` markers for uncommitted
  or unpushed work, one-click **Sync changes** (stage, commit, pull --rebase,
  push), model, tokens and cost.
- **Files drawer.** Browse the project, view text, Markdown and images, insert
  paths into the prompt.
- **Toolbar and palette.** Prompt presets, commands, model, compact, interrupt;
  Ctrl+Shift+P palette, Ctrl+1..9 and Ctrl+Tab for tabs.
- **Workspaces.** Home can group sessions by project folder (off by default).
- **Skills.** A bundled library of agent skills from
  [ai-cookbook](https://github.com/mgerdov-igt/ai-cookbook), loaded into
  sessions without touching your own skill folders.
- **Orderly shutdown.** Closing the app or a running tab asks first, interrupts
  gracefully, then force-stops what remains and reports it.

## Install

Download `TauShell-<version>-windows-x64.zip` from the
[Releases page](https://github.com/ma99us/tau-shell/releases), extract it
anywhere and run `taushell.exe`. There is no installer and no admin rights are
needed. Session data lives in `%USERPROFILE%\.tau_shell`. Later releases can be
installed from Home (**Update and restart**, with rollback).

Requires Windows 10 or later, the Microsoft Edge WebView2 Runtime and
PowerShell 7; first run offers confirmed installs. Missing harnesses do not
block Home; install them from the Tools panel or their vendor pages.

Releases are pre-releases and unsigned, so SmartScreen warns on first launch
(**More info**, then **Run anyway**, or unblock the ZIP first). Windows 11 Smart
App Control blocks unsigned apps. To verify a download:

```powershell
(Get-FileHash .\TauShell-<version>-windows-x64.zip -Algorithm SHA256).Hash   # compare with SHA256SUMS.txt
gh attestation verify .\TauShell-<version>-windows-x64.zip --repo ma99us/tau-shell
```

## Quick start

1. Run `taushell.exe` and choose **Create session**, or add
   `%USERPROFILE%\.tau_shell\sessions\<id>.yaml`:

   ```yaml
   id: backend-agent
   name: Backend Agent
   tool: claude-code
   working_dir: C:/Projects/App
   ```

2. Start the session from Home. Right-click its tab to rename or recolor it.
3. Stop and Restore later to resume the harness conversation where supported.

Schema and options: [YAML schema](docs/v2/18-yaml-schema.md).

### Command line

```powershell
taushell                         # open Home
taushell list
taushell start <session-id>
taushell restore <session-id>
taushell ctl list [--json] [--instance <pid>]      # ctl needs control.enabled: true
taushell ctl wait <session> [--for idle|input|working] [--timeout 10m]
taushell ctl send [--insert-only] [--stdin] <session> <text...>
taushell ctl start [--restore|--fresh] <session>
taushell ctl stop <session>                        # graceful, like the Stop button
taushell ctl read [--rows N] <session>             # visible terminal text
```

`ctl send` is refused unless the harness state is confirmed.

## Status

Phase 1 (local Windows desktop) is implemented. Remote access and web
dashboards are out of scope; a phone server exists as an experimental, off by
default option.

| Harness | Verified |
|---|---|
| GitHub Copilot CLI | Launch, restore with real turns, usage, skills, attention cues |
| Claude Code | Launch, graceful exit, trust prompt, resume gate; model-dependent checks blocked on a credential |
| GSD, OpenCode | Launch, resume ID, graceful stop, restore; replies and usage unproven |
| OpenAI Codex CLI | Unit-tested only; never run signed in |

Unicode, ANSI, cursor movement, typed input and live resize are verified through
WebView2, xterm.js and ConPTY on Windows 10. Not yet live-verified: the
flash/toast alerts, some newer UI behavior, and a live winget install. Open
items are tracked in the [engineering handoff](docs/v2/21-engineering-handoff.md).

## Build

Requires Windows, Go 1.26+, Node.js and Python 3.

```powershell
Push-Location frontend; npm ci; npm run build; Pop-Location
go test ./...
python build\windows\gen_icon.py
go build -tags production -ldflags "-H=windowsgui" -o taushell.exe ./cmd/taushell
.\scripts\fetch-conpty.ps1
```

- `-H=windowsgui` is required; without it a console window can cover the app.
- If `npm ci` fails with `EALLOWSCRIPTS`, set `$env:npm_config_allow_scripts=''`.
- `fetch-conpty.ps1` downloads the pinned, signature-checked ConPTY package.
  Without `conpty.dll` and `OpenConsole.exe` next to the exe, TauShell uses the
  Windows 10 inbox ConPTY, which has mouse and status-reply limitations.
- `.\scripts\build-release.ps1 -Version <x.y.z>` builds the release ZIP and
  `SHA256SUMS.txt`; CI runs it when a `v*` tag is pushed.

## Diagnostics

Each run writes a JSON log to `%TEMP%\TauShell\taushell.log` (path shown on
Home). Prompt text, terminal I/O, environment values and credentials are
excluded, and credential-like strings in errors are redacted.

## Documentation

[`docs/v2/`](docs/v2/) is the active documentation: start with the
[product vision](docs/v2/00-product-vision.md),
[system architecture](docs/v2/01-system-architecture.md) and
[contributor guide](docs/v2/17-contributor-guide.md). Also see
[adding a harness](docs/v2/23-adding-a-harness.md),
[UI design system](docs/v2/25-ui-design-system.md) and
[security model](docs/v2/11-security-model.md). Agents and contributors using AI
tools should read [`AGENTS.md`](AGENTS.md).

## License

[MIT](LICENSE). Third-party notices: [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
