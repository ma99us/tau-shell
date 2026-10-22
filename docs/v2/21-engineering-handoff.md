# Engineering Handoff

**Updated:** 2026-10-19
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
  are running. Its Wails close callback returns immediately while confirmation
  and session stopping run asynchronously, avoiding a blocked native message
  loop. The Home tool inventory shows the resolved executable path and
  detected version so PATH precedence is visible. The shutdown prompt uses
  Yes/No responses and defaults safely to No.
  Session creation returns its never-started snapshot from the
  validated configuration after saving, without rereading the full session
  list; an unrelated listing failure can no longer make a successful save look
  like a failed create. Desktop tests cover the returned snapshot, persisted
  launch arguments, no-list behavior, invalid configuration, and duplicate-ID
  rejection without overwriting an existing YAML file.
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
go build -tags production -ldflags "-H=windowsgui" -o taushell.exe ./cmd/taushell
```

The production-tagged GUI-subsystem executable was smoke-launched and opened a
responsive, styled TauShell window using the installed WebView2 Runtime. The
`-H=windowsgui` linker flag is required for direct `go build` use; without it,
the console window can cover the desktop window when Wails writes startup
logs. The Windows tests include the
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

The latest asynchronous close-flow update passed `go test -count=1 ./...`,
`go vet ./...`, and a production-tagged build. It adds a regression test that
the Wails close callback returns while confirmation is blocked, ignores
duplicate close attempts during that work, and allows the app to close only
after confirmation completes. A separate native production smoke with an
inert fixture verified the responsive Yes/No dialog behavior.

This verification used Windows 10 Pro 22H2 (build 19045), Go 1.27.2,
PowerShell 7.6.7, GitHub Copilot CLI 1.0.99-2, and WebView2 Runtime
155.0.4283.60. Claude Code 2.1.302 was installed after the user confirmed the
verified `Anthropic.ClaudeCode` winget package. An existing npm-installed
Claude Code 2.1.133 is earlier on PATH, so `claude` currently resolves to that
version while the winget executable is at its package path. Neither CLI has
passed provider-backed hosted-terminal testing. TauShell follows the first
executable resolved from `PATH`; Home now displays that exact path beside the
detected version, making the selected installation visible without changing
precedence. The Go toolchain used for
this run was downloaded to the system temp directory and its official archive
SHA-256 was verified; it was not installed system-wide.

A deterministic terminal fixture in `cmd/terminal-fixture/` was built as
`copilot.exe` and run from an isolated TauShell profile through the production
Wails window, session bridge, PowerShell 7, and real ConPTY. The hosted xterm
rendered Greek/Japanese text, a combining mark, emoji, ANSI colors, cursor
movement/erase, and a four-byte emoji split across writes 50 ms apart; it
replied to a cursor-position query (`ESC[10;1R`), forwarded typed input
(`tau-proof`), and forwarded a live resize from 117x35 to 94x28. The fixture's
explicit `q` exit was also recorded. This is fixture-path evidence only, not
Claude Code or Copilot CLI compatibility evidence. The explicit exit restored
the preserved primary-screen marker. Repeated hosted clicks produced no
captured mouse report. A later post-start device-status reply was accepted and
written by the Go bridge but did not reach any fixture input chunk. A repeated
cursor-position query produced a fixture-visible seven-byte input chunk
without a matching bridge marker. These remain unexplained.
The shutdown prompt exposed a native Wails Yes/No return-value mismatch; the
host now treats `Yes` as confirmation and defaults/cancels to `No`. Unit tests
check the native choice values and safe default. A production Wails smoke with
an inert live fixture verified the active-session prompt remains responsive:
No left TauShell and the fixture running, while Yes stopped the fixture
process and closed TauShell. The fixture's PowerShell process exited with code
1 during interruption, so this does not prove graceful-stop success or cover
the forced-termination notice. The native close flow runs outside Wails'
synchronous `OnBeforeClose` callback; automated Wails-host integration
coverage remains open.

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
  The checked-in fixture has verified Unicode/emoji, split UTF-8 output, ANSI,
  cursor movement, cursor-position query replies, typed input, explicit exit,
  primary-screen restoration, and live resize through the production
  WebView2/ConPTY path. Mouse reporting, device-status query delivery, and real
  harness walkthroughs still need verification.
  - An isolated browser-side xterm check emitted `ESC[0n` for a device-status
    query and SGR mouse reports when tracking was enabled and the terminal was
    clicked. This does not validate the Wails bridge/ConPTY path. Payload-free
    diagnostics now log the first successfully forwarded device-status,
    cursor-position, SGR mouse, and X10 mouse report category per session, plus
    pointer-event categories and xterm's effective mouse-tracking mode without
    coordinates. A production run observed `pointerdown` and `mousedown` in
    the terminal container, but xterm remained in `none` mode and emitted no
    mouse report. Resending `CSI ? 1000 h` and `CSI ? 1006 h` from the fixture
    did not change the mode. A temporary xterm-side probe enabled `vt200` and
    forwarded an SGR mouse report to the Go bridge, proving the UI input path
    works when tracking is active. A direct ConPTY probe found that Windows
    ConPTY consumes those DECSET controls before they reach xterm, while
    preserving adjacent text output. Do not force mouse tracking
    unconditionally; that changes normal selection and input behavior. Mouse
    support across the PowerShell/ConPTY boundary needs an explicit design
    decision.
    The device-status reply (`ESC[0n`) still reaches the Go bridge and its
    ConPTY input write succeeds, but no corresponding fixture input chunk
    arrives. The fixture's `c` key produced a fixture-visible cursor reply
    (`ESC[6;15R`) without a matching bridge marker; its source remains
    unknown. Fixture input chunk sizes are recorded without contents.
  - A repeated native handle-close test now verifies that the process, job,
    input-pipe, and output-pipe handles are valid before `Close` and invalid
    afterward. It passed 10 consecutive runs. The opaque pseudoconsole handle
    is not compatible with `GetHandleInformation`; its successful close and
    cleared field are still covered by the existing cleanup test. Wails-host
    close integration tests remain open.
  Close-flow unit tests now cover cancellation, successful stop and
  verification, forced-stop reporting, stop failure, and keeping the window
  open when a managed session remains running after `StopAll` or the shutdown
  confirmation dialog fails. Confirmation-dialog failures are logged rather
  than silently ignored. The close path also reports session-list failures
  without allowing the window to close; forced-stop notices remain pending and
  retry after a native-dialog failure.
  A production-window smoke exercised both No and Yes with an inert fixture;
  an automated Wails-host close integration test and forced-termination notice
  verification remain open.
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
  active-tab scrolling, and horizontal overflow buttons. A mocked-browser
  keyboard smoke verified Home/End selection, arrow-key wraparound, focus
  movement, and synchronized selected-tab state. A production-host smoke
  exposed a missing flex layout on the tab strip; after the CSS fix, Home and
  Right switched focus/selection between Home and a stopped fixture tab. A
  mouse click seeded focus, so a complete keyboard-only walkthrough remains
  unverified. With eight inert fixture tabs open, the native left/right
  controls scrolled between earlier and later tabs. Persisted presentation
  state under different WebView profile conditions and the background terminal
  path through the real Wails/ConPTY host also remain unverified.
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
- A process-wide Windows handle-count stress check is noisy in the Go test
  process. An isolated comparison rose by 59 handles across 20 ConPTY
  Start/Wait/Close cycles and by 41 handles across 20 ordinary PowerShell
  launches in its control process. The difference is not reliable enough to
  attribute to ConPTY, so the experiment was discarded as a regression test.
  A follow-up isolated `CreatePseudoConsole`/`ClosePseudoConsole` probe, with
  no client process and all four pipe handles closed, observed one additional
  handle targeting `C:\Windows\System32\conhost.exe`. The conhost process had
  exited after close, but its process handle remained in the calling test
  process. This local Windows 10 result points to ConPTY/conhost lifecycle
  behavior rather than an unclosed handle field in TauShell's `Process`; do
  not add an unsafe handle-table workaround or brittle process-wide threshold.
  A deterministic test now verifies that process, job, and input/output pipe
  handles are valid before `Close` and invalid afterward, repeated 10 times.
  The opaque pseudoconsole handle cannot be checked with `GetHandleInformation`;
  its successful close and cleared field remain verified. Existing tests also
  assert cleared resource fields, idempotent close, and manager
  pipe/supervisor cleanup. A comprehensive native handle-leak check remains
  open.

## Recommended next actions

1. Run the bundled-ConPTY spike (decided): load a newer `conpty.dll` +
   `OpenConsole.exe` behind a loader switch in `internal/conpty`, keep inbox
   ConPTY as fallback, and compare both using the terminal fixture for
   mouse-mode passthrough (`CSI ? 1000 h`/`CSI ? 1006 h`), device-status and
   cursor-report delivery (hypothesis: inbox ConPTY answers them itself), and
   the residual console-host process handle. Record license, Windows 10
   support, and packaging. Adopt only if mouse modes pass through; do not
   force xterm mouse tracking.
2. Run user-authorized, non-destructive Claude Code and Copilot CLI hosted
   walkthroughs with the user's existing sign-ins: start, short read-only
   prompt, stop, restore/resume. Record Home-displayed executable path and
   version; TauShell uses the first PATH match.
3. In parallel (decided), implement Step 3 tool metadata/version ranges,
   capability probes, and confirmed winget installs, plus Step 4 session
   templates with argv preview; neither depends on the terminal path.
   Missing-WebView2 verification is deferred to a manual Windows Sandbox/VM
   check; rely on registry-detection unit tests until then.
4. Add Wails-host close integration coverage and complete the keyboard-only
   workflow.
   A mocked-browser smoke verified frontend Home/End and arrow-key tab
   selection, and a production-host smoke verified Home/Right switching after
   fixing tab-strip layout; native left/right overflow controls also scrolled
   eight inert fixture tabs.
5. Run `npm ci`, `npm run build`, `go test -count=1 ./...`, `go vet ./...`, and
   a production-tagged build after code changes. Re-run the executable startup
   smoke test when changing host/runtime code.
6. Keep README, V2 architecture/schema/backlog, and this handoff aligned with
   observed behavior. Do not mark a feasibility item complete based on a
   compile-only check.

## Diagnostics logging

- Every process truncates `%TEMP%\TauShell\taushell.log` at startup and emits
  JSON records with timestamps, severity, source locations, and safe
  operational metadata. The Home screen displays the resolved path.
- Go command failures and top-level panics, Wails framework errors, session
  lifecycle outcomes, UI operation failures, browser errors/unhandled promise
  rejections, and React render errors are recorded. The Wails bridge logger
  suppresses failed request payloads so terminal input is not copied into the
  diagnostic log.
- Do not add prompt text, terminal bytes/text, environment values, credentials,
  or launch argument values to diagnostic attributes. Error messages and
  frontend stacks are bounded and credential-like strings are redacted.
- Log initialization failure is surfaced and prevents the process from
  starting without the requested diagnostic file. A process terminated by the
  OS or a runtime failure before Go initialization cannot write an app log.

## Build and workspace notes

Use Go 1.26 or newer and Node.js/npm on Windows. A production executable must
be built with Wails' `production` build tag; a plain `go build` can produce
Wails' diagnostic stub instead of the desktop host. Direct `go build` also
needs `-ldflags "-H=windowsgui"` to prevent the console subsystem window from
covering the desktop app. Run the frontend build first because Go embeds its
output. Generated frontend files are part of the application build input, not
a substitute for frontend source.

Go is not currently on this checkout's PATH. The latest Windows verification
used a temporary Go 1.27.2 archive downloaded from the official Go distribution
and verified against SHA-256
`1314008898BD40DF77AF4B014F777F08873DBDFBCD3D92308728EE03304FE04F`; the
archive and extracted toolchain were removed after verification. Do not
hard-code machine-specific toolchain paths in project scripts.
