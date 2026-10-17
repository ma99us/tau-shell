# Engineering Handoff

**Updated:** 2026-10-17
**Project state:** Phase 1 desktop migration in progress; not release-ready.

This is the implementation handoff for the next engineering session. The
active product and engineering requirements are in `docs/v2/`; the desktop
interaction decisions and feasibility gate are in
`docs/v1/08-UI-Design.md` and `docs/v1/09-UI-Implementation-Plan.md`.

## Product boundaries

- Phase 1 is a local Windows desktop app using PowerShell 7 through ConPTY.
- Session intent lives in human-editable YAML; runtime and harness resume state
  live separately in SQLite.
- Claude Code and GitHub Copilot CLI have built-in launch/restore adapters.
  Restore is explicit and must not be reported as successful unless the
  harness actually resumes.
- The desktop app is the only interactive TauShell interface. Bubble Tea has
  been removed. `list`, `start <session-id>`, and `restore <session-id>` remain
  standalone commands.
- Windows 10 terminal encoding, Unicode/emoji bytes, ANSI output, and
  provider-preserving output remain requirements. TauShell must not rewrite
  harness output.
- Do not add remote access, mobile integration, web dashboards, or
  notifications in Phase 1. Do not inspect or modify `docs/.todo`.

## Current implementation

- `cmd/taushell/main_windows.go` routes no-argument launch to the desktop app
  and keeps the CLI commands available.
- `cmd/taushell/desktop_windows.go` checks the Evergreen WebView2 registry
  registration before host startup; creates a Wails window with embedded
  frontend assets; binds session operations; exposes directory selection;
  detects tool executables and versions; and confirms app close when sessions
  are running.
- `frontend/` contains the React launcher, session creation form, Home tool
  inventory, local tab-order/selection storage, xterm.js panes, and typed
  frontend bridge declarations. Built assets are embedded from
  `cmd/taushell/frontend/dist/`; build the frontend before Go compilation.
- `internal/sessionhost/manager_windows.go` owns the desktop session lifecycle:
  session listing, start/restore/fresh, ConPTY output events, terminal input
  and resize, exit supervision, and bounded graceful/forced shutdown.
- `internal/session/` validates strict YAML, safely creates session files,
  stores runtime state, and generates new harness IDs for fresh starts.
  `launch_args` is ordered and adapter-managed identity flags are rejected.
- `internal/conpty/`, `internal/harness/`, and `internal/terminal/` provide the
  shared process, adapter, and standalone CLI hosting behavior.
- TUI-specific Bubble Tea, ultraviolet, and x/vt modules were removed by
  `go mod tidy`. The checked-in `cmd/taushell/frontend/dist/` directory is
  required by Go's `embed` directive; it is generated from `frontend/`.

## Verified on the current Windows 10 development machine

The following checks passed after the latest implementation changes:

```powershell
Push-Location frontend
npm ci
npm run build
Pop-Location
go test -count=1 ./...
go vet ./...
go build -tags production -o taushell.exe ./cmd/taushell
```

The production-tagged executable was smoke-launched and opened a responsive
TauShell window using the installed WebView2 Runtime. This verifies host
startup and local embedded asset loading only. The Windows tests include the
existing ConPTY/CLI tests, YAML and runtime-store tests, and a session-output
test that verifies base64 transport preserves raw terminal bytes.

## Do not overstate current readiness

The Step 1 WebView2/xterm.js feasibility gate remains open. Neither supported
harness has been exercised end-to-end inside the hosted terminal. The visible
window is not evidence that terminal behavior, harness restore, or application
security has passed.

Known gaps and risks:

- Test xterm input/output, split UTF-8 sequences, ANSI, terminal queries,
  alternate screen, cursor movement, resize, mouse modes, and Unicode/emoji
  against a controllable full-screen fixture, then perform non-destructive
  Claude Code and Copilot CLI walkthroughs on Windows 10.
- Terminal output events can arrive before a newly-started session's xterm
  instance is mounted. Review startup ordering and add a bounded per-session
  output queue or initialize the terminal before process launch so initial
  prompts/output cannot be lost.
- Add lifecycle tests for concurrent fake sessions, independent input/output,
  process exit, restore failure, graceful interruption, forced termination,
  app-close error handling, and goroutine/handle cleanup. The manager currently
  has no broad lifecycle integration test suite.
- Explicitly verify that external navigation cannot access Wails-bound methods.
  The app binds a small typed API, but external-navigation and origin behavior
  have not been tested or explicitly restricted in the app configuration.
- Exercise the missing-WebView2 native prompt and download handoff. Current
  runtime detection and prompt exist, but the missing-runtime path has not been
  tested in a clean environment.
- Tool detection is implemented, but first-run PowerShell setup, version
  support ranges, capability probes, winget consent/install actions, and
  vendor-link fallback are not.
- Session creation saves validated YAML, but generated harness-specific
  templates, editable YAML preview, effective-argv preview, recent folders,
  and offer-to-start-after-creation are incomplete.
- Tabs and selected tab persist in WebView local storage. They do not
  auto-launch sessions. Keyboard tab navigation, tab overflow controls, and
  explicit restoration of presentation state under different WebView profile
  conditions remain unverified.
- Start Fresh is explicitly confirmed in the UI and rotates the persisted
  harness ID. Confirm failure/retry behavior with each adapter; Claude Code
  provider-backed restore remains unverified. A disposable Copilot CLI
  provider-backed restore smoke test is documented in the README.
- The WebView bridge currently receives terminal chunks as base64 events.
  Evaluate event volume and backpressure with long-running/high-output
  harnesses; do not switch to text decoding in Go.

## Recommended next actions

1. Close the terminal startup-output race and write session-manager lifecycle
   tests using fake harness executables and temporary session stores.
2. Complete Step 1's terminal fixture tests, both harness walkthroughs, runtime
   missing-path check, resize/input checks, and local-origin/bridge security
   verification. Record exact Windows/WebView2/harness versions and outcomes
   in the UI design document. If either harness is unreliable, stop and
   present evidence and alternatives before expanding the UI.
3. Complete the setup/install and session-template/preview behavior only after
   the feasibility gate is accepted.
4. Run `npm ci`, `npm run build`, `go test -count=1 ./...`, `go vet ./...`, and
   a production-tagged build after code changes. Re-run the executable startup
   smoke test when changing host/runtime code.
5. Keep README, V2 architecture/schema/backlog, and this handoff aligned with
   observed behavior. Do not mark a feasibility item complete based on a
   compile-only check.

## Build and workspace notes

Use Go 1.26 or newer and Node.js/npm on Windows. A production executable must
be built with Wails' `production` build tag; a plain `go build` can produce
Wails' diagnostic stub instead of the desktop host. Run the frontend build
first because Go embeds its output. Generated frontend files are part of the
application build input, not a substitute for frontend source.

The Windows toolchain is currently available at
`C:\Users\Mike\AppData\Local\Programs\go1.27.2\bin\go.exe` on the development
machine, but do not hard-code this machine-specific path in project scripts.
