# Engineering Handoff

**Updated:** 2026-10-24
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
- Do not add remote access, mobile integration, or web dashboards in Phase 1.
  Local attention alerts (taskbar flash and Windows toast) are in Phase 1; see
  `22-local-attention-notifications.md`. Do not inspect or modify `docs/.todo`.

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
  Sessions can be deleted from Home cards and the sidebar (`x`, with a
  confirmation warning): `DeleteSession` removes the YAML and the runtime row,
  refuses running/starting sessions, and never touches the harness's own data
  or the working directory.
  Sidebar rows and tabs show a state icon (`StateIcon` in `main.tsx`): spinner
  = output in the last 2.5 s ("working"), solid green = output stopped
  ("quiet"), grey = not running, red = failed. A quiet session that was not
  selected+focused glows amber ("unseen") until selected or typed in. Signals
  are harness-specific (`frontend/src/attention.ts`), never whole-screen
  comparison: Copilot CLI = its "Working ... esc" status row / "? help" footer
  (last two rows only; verified live) and, for "waiting for your answer", the
  selection-prompt footer "enter to select ... esc to cancel" (verified live
  on Copilot CLI 1.0.103 with a permission prompt in a session started without
  `--yolo`; shown as an orange square, glowing until seen, cleared when the
  prompt is answered). Other Copilot pickers share that footer and are also
  treated as waiting for input. Claude Code = terminal title (braille
  spinner = working, U+2733 = idle; verified live on 2.1.311: idle
    `✳ Claude Code`, working animates U+25D0-25D3 as well as braille); its
    "needs input" cue is the "Enter to confirm ... Esc to cancel" footer seen on
    the trust-folder prompt; the tool-permission prompt itself is not yet
    captured (provider returns 401). With no cue (unknown tool) it falls back to output timing
  (output within 400 ms of a keystroke is ignored as echo). "Quiet" is not
  proof the harness awaits input. Copilot
  sends only topic titles, no BEL/OSC 9/777/133.
  The session name is optional: when blank, `CreateSession` uses the project
  name from `package.json`, `go.mod`, `Cargo.toml`, or `pyproject.toml` in the
  folder, else the folder name, adding ` (2)`, ` (3)` on collisions.
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
bound methods. Code review of frame/new-window exposure: the UI has no
iframes, `innerHTML`, or `window.open`, and the vendored Wails host handles
only top-level `NavigationStarting` (no `NewWindowRequested` handler). The one
`target="_blank"` link (tool-install notice) was therefore replaced by a button
that calls the metadata-restricted `OpenToolSetupPage` binding; a live frame
injection test is still not automated.

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

The Step 1 WebView2/xterm.js feasibility gate is substantially closed: both
supported harnesses have now been exercised end-to-end inside the hosted
terminal with real, signed-in credentials on this Windows 10 machine (see
below). A clean-environment test without WebView2 will not be performed (user
decision; the existing prompt is accepted as working). Also open: completing an actual OAuth login flow for Claude Code (deliberately not
attempted, to avoid live browser-based authentication during automated
testing). The visible window alone is still not evidence that terminal
behavior, harness restore, or application security has passed in general —
rely on the specific verified findings below, not on the window simply
rendering.

Known gaps and risks:

- **Feasibility blocker:** test xterm input/output, split UTF-8 sequences, ANSI,
  terminal queries, alternate screen, cursor movement, resize, mouse modes,
  and Unicode/emoji against a controllable full-screen fixture, then perform
  non-destructive Claude Code and Copilot CLI walkthroughs on Windows 10.
  The checked-in fixture has verified Unicode/emoji, split UTF-8 output, ANSI,
  cursor movement, cursor-position query replies, typed input, explicit exit,
  primary-screen restoration, and live resize through the production
  WebView2/ConPTY path.
  - **Resolved: real harness walkthroughs, user-authorized.** With explicit
    user authorization, real signed-in Claude Code and GitHub Copilot CLI
    processes were launched through the production app in the real (non-test)
    user profile. Claude Code rendered its genuine first-run wizard (ASCII-art
    banner, theme-selection menu, syntax-highlighted diff preview) correctly;
    one menu step was confirmed via Enter to reach "Select login method",
    where testing deliberately stopped rather than complete a real OAuth
    login. GitHub Copilot CLI rendered its full real UI (tab bar, AI
    disclaimer, live org-policy error banners, status bar) correctly and a
    local `/exit` command (not an actual model prompt, to avoid consuming
    real AI credits) was used to exit cleanly. Both harnesses correctly
    render Unicode/ASCII art, ANSI/syntax-highlighting colors,
    inline-autocomplete/ghost-text, and multi-line interactive menus through
    the real production WebView2 + bundled ConPTY + PowerShell 7 path. See
    the Start Fresh/restore bullet below for the corresponding restore
    findings from this same walkthrough.
  - An isolated browser-side xterm check emitted `ESC[0n` for a device-status
    query and SGR mouse reports when tracking was enabled and the terminal was
    clicked. This does not validate the Wails bridge/ConPTY path. Payload-free
    diagnostics now log the first successfully forwarded device-status,
    cursor-position, SGR mouse, and X10 mouse report category per session, plus
    pointer-event categories and xterm's effective mouse-tracking mode without
    coordinates. An earlier production run (on inbox ConPTY, before the
    bundled-ConPTY adoption) observed `pointerdown`/`mousedown` events but
    xterm stayed in `none` mode and emitted no mouse report; a direct ConPTY
    probe found that Windows' inbox ConPTY consumes `CSI ? 1000 h`/
    `CSI ? 1006 h` DECSET controls before they reach xterm, while preserving
    adjacent text output.
  - **Resolved by the bundled-ConPTY adoption.** Re-running the same fixture
    (built as `copilot.exe`, isolated profile, production WebView2/ConPTY
    host) with bundled `conpty.dll`/`OpenConsole.exe` present no longer
    reproduces any of the above gaps: the diagnostic log recorded
    `terminal mouse tracking mode changed` → `vt200` immediately after
    startup, a `mousedown` pointer event, and a matching `mouse-sgr` forwarded
    marker on the very first hosted click; the fixture's own event log shows
    the startup device-status and cursor-position replies each arriving as a
    distinct, correctly-sized input chunk with a matching report line, and a
    manual re-query (`d`/`c` keys) after that produced another correctly-sized
    input chunk and matching report in the fixture log (the Go-side
    diagnostic marker is logged only once per category per session by
    design, so it does not re-fire on the second query — this is not a gap).
    Do not force mouse tracking unconditionally regardless; that still
    changes normal selection and input behavior. Mouse support only needed an
    explicit design decision while inbox ConPTY was in use; bundled ConPTY
    removes the underlying DECSET-consuming behavior that motivated it.
    Fixture input chunk sizes are recorded without contents.
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
  verification remain open. Decision: a fully automated real-window
  `OnBeforeClose` CI test is deemed infeasible without new Windows GUI test
  infrastructure (Wails does not expose a navigation/close-denied callback
  suitable for scripted assertions); this is treated as an accepted,
  documented limitation covered by the mocked unit tests plus manual
  production-window smokes above, not a pending action item.
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
- Missing-WebView2 native prompt and download handoff: runtime detection and
  prompt exist; a clean-environment test will not be performed (user
  decision) and the path is accepted as working.
- Tool detection, version support ranges, capability probes, first-run
  PowerShell setup, winget consent/install actions, and vendor-link fallback
  are implemented in `internal/tools` and wired through
  `cmd/taushell/tools_windows.go` and the Home/Setup frontend. Verified via
  unit tests and a production Wails smoke of the rendered Home/Setup UI; a
  live winget install run was not exercised (only the declined-confirmation
  and concurrent-install-rejection paths were).
- Session creation saves validated YAML. Generated harness-specific launch
  templates (Claude Code →
  `--dangerously-skip-permissions`, Copilot CLI → `--yolo`) and a live
  effective-argv preview (resolved executable, full argv including the
  managed `--session-id` placeholder, and unsupported-option warnings probed
  against the harness's own `--help` output) are implemented via
  `Adapter.DefaultLaunchArgs()` and the new `GetSessionTemplate`/
  `PreviewSessionLaunch` bound methods, verified with new Go unit tests and an
  isolated-profile UI smoke test for both harnesses.
  Editable full-YAML preview remains
  incomplete. The session form now suggests up to five recently created
  session folders, persisted in WebView local storage.
- Tabs and selected tab persist in WebView local storage. They do not
  auto-launch sessions. In-process background tabs retain their xterm buffer.
  The tab strip now has tab semantics, arrow/Home/End keyboard selection,
  active-tab scrolling, and horizontal overflow buttons. A mocked-browser
  keyboard smoke verified Home/End selection, arrow-key wraparound, focus
  movement, and synchronized selected-tab state. A production-host smoke
  exposed a missing flex layout on the tab strip; after the CSS fix, Home and
  Right switched focus/selection between Home and a stopped fixture tab. With
  eight inert fixture tabs open, the native left/right controls scrolled
  between earlier and later tabs.
  A production-host smoke starting from a fresh profile with zero mouse
  clicks confirmed `Tab` from window launch reaches the tab strip directly
  (seeding focus on "Home" without a click), and a further keyboard-only walk
  (`Tab`/`Enter`/typed text) opened New Session, filled every field, and
  submitted the dialog. Multi-tab switching with a second open session tab
  was then verified without invoking any live harness, by resolving
  `exec.LookPath("copilot")` to the checked-in `terminal-fixture` binary
  (copied to an isolated profile's `PATH` as `copilot.exe`) so `StartSession`
  launches the harmless fixture instead of a real credentialed CLI. With
  "Home" and the running fixture tab both open, repeated `Shift+Tab`/`Tab`
  reached each session's left-sidebar list entry, and pressing `Enter` on
  either entry switched the active top tab and rendered its content
  correctly in both directions (Home -> fixture tab and back) with zero
  mouse clicks. Persisted presentation state
  under different WebView profile conditions and the background terminal
  path through the real Wails/ConPTY host also remain unverified.
- Start Fresh is explicitly confirmed in the UI and rotates the persisted
  harness ID. A real, user-authorized provider-backed restore walkthrough was
  performed for both harnesses:
  - **Claude Code:** restoring a session that had never progressed past its
    first-run onboarding (no completed login) re-launched the wizard from the
    same theme-selection step, which is the harness's own expected behavior
    for a session with no real conversation state to resume — not a TauShell
    defect.
  - **GitHub Copilot CLI:** restoring a session that had been cleanly exited
    via `/exit` before any real prompt was sent invoked `copilot --resume=<id>`
    with TauShell's session UUID. The real `copilot.exe` rejected it
    (`Error: No session, task, or name matched '<uuid>'`) and printed its own
    usage guidance. TauShell correctly surfaced this as a failed session
    (`PowerShell exited with code 1`) rather than claiming success, matching
    the documented "do not claim restored unless harness confirms" principle.
    This is most likely because a session with zero real turns was never
    registered as resumable by Copilot CLI's own internal session tracking.
    Decision (user): Restore is now disabled for such sessions. The Copilot
    adapter's `CheckResume` requires
    `~\.copilot\session-state\<id>\events.jsonl` (present for sessions with
    turns, absent for zero-turn ones; undocumented Copilot internals, so it
    fails open if the session-state folder is missing). Home and the toolbar
    hide Restore entirely (the reason is the Start fresh tooltip) and keep
    Start fresh; the backend also refuses a restore with the same message.
    Covered by unit tests. The earlier smoke showed Restore disabled; the
    UI was later changed to hide it, and that change is build-verified only.
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
- Terminal sizing: xterm's `FitAddon.fit()` is a silent no-op until the first
  render measures a cell, which leaves `cols/rows` at 80x24. `launch()` awaits
  `fitTerminalNow` before `StartSession`, and the size is re-sent 0.5 s and 2 s
  after start because a resize during harness initialization can be lost.
  Padding for the terminal lives on `.xterm` (the fit addon does not subtract
  its parent's padding). The terminal has `scrollback: 0` and no scrollbar by
  design: scrolling belongs to the harness.
- Clipboard keys use the Wails runtime clipboard (native, no WebView2 prompt):
  Ctrl+C copies only with a selection, so Ctrl+C still interrupts the harness
  otherwise.
- Session `name` and `color` are edited in place in the YAML
  (`session.UpdateConfigField`, preserving comments); `id` and the file name
  never change. Recovery of a `running` runtime row checks the process
  creation time against the row's update time so a reused PID is not mistaken
  for a live session.
- Go-side confirmations (shutdown, forced-stop notice, tool installs) go
  through `App.messageDialog`, which shows an in-app modal via the
  `dialog-request` event and `AnswerDialog` (`cmd/taushell/dialog_windows.go`)
  once the page has called `DialogHostReady`. Before that, or if it never
  registers, it falls back to the native Wails message box. Only the
  pre-window WebView2 prompt and the startup error box stay native.
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
  pipe/supervisor cleanup. The bundled-ConPTY spike measured no handle growth
  over 10 start/close cycles with `OpenConsole.exe`, versus +16 with inbox
  ConPTY, so the residue is inbox console-host behavior; see
  [`02-conpty-integration.md`](02-conpty-integration.md#bundled-conpty).

## Model selection

The Create dialog has no Model field. A model can be set by adding
`--model <value>` to the editable launch arguments.

Running sessions have a **Model** menu in the terminal toolbar (and in the
command palette). It is shown only for harnesses that declare widgets and whose
`--model` capability probe succeeded. It types `/model <id>` into the harness,
or `/model` alone to open the harness's own picker, using the
[widget delivery policy](../v1/08-UI-Design.md#10-harness-widgets). TauShell
does not read the live model back, so the UI says "sent", never "applied". The
suggested IDs live in `internal/tools/tools.go` (`Widgets.Models`) and are only
suggestions: a custom ID can be entered, and availability depends on the
account.

Verified on Copilot CLI 1.0.104 through the real session manager and ConPTY
(rendered with pyte): `/model auto` and `/model claude-haiku-5.5` both changed
the model and the footer, and bare `/model` opened the picker. Claude Code's
`/model` was not exercised because its provider returns 401 on this PC.

## Harness widgets (implemented)

Terminal toolbar for running sessions: **Prompts**, **Commands**, **Model**,
**Compact**, **Interrupt**; plus a command palette (Ctrl+Shift+P) and shortcuts
Ctrl+1..9 (Nth session tab), Ctrl+Tab / Ctrl+Shift+Tab (cycle Home and tabs).
Prompt presets come from `~/.tau_shell/prompts.yaml` plus an optional
`presets:` list in the session YAML
([`18-yaml-schema.md`](18-yaml-schema.md)).

Evidence (Copilot CLI 1.0.104, real ConPTY, probe removed afterwards):
text followed by a separate Enter after 150 ms was accepted and answered
(`PRESETOK`); `/model` opened the picker and Esc closed it; **a single Esc did
not interrupt a working Copilot, two Escs 250 ms apart did** ("Operation
cancelled by user"), so Copilot's `InterruptKeys` are two Esc presses; `/compact`
sent while working is queued by the harness, which is why delivery is
insert-only unless the harness is idle. Not verified live: Claude Code's slash
commands, `/model`, and Esc interrupt (provider 401); its command list comes
from Claude Code's documented built-ins. The idle/input detection reuses the
attention cues, which are heuristic: when no cue is available the text is only
inserted.

Known limits: text is appended to whatever is already typed in the prompt;
there is no success confirmation from the harness; the frontend has no
automated tests, so the toolbar, palette, and shortcuts were verified by
build/type-check and the delivery path by the real-harness probe, not by
driving the packaged window.

## Recommended next actions

1. Done: the bundled-ConPTY spike passed and was adopted. `conpty.StartPreferred`
   uses `conpty.dll` + `OpenConsole.exe` from the executable directory (placed
   by `scripts/fetch-conpty.ps1`) and falls back to inbox ConPTY. Run the
   opt-in comparison with `TAUSHELL_CONPTY_SPIKE_DLL=<abs path to conpty.dll>
   go test -v -run TestConPTYBackendSpike ./internal/conpty`.
2. Done: real Claude Code and Copilot CLI walkthroughs (see Known gaps).
   Remaining manual checks: Copilot restore of a session with at least one
   real turn hosted in the app. **Postponed by the owner (2026-10-24):** Claude
   Code restore after a completed login, and the unverified Claude Code title
   cue (braille spinner = working, U+2733 = idle) used by the state icons. Do
   these when the owner logs in to Claude Code; they do not block later work.
   The full list of Claude Code gaps (G1-G12) and the test plan (T1-T9, to be
   run on another PC) is in
   [`24-claude-code-verification-plan.md`](24-claude-code-verification-plan.md).
   Test sessions `kbd-test-1` and `copilot-smoke-1` were removed from the dev
   profile with the new delete feature.
3. Done: Step 3 tool metadata/version ranges, capability probes, and
   confirmed winget installs are implemented (`internal/tools`,
   `cmd/taushell/tools_windows.go`, Home/Setup frontend). Done: Step 4
   session templates with argv preview (`Adapter.DefaultLaunchArgs()`,
   `GetSessionTemplate`/`PreviewSessionLaunch`); done: the Create dialog's
   "Edit session YAML" section (`PreviewSessionYAML`/`CreateSessionFromYAML`,
   strict `session.ParseConfig`) follows the form until hand-edited, then is
   validated and saved; the empty `powershell:` block is no longer written.
   A newly created session is now selected and started automatically. Does not depend on
   the terminal path.
   A clean-Windows-10/missing-WebView2 test will not be performed (user
   decision); registry-detection unit tests are the coverage.
4. Done: keyboard-only workflow and Wails-host close integration coverage.
   A mocked-browser smoke verified frontend Home/End and arrow-key tab
   selection, and a production-host smoke verified Home/Right switching after
   fixing tab-strip layout; native left/right overflow controls also scrolled
   eight inert fixture tabs. A zero-click production smoke confirmed window
   launch seeds tab-strip focus and a full keyboard walk can create a
   session; multi-tab keyboard switching was verified using the
   `terminal-fixture` binary as a harmless stand-in harness (no live
   credentials). Automated real-window `OnBeforeClose` CI coverage is an
   accepted limitation (see Known gaps) given current mocked-unit-test plus
   manual-smoke coverage.
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

Go is installed at `C:\Program Files\Go\bin` but is not on this machine's
default PATH; add it per shell (`$env:Path += ";C:\Program Files\Go\bin"`).
Do not hard-code machine-specific toolchain paths in project scripts; local
VS Code terminal settings may configure the installed toolchain path.

VS Code launch/tasks/settings in `.vscode/` are ignored workspace-local files,
not included in a fresh checkout (see
[`17-contributor-guide.md`](17-contributor-guide.md)). The icon generator is
tracked, but its PNG/ICO and `.syso` outputs are ignored; run it before building
to include the custom app icon.

## Quick start for the next session

- **Graceful stop:** `Manager.stop` first runs the adapter's `GracefulExit`
  input (Copilot CLI: Ctrl+C, `/exit`, Enter) and waits up to 8 s for the
  harness and `pwsh` to quit. If that is absent, fails, or times out, it falls
  back to a single Ctrl+C (3 s) and then force-termination, which is shown as
  "Graceful interrupt failed or timed out". Claude Code still uses the
  Ctrl+C-only path until a real probe shows `/exit` is safe for it (a real
  probe since showed Ctrl+C, `/exit`, Enter exits Claude in about 0.4 s).
  Unit tests
  use fake processes; the real Copilot `/exit` path was smoke-tested in the
  production window (clean exit in about 1.5 s, exit code 0, not forced). That
  a later Restore resumes the conversation is now verified: a real `Manager`
  run on Copilot CLI 1.0.104 (stop 2.2 s, not forced) restored the session,
  replayed the prior turn, and the model recalled a word given before the
  stop. Claude's model-dependent checks stay blocked: this PC's provider
  returns 401 on every request.
  Stop/shutdown
  timeouts were raised to 25 s to fit the longer sequence.

1. Read "Product boundaries", "Do not overstate current readiness", and
   "Known gaps" here, then [`22-local-attention-notifications.md`](22-local-attention-notifications.md)
   for the implemented (not live-verified) flash/toast alerts, shared session
   order (`session_order` in `state.db`), clone session, and
   `~/.tau_shell/config.yaml`.
2. Build and verify with the commands in the contributor guide. Check
   `git status` for the current workspace state; commit only when the owner
   asks.
3. Latest change: Copilot Restore gating (`Adapter.CheckResume`,
   `Snapshot.ResumeNote`; Restore disabled with Start fresh when Copilot has no
   `events.jsonl` for the session). It relies on Copilot's undocumented
   session-store layout and fails open if the store is missing.
