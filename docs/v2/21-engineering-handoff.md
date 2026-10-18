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
  frontend bridge declarations. Start and restore now open and size the
  xterm.js pane before launching the harness so initial output has a mounted
  terminal to receive it. Choosing to keep a live tab in the background keeps
  its xterm pane mounted, so its buffer continues receiving output and the
  screen is reused when the session is reopened. Built assets are embedded
  from `cmd/taushell/frontend/dist/`; build the frontend before Go compilation.
- `internal/sessionhost/manager_windows.go` owns the desktop session lifecycle:
  session listing, start/restore/fresh, ConPTY output events, terminal input
  and resize, exit supervision, and bounded graceful/forced shutdown. Fake
  process tests now cover concurrent session I/O and resize, graceful and
  forced shutdown, cleanup, restore failure without losing the saved harness
  session ID, manager-close idempotence, supervisor completion, and retry after
  stop timeout or database-close failure. A running runtime record without a
  process ID and an abandoned `starting` record are reconciled to failed
  without discarding their harness resume IDs. A live recorded process owned
  by another window remains marked running but not `RunningHere`. Manager-close
  tests also verify the output pipe handles close and manager supervisor
  goroutines return to their baseline. Desktop close-path tests keep the window
  open when listing sessions fails and retry a forced-stop notice if its native
  dialog fails.
- `internal/session/` validates strict YAML, safely creates session files,
  stores runtime state, and generates new harness IDs for fresh starts.
  `launch_args` is ordered and adapter-managed identity flags are rejected.
- `internal/conpty/`, `internal/harness/`, and `internal/terminal/` provide the
  shared process, adapter, and standalone CLI hosting behavior.
  ConPTY lifecycle tests verify that termination followed by `Close` clears
  every owned process, job, console, and pipe field and that a second close is
  safe. A process-wide handle-count stress test was inconclusive because the
  Go test process's handle count also rose during ordinary child-process runs;
  this does not establish a ConPTY leak or prove its absence.
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
existing ConPTY/CLI tests, YAML and runtime-store tests, a session-output test
that verifies base64 transport preserves raw terminal bytes, and manager tests
for concurrent independent I/O/resize, process exit, graceful and forced
shutdown, restore failure, and cleanup.

The navigation guard is also verified separately: a temporary production-mode
Wails host attempted a top-level navigation to a reserved `.invalid` origin,
and WebView2 raised `NavigationStarting` and cancelled it. That temporary
smoke host and its logs were removed; the check is not yet a checked-in
repeatable integration test. The host allows only the configured top-level
`http://wails.localhost` origin. Frame navigation and negative bridge-dispatch
tests remain open; Wails' origin validator is still an independent guard on
bound methods.

A browser-canvas interaction smoke with a mocked Wails bridge confirmed that
closing a live tab to the background retains the same xterm instance, buffers
output received while hidden, restores that output when the session is opened
again, and routes keyboard input to the reopened session. This tests frontend
state handling only; it does not verify xterm behavior inside Wails or the
ConPTY/PowerShell data path.

The same browser-only smoke exercised split UTF-8/emoji chunks, ANSI color,
alternate-screen switching, device-status query replies, mouse-tracking input
encoding, and fit-driven resize forwarding. These checks are not a substitute
for a repeatable full-screen fixture through the production WebView2/ConPTY
path.

The frontend browser preview was also exercised with 20 seeded persisted tabs:
ArrowLeft moved selection to the preceding tab, horizontal overflow controls
were enabled and scrolled the tab list, and the session form exposed the
persisted recent-folder suggestions. This is frontend-only coverage; browser
preview without the Wails bridge displays the bridge error and cannot exercise
session lifecycle or terminal transport.

The latest repeat of `npm run build`, `go test -count=1 ./...`, `go vet ./...`,
and the production-tagged build passed using a temporary Go 1.27.2 toolchain.
The frontend output has a non-failing Vite warning because its 566 kB JavaScript
chunk exceeds the default 500 kB advisory threshold. The Go archive SHA-256 was
verified against the recorded official value, and both archive and extracted
toolchain were removed afterward. `go.exe` is not currently discoverable on
this checkout's PATH; the older machine-specific handoff path below names
`C:\Users\Mike\...`, while this host profile is `C:\Users\Ira`. Use an installed
Go 1.26+ toolchain or redownload and verify the official archive before
repeating Go checks. Node.js/npm and the required WebView2 Runtime were
available.

The latest shutdown/recovery update again passed the full Windows Go test
suite, `go vet`, and a production-tagged build. It adds checks for the shutdown
confirmation and cancellation flow, forced-stop notices, stop failures, and
the case where a session is still running after `StopAll` returns successfully.

This verification used Windows 10 Pro 22H2 (build 19045), Go 1.27.2,
PowerShell 7.6.7, GitHub Copilot CLI 1.0.99-2, and WebView2 Runtime
155.0.4283.60. Claude Code 2.1.302 was installed after the user confirmed the
verified `Anthropic.ClaudeCode` winget package. An existing npm-installed
Claude Code 2.1.133 is earlier on PATH, so `claude` currently resolves to that
version while the winget executable is at its package path. Neither CLI has
passed provider-backed hosted-terminal testing. The Go toolchain used for
this run was downloaded to the system temp directory and its official archive
SHA-256 was verified; it was not installed system-wide.

## Do not overstate current readiness

The Step 1 WebView2/xterm.js feasibility gate remains open. Neither supported
harness has been exercised end-to-end inside the hosted terminal. The visible
window is not evidence that terminal behavior, harness restore, or application
security has passed.

Known gaps and risks:

- **Feasibility blocker:** test xterm input/output, split UTF-8 sequences, ANSI,
  terminal queries, alternate screen, cursor movement, resize, mouse modes,
  and Unicode/emoji against a controllable full-screen fixture, then perform
  non-destructive Claude Code and Copilot CLI walkthroughs on Windows 10.
  Existing ConPTY byte tests and the browser-only mocked-bridge xterm smoke do
  not validate the production WebView2/ConPTY path end to end.
- Add native ConPTY handle-leak checks and Wails-host close integration tests.
  Close-flow unit tests now cover cancellation, successful stop and
  verification, forced-stop reporting, stop failure, and keeping the window
  open when a managed session remains running after `StopAll` or the shutdown
  confirmation dialog fails. Confirmation-dialog failures are logged rather
  than silently ignored. The close path also reports session-list failures
  without allowing the window to close; forced-stop notices remain pending and
  retry after a native-dialog failure.
  Extend stale-runtime recovery tests beyond an abandoned startup, recent
  starting state, running records with missing or exited PIDs, and a live
  other-window process.
- The ConPTY job-cleanup test now launches a direct child fixture and verifies
  that inherited job members are terminated. A PowerShell `Start-Process`
  experiment produced a child outside the TauShell job; that detached launch
  survived `TerminateJobObject`. The agreed boundary is to manage only
  processes in TauShell's job and document that detached shell/broker launches
  are unmanaged; do not guess at process ancestry or terminate unrelated
  processes.
- The Windows host now blocks top-level navigation outside
  `http://wails.localhost` in WebView2's native `NavigationStarting` event,
  before the destination loads. A small repository-local Wails/WebView2 fork
  exposes this callback; Wails' existing origin validation remains enabled
  as a separate guard on bound-method dispatch. Origin-policy unit tests,
  a production host smoke test, and a production-mode WebView2 smoke that
  attempted a `.invalid` external navigation all passed on this machine.
- Exercise the missing-WebView2 native prompt and download handoff. Current
  runtime detection and prompt exist, but this machine has WebView2 installed
  and the missing-runtime path has not been tested in a clean environment.
- Tool detection is implemented, but first-run PowerShell setup, version
  support ranges, capability probes, winget consent/install actions, and
  vendor-link fallback are not.
- Session creation saves validated YAML, but generated harness-specific
  templates, editable YAML preview, effective-argv preview, and
  offer-to-start-after-creation are incomplete. The session form now suggests
  up to five recently created session folders, persisted in WebView local
  storage.
- Tabs and selected tab persist in WebView local storage. They do not
  auto-launch sessions. In-process background tabs retain their xterm buffer.
  The tab strip now has tab semantics, arrow/Home/End keyboard selection,
  active-tab scrolling, and horizontal overflow buttons. The complete
  keyboard-only walkthrough, persisted presentation state under different
  WebView profile conditions, and the background terminal path through the
  real Wails/ConPTY host remain unverified.
- Start Fresh is explicitly confirmed in the UI and rotates the persisted
  harness ID. Confirm failure/retry behavior with each adapter; Claude Code
  provider-backed restore remains unverified. A disposable Copilot CLI
  provider-backed restore smoke test is documented in the README.
- The WebView bridge currently receives terminal chunks as base64 events.
  Evaluate event volume and backpressure with long-running/high-output
  harnesses; do not switch to text decoding in Go.

### Gotchas and settled boundaries

- The repository-local Wails fork is wired through two `go.mod` replacements:
  Wails v2.16.0 and its nested go-webview2 v1.0.22 source. When refreshing
  either upstream dependency, preserve both licenses and reapply the narrow
  native `NavigationStarting` callback patch documented in
  `third_party/wails/TAUSHELL_PATCHES.md`; changing only the top-level Wails
  copy will not update the WebView2 event wrapper.
- Navigation policy currently covers top-level navigation only. The app
  cancels destinations outside exactly `http://wails.localhost`; it does not
  open external links in a browser. Do not widen allowed origins to make a
  dev server or external link work. Keep bridge-origin validation enabled,
  and add explicit frame/bridge tests before claiming the surface is secure.
- Multiple Claude Code versions exist on the machine: npm Claude Code 2.1.133
  is earlier on PATH than the winget 2.1.302 binary. Do not silently change
  PATH; use the explicit winget path for any authorized 2.1.302 check.
- PowerShell `Start-Process` can detach a child outside the ConPTY job.
  TauShell intentionally manages only job members and must not infer ancestry
  or kill detached processes.
- The temporary WebView2 navigation smoke proved event cancellation, not
  full xterm behavior, frame isolation, or provider restore. It was not kept
  in the repository.
- A process-wide Windows handle-count stress check is noisy in this Go test
  process: the count rose during both ordinary child-process runs and ConPTY
  runs, so it cannot distinguish runtime/thread retention from a native handle
  leak. ConPTY tests currently assert explicit `Close` success, cleared owned
  resource fields, idempotent close, and manager pipe/supervisor cleanup; a
  dedicated native leak check remains open.

## Recommended next actions

1. Complete the Step 1 terminal fixture and non-destructive walkthroughs for
   both harnesses on Windows 10, resolving the Claude PATH ambiguity first.
   A clean missing-WebView2 environment is unavailable on this machine; do
   not treat host startup or compile-only checks as the feasibility gate.
2. Continue independent shutdown/recovery coverage with native ConPTY
   handle-leak and Wails-host close integration checks; keep setup/install and
   session-template implementation behind the feasibility gate, as required
   by the implementation plan.
   The frontend tab strip's keyboard navigation and overflow controls are
   implemented; verify them as part of the desktop-host keyboard walkthrough.
3. Run `npm ci`, `npm run build`, `go test -count=1 ./...`, `go vet ./...`, and
   a production-tagged build after code changes. Re-run the executable startup
   smoke test when changing host/runtime code.
4. Keep README, V2 architecture/schema/backlog, and this handoff aligned with
   observed behavior. Do not mark a feasibility item complete based on a
   compile-only check.

## Build and workspace notes

Use Go 1.26 or newer and Node.js/npm on Windows. A production executable must
be built with Wails' `production` build tag; a plain `go build` can produce
Wails' diagnostic stub instead of the desktop host. Run the frontend build
first because Go embeds its output. Generated frontend files are part of the
application build input, not a substitute for frontend source.

Go is not currently on this checkout's PATH. The latest Windows verification
used a temporary Go 1.27.2 archive downloaded from the official Go distribution
and verified against SHA-256
`1314008898BD40DF77AF4B014F777F08873DBDFBCD3D92308728EE03304FE04F`; the
archive and extracted toolchain were removed after verification. Do not
hard-code machine-specific toolchain paths in project scripts.
