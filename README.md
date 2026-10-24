# TauShell

TauShell is a PowerShell-first orchestrator for persistent sessions with
terminal-native AI coding tools. Its target interactive experience is a Windows
desktop GUI with a session launcher and tabs containing embedded
PowerShell/ConPTY harness terminals. It aims to resume the harness conversation
when the harness supports explicit session restore.

## Project status

Phase 1 implementation is underway. The Bubble Tea TUI has been removed; no
TUI fallback is planned. No-argument launch now opens the Windows desktop
application, while `list`, `start`, and `restore` remain standalone commands.

The initial desktop implementation uses a Go/Wails WebView2 host with bundled
React and xterm.js assets. It includes a local session launcher, persisted
presentation tabs, session creation, tool discovery, explicit Start/Restore/
Start Fresh actions, ConPTY input/output/resize wiring, and a shutdown
confirmation path. The session manager is UI-independent, and harness IDs and
runtime metadata remain outside session YAML.

Keeping a live session in the background retains its xterm instance and buffer
until the app closes; sessions are never auto-launched after restart. Browser
smokes with a mocked bridge exercised background output and xterm handling for
split UTF-8/emoji, ANSI, alternate screen, terminal queries, mouse reporting,
resize, and keyboard input. The production WebView2/ConPTY path has not passed
those end-to-end checks.

**The desktop feasibility gate is not yet complete.** The Go package and
frontend compile and their current tests pass, but neither supported harness
has passed a hosted-terminal walkthrough on the target Windows environment.
Missing-runtime behavior, installer/setup flows, full terminal protocol
compatibility, and provider-backed Claude Code restore remain outstanding.
Treat the UI as an early implementation, not a verified release. Top-level
navigation outside the bundled `http://wails.localhost` origin is blocked and
smoke-tested; frame navigation and cross-origin bridge denial are not separately
tested.

Implemented: PowerShell 7 hosted through ConPTY; human-edited YAML with separate SQLite runtime state; built-in Claude Code and Copilot CLI command adapters; UTF-8/code-page setup; and desktop close confirmation with graceful interruption followed by bounded force termination.

Automated Windows ConPTY tests verify PowerShell output/exit codes, resizing,
and that forced termination stops direct job-child fixtures. Shell/broker
launches detached outside TauShell's Job Object remain unmanaged.

Remaining work includes end-to-end WebView2/xterm.js testing with both
harnesses, terminal protocol/Unicode/ANSI validation, prerequisite setup and
confirmed install flows, and provider-backed restore with Claude Code. A
disposable Copilot CLI provider-backed restore smoke test passed. TauShell must
not rewrite harness output; glyph rendering depends on the selected terminal
renderer and installed fonts.

Remote access, phone/mobile integration, web dashboards, and notifications are not part of Phase 1.

The current CLI commands are:

```powershell
taushell
taushell list
taushell start <session-id>
taushell restore <session-id>
```

Running `taushell` without arguments opens the desktop launcher. It does not
start sessions automatically; use **Start** or **Restore**. Tabs may continue
running in the background. Closing the app asks before interrupting live
sessions.

## Diagnostics

Each TauShell process truncates and writes a structured JSON diagnostic log to
`%TEMP%\TauShell\taushell.log` (the actual path is shown on Home). It records
startup, session lifecycle, prerequisite detection, bridge errors, frontend
runtime/rejection failures, and React render failures. Prompt text, terminal
input/output, environment values, and credentials are excluded; credential-like
strings in error text are redacted. The file is local to the current Windows
user's temp directory and is replaced on the next launch.

Build on Windows with Go 1.26 or newer and Node.js:

```powershell
Push-Location frontend
npm ci
npm run build
Pop-Location
go test ./...
go build -tags production -ldflags "-H=windowsgui" -o taushell.exe ./cmd/taushell
.\scripts\fetch-conpty.ps1
```

`scripts\fetch-conpty.ps1` downloads the pinned, MIT-licensed
`Microsoft.Windows.Console.ConPTY` package, verifies its SHA-256 hash and
Microsoft Authenticode signatures, and copies the x64 `conpty.dll` and
`OpenConsole.exe` next to `taushell.exe` (use `-Destination` for another
folder). TauShell prefers this bundled ConPTY because Windows 10's inbox ConPTY
consumes harness mouse-mode requests and terminal status replies. Without the
two files, TauShell falls back to inbox ConPTY with those limitations, and the
diagnostic log records which backend each session used.

Create one YAML file at `%USERPROFILE%\.tau_shell\sessions\<session-id>.yaml` for each session; see the [Phase 1 schema](docs/v2/18-yaml-schema.md). For example:

```yaml
id: backend-agent
name: Backend Agent
tool: claude-code
working_dir: C:/Projects/App
powershell:
  utf8: true
```

Install PowerShell 7 and the selected harness CLI (`claude` or `copilot`) on `PATH`. `start` creates a harness session ID and `restore` reuses that ID. A fake executable verified both commands receive the same explicit UUID. A disposable test with Copilot CLI 1.0.100-1 then confirmed that `restore` reopened the conversation and recovered an exact response marker without using tools or accessing files; Claude Code's provider-backed restore remains unverified. TauShell closes the managed PowerShell process when its harness exits.

The ignored [`spikes/conpty/`](spikes/conpty/) and [`spikes/session-config/`](spikes/session-config/) directories contain earlier prototypes and test references, not the application architecture.

## Documentation

[`docs/v2/`](docs/v2/) is the active product and engineering documentation set. Start with:

- [Product vision](docs/v2/00-product-vision.md)
- [System architecture](docs/v2/01-system-architecture.md)
- [ConPTY integration and spike findings](docs/v2/02-conpty-integration.md)
- [Session lifecycle](docs/v2/03-session-lifecycle.md)
- [Session restoration](docs/v2/04-session-restoration.md)
- [Harness adapters](docs/v2/05-plugin-system.md)
- [YAML schema](docs/v2/18-yaml-schema.md)
- [Storage layout](docs/v2/12-storage-layout.md)
- [Workspaces](docs/v2/19-workspaces.md)
- [MVP backlog](docs/v2/20-mvp-backlog.md)
- [Engineering handoff](docs/v2/21-engineering-handoff.md)
- [PowerShell bootstrap](docs/v2/09-powershell-bootstrap.md)
- [Terminal rendering and compatibility](docs/v2/15-terminal-rendering.md)
- [Desktop UI design](docs/v1/08-UI-Design.md)
- [Desktop UI implementation plan](docs/v1/09-UI-Implementation-Plan.md)

The V2 documents are the active product and engineering specifications; apply
the current Phase 1 scope where they still describe later remote/mobile
features. The desktop UI direction and implementation plan are recorded in
[`docs/v1/08-UI-Design.md`](docs/v1/08-UI-Design.md) and
[`docs/v1/09-UI-Implementation-Plan.md`](docs/v1/09-UI-Implementation-Plan.md).
Other V1 documents are historical unless explicitly referenced by current
specifications.

## Core runtime

```text
TauShell -> ConPTY -> PowerShell 7 -> AI harness
```

TauShell should preserve ordinary PowerShell behavior and avoid coupling its core session management to any one AI provider.
