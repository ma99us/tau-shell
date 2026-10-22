# Phase 1 Desktop UI Implementation Plan

**Status:** In progress; the WebView2/xterm.js feasibility gate has not passed. Implements
[08-UI-Design.md](./08-UI-Design.md).

Build a Windows desktop application around the existing Go session manager and
PowerShell/ConPTY host. The GUI is the primary interactive interface; the
`list`, `start`, and `restore` CLI commands remain for scripting and recovery.
The existing Bubble Tea interactive TUI is a disposable prototype, not a
supported frontend to preserve. Replace it with the desktop GUI; do not ship
or maintain both interactive interfaces. Reuse only backend/process behavior
and tests demonstrated to be UI-independent and correct for the GUI.

## 1. Agreed decisions

- **Application:** local Windows desktop GUI; not a TauShell TUI.
- **Frontend migration:** replace and remove the current interactive TUI; do
  not retain it as a fallback or parallel supported experience. Keep the
  standalone CLI commands.
- **Host/runtime:** Go remains responsible for session processes, adapters,
  persistence, and shutdown.
- **Presentation:** local web UI hosted in Windows WebView2. React is the
  preferred UI candidate; verify the Go-to-WebView2 host choice in a spike.
- **Embedded terminal:** xterm.js is the preferred candidate; verify full-screen
  Claude Code and Copilot CLI behavior before adoption.
- **Runtime prerequisite:** use installed Microsoft Edge Evergreen WebView2
  Runtime and explain how to install it if missing.
- **Setup:** detect tools at startup. Missing WebView2/PowerShell 7 gets a
  first-run prerequisite flow; missing harnesses and optional tools remain
  installable from Home without blocking the app.
- **Install consent:** show package/source and require confirmation before
  invoking a known winget package; use official vendor links if winget is
  unavailable.
- **Version policy:** use installed versions, maintain tested ranges and
  minimums per tool, probe capabilities, warn outside tested ranges, and never
  silently pin, downgrade, or auto-update.
- **Session creation:** choose a supported harness and work folder; generate
  editable YAML with ordered, harness-specific command-line argument defaults.
  The Claude template may include `--dangerously-skip-permissions`; the Copilot
  CLI template may include `--yolo`. Saved YAML is the explicit choice and
  requires no extra launch warning.
- **Arguments:** one ordered `launch_args` list applies to both start and
  restore; adapters add their mode-specific TauShell-managed session flags.
- **Startup:** show a session-launcher/home screen and make prior tabs
  available; never automatically launch/restore harnesses.
- **Concurrency:** sessions in background tabs keep running and their terminal
  screens keep updating.
- **Start fresh:** replacing an existing saved harness ID requires
  confirmation.
- **Close tab:** ask whether to stop the session or leave it running in the
  background.
- **Shutdown:** confirm when sessions are running, then gracefully interrupt
  and force-terminate stragglers with visible reporting.
- **Scope:** launcher, tabs, embedded terminal, status, and safe shutdown first.
  Prompt presets and model/command widgets are later work.
- **Future presets:** reusable app-level definitions with optional per-session
  overrides; schema is deferred until that feature is designed.

## 2. Delivery phases

### Current implementation status

- **Step 1 (feasibility): open.** A production-tagged Wails host opens with
  bundled React/xterm.js assets on the current Windows 10 development machine.
  A production-mode WebView2 smoke confirmed that navigation to an external
  `.invalid` origin is cancelled. The checked-in `cmd/terminal-fixture/`
  harness has now run through the production Wails window, bridge, PowerShell
  7, and ConPTY. The real hosted xterm rendered Unicode/emoji (including a
  four-byte sequence split across writes 50 ms apart), ANSI colors, and cursor
  movement/erase; it replied to a cursor-position query, accepted typed input,
  and reflected live resize. The fixture's explicit `q` exit restored the
  preserved primary-screen marker. A repeated run confirmed the cursor-position
  reply (`ESC[10;1R`). In a later isolated run, the post-start device-status
  reply was recognized and successfully written by the Go bridge, but did not
  arrive in any fixture input chunk. A post-start cursor query produced a
  fixture-visible seven-byte reply (`ESC[6;15R`) without a corresponding
  bridge marker. Native `pointerdown`/`mousedown` events reach the terminal
  container, but xterm remains in `none` mouse mode and emits no report.
  A direct ConPTY probe showed mouse-mode DECSET controls are consumed before
  reaching xterm; a temporary xterm-side mode enable forwarded SGR reports to
  the Go bridge. An isolated browser-side xterm check also emitted the
  expected device-status and SGR mouse responses.
  Missing-runtime behavior and Claude Code/Copilot CLI hosted-terminal
  walkthroughs remain unverified.
- **Step 2 (host and bridge): partial.** The no-argument Windows entry point,
  bundled local asset server, WebView2 registry precheck, typed bound methods,
  standalone CLI path, and top-level local-origin navigation guard exist.
  The external navigation denial is smoke-verified; untrusted-origin bridge
  dispatch has not been separately exercised.
- **Step 3 (discovery/install): partial.** Home displays executable paths and
  `--version` output for PowerShell 7, Claude Code, and Copilot CLI, making
  PATH precedence visible. Minimum/tested ranges, capability probes, setup
  flows, winget consent, and installation are not implemented.
- **Step 4 (session manager): foundation implemented.** Process lifecycle has
  been extracted from the TUI, and start/restore/fresh, input, resize, output
  events, and bounded stop operations are wired. Windows fake-process tests
  cover concurrent independent I/O/resize, process exit, restore failure,
  graceful/forced shutdown, cleanup, supervisor and pipe-handle completion,
  and retry after stop or database-close failure. Recovery tests cover an
  abandoned startup, recent starting state, running records with missing or
  exited process IDs, and a live session owned by another window. App-close
  error handling now keeps the window open when session checks or
  forced-shutdown reporting fail. Process tests verify
  explicit ConPTY resource clearing and idempotent close. A repeated native
  check confirms process, job, and input/output pipe handles are invalidated
  after close; the opaque pseudoconsole handle remains covered by successful
  `ClosePseudoConsole` and cleared state. Wails-host close integration and a
  comprehensive native handle-leak check remain open.
- **Step 5 (launcher/tabs): partial.** Home, session creation, tabs, terminal
  panes, and stop-or-background tab close are implemented. Tab IDs and
  selection are stored in WebView local storage and are not auto-launched.
  Choosing to keep a live session in the background keeps its xterm instance
  mounted and reuses its screen when reopened; a browser interaction smoke
  with a mocked bridge verified that output received while hidden appears on
  reopen and input routes to the reopened session. The actual Wails/ConPTY path
  remains unverified. Tabs now use keyboard-accessible tab roles, arrow/Home/
  End selection, active-tab visibility, and horizontal overflow buttons.
  A mocked-browser keyboard smoke verified Home/End selection, arrow-key
  wraparound, focus movement, and synchronized selected-tab state. A complete
  production-host smoke found that the tab strip was missing its flex layout;
  after the CSS fix, Home and Right switched focus and selection between Home
  and one stopped fixture tab. A mouse click seeded tab focus, so this does not
  count as a complete keyboard-only walkthrough. With eight inert fixture tabs
  open, the native left/right overflow controls scrolled between earlier and
  later tabs. The session dialog now suggests the five most recently created
  project folders using WebView local storage. YAML preview/editing, generated
  harness defaults, and persisted-tab restoration under a real WebView profile
  remain missing.
- **Step 6 (persistence/shutdown): partial.** Tab metadata persists separately
  from YAML/runtime state. App close confirms before stopping, and stop results
  that report forced termination receive an additional native notice. Manager
  tests verify stale running state with missing or exited process IDs and
  abandoned startup are marked failed without losing their resume IDs, while a
  recent starting state is retained and a live process in another window
  remains running but is not marked local. Manager tests cover
  retrying after stop timeouts and database-close errors; close-path tests
  verify the window remains open when session listing fails and a forced-stop
  notice is retried after its native dialog fails. Additional close-flow unit
  tests verify cancellation leaves sessions untouched, confirmed shutdown
  stops and rechecks sessions, forced outcomes are reported, and failures to
  show the confirmation or stop sessions keep the window open. A session still
  running after StopAll also prevents close. Broader restart
  reconciliation, automated Wails-host close integration, and real WebView
  profile restoration remain outstanding. The Wails `OnBeforeClose` callback
  now returns immediately and performs confirmation and session stopping
  asynchronously. A production-window smoke with an inert live fixture
  verified the native prompt remained responsive: choosing No kept TauShell
  and the fixture running; choosing Yes stopped the fixture process and closed
  TauShell. The fixture's PowerShell process exited with code 1 during
  interruption, so this does not verify graceful-stop success or the
  forced-termination notice path.
  Detached shell/broker processes that escape TauShell's job are intentionally
  unmanaged.

### Step 1 — WebView2 host and embedded-terminal feasibility

**Depends on:** none.

- Evaluate maintained Go WebView2 host libraries for Windows 10 support,
  Evergreen runtime detection, DPI/window behavior, keyboard focus, window
  close events, and packaging/maintenance health.
- Build a disposable local host that loads bundled web assets, proves a
  restricted Go↔UI bridge, and shows a terminal component hosted by the
  WebView.
- Evaluate xterm.js or alternatives with a fake interactive TUI over the
  existing ConPTY path. The checked-in terminal fixture verifies Unicode/
  emoji, ANSI colors, cursor movement/erase, cursor-position query replies,
  typed input, live resize, explicit exit, and split UTF-8 output through the
  actual hosted path. The explicit exit restored the preserved primary-screen
  marker. Device-status input reaches the Go bridge but is not observed by the
  fixture; mouse input is not observed at the bridge or fixture. Payload-free
  diagnostics now confirm native pointer events reach the terminal container,
  while xterm remains in `none` mouse-tracking mode. A direct ConPTY probe
  showed that Windows ConPTY consumes `CSI ? 1000 h` and `CSI ? 1006 h` before
  they reach xterm; a temporary xterm-side mode-enable probe forwarded SGR
  mouse input to the Go bridge. Do not force mouse tracking unconditionally:
  it would change normal terminal selection and input behavior. The current
  PowerShell/ConPTY boundary needs an explicit design decision to support
  harness-requested mouse modes. Device-status delivery and the cursor-report
  source remain unresolved, and both supported real harnesses remain
  unverified.
- **Bundled ConPTY spike (decided):** evaluate shipping a newer
  `conpty.dll` + `OpenConsole.exe` (Windows Terminal /
  `Microsoft.Windows.Console.ConPTY` package) behind a loader switch in
  `internal/conpty`, keeping inbox kernel32 ConPTY as the fallback. Done when
  the spike records, for both inbox and bundled ConPTY: (a) whether
  `CSI ? 1000 h`/`CSI ? 1006 h` reach xterm and SGR mouse input reaches the
  fixture; (b) whether device-status/cursor-position replies are answered by
  ConPTY itself or delivered to the fixture; (c) whether the per-cycle
  process handle to the exited console host remains; plus license (MIT),
  Windows 10 support, and packaging/signing implications. Adopt the bundled
  ConPTY only if it passes mouse modes through; otherwise record the evidence
  and return for a mouse-support decision. Do not force xterm mouse tracking.
- Verify how a missing WebView2 runtime is detected and handed off before the
  WebView can render. The bootstrap path must use a minimal native Windows
  surface or an equivalent independently available mechanism. **Decided:**
  registry-detection unit tests cover Phase 1 automation; the native prompt
  and download handoff are deferred to a manual Windows Sandbox/VM check and
  are not claimed verified until then.
- Exercise Claude Code and Copilot CLI terminal screens without sending
  prompts that modify user data. Test on supported Windows 10 environments.
  The user has authorized non-destructive walkthroughs using their existing
  signed-in installations: start, a short read-only prompt, stop, and
  restore/resume, recording executable path, version, and observed outcome.
- Verify runtime-missing behavior, app asset loading, local-only navigation,
  license/maintenance acceptability, and an install/build path.
- Record the selected Go host and terminal component plus tested limitations
  in the UI design document. If the prototype cannot reliably host both
  harnesses, stop and return with evidence and alternatives before committing
  to a framework.
- This feasibility spike is for the WebView2/embedded-terminal target only.
  Do not invest further in the existing Bubble Tea UI or its terminal
  rendering/input stack.

**Exit criteria:** a Windows 10 prototype displays both target harness TUIs in
an embedded terminal without corrupting output, routes input and resize
correctly, safely detects the WebView2 runtime prerequisite, and communicates
with Go through a restricted bridge.

### Step 2 — Desktop application shell and secure bridge

**Depends on:** Step 1.

- Establish the minimal Go desktop host and build pipeline for bundled local UI
  assets. Avoid an unrestricted local HTTP server unless the selected host
  requires one and its binding/security model is explicitly reviewed.
- Build the initial Home/session-launcher view, app window lifecycle, and
  error/empty states. Show tool inventory and missing-tool actions without
  requiring an installed harness to open the app.
- Define typed bridge messages for listing configured sessions, reading
  sanitized runtime status, starting/restoring/stopping, terminal input,
  terminal output/events, resize, and user confirmation.
- Restrict bridge access to trusted bundled UI origins; deny unexpected
  navigation and external content. Keep environment values, credentials, and
  arbitrary process controls out of renderer state.
- Add clear WebView2 missing-runtime instructions and startup diagnostics.
- Create a per-process structured diagnostic log under the user's system temp
  directory, truncate it on each launch, and capture host, bridge, and frontend
  failures without recording prompts, terminal input/output, credentials, or
  environment values.
- Keep `list`, `start`, and `restore` independent of the GUI host.
- Once implemented, make no-argument interactive launch open the desktop app.
  If no interactive desktop or required GUI host is available, report a clear
  error; do not silently fall back to the obsolete TUI. Keep the three CLI
  commands usable without WebView2.

**Exit criteria:** Home launches reliably, bridge requests are validated, GUI
errors are actionable, CLI commands still operate, and no remote content can
invoke privileged bridge operations.

### Step 3 — Dependency discovery, version support, and installation

**Depends on:** Step 2. **Decided:** proceeds in parallel with the Step 1
feasibility gate because detection, metadata, and confirmed-install flows do
not depend on the terminal path. The Step 1 bootstrap item still governs the
missing-WebView2 surface.

- Define a metadata contract for host prerequisites, harnesses, and optional
  development/runtime tools: executable detection, version detection,
  minimum-required version, tested-version range, and required adapter
  capabilities.
- Detect WebView2, PowerShell 7, Claude Code, Copilot CLI, and declared
  tool-specific dependencies without blocking Home when a harness is missing.
- Classify Node.js, Git, Python, and other candidates by actual adapter or
  feature requirements. Treat winget as an optional installer backend, not a
  runtime prerequisite.
- Implement a startup setup screen for missing WebView2/PowerShell 7. If
  WebView2 itself is missing, present its setup action using the independent
  bootstrap surface identified in Step 1.
- Add per-tool Install actions using explicit maintained package IDs and
  sources. Show the package/source and request confirmation before invoking
  winget. When winget is absent, show official vendor setup links.
- Never silently elevate, invoke arbitrary scripts, or claim success before
  re-detection confirms the executable/version and adapter probes.
- Warn for versions outside the tested range but allow use when required
  capabilities remain available. Surface unavailable capabilities and
  regressions explicitly. Do not automatically pin, downgrade, or update.

**Exit criteria:** unit/integration tests cover detection results, version
range reporting, capability probes, user cancellation, installer failure,
missing winget fallback, and post-install verification.

### Step 4 — Session manager and terminal lifecycle integration

**Depends on:** Steps 1, 2, and 3.

- Expose managed-session lifecycle operations through a UI-independent Go
  session manager rather than putting process logic in WebView handlers.
- Reuse strict session YAML validation, the existing SQLite runtime store,
  and tool-specific harness adapters.
- Extend the strict YAML schema with one ordered `launch_args` list used for
  both start and restore. Keep TauShell-owned session/resume identifiers in
  adapter-generated arguments; pass configured values as discrete argv
  entries, never as shell text.
- Generate per-harness session templates with editable argument defaults,
  including the agreed Claude Code and Copilot CLI examples. **Decided:**
  template generation, argv preview, and YAML validation may proceed in
  parallel with the Step 1 feasibility gate; they do not depend on the
  terminal path. Preserve those
  values in the user-edited YAML and preview the effective executable and
  discrete argument list before saving. Secrets must not be entered as
  arguments.
- Probe whether the installed harness supports each required adapter
  operation/option. Report unsupported arguments and launch failures; do not
  allow configured values to override TauShell's managed session identity.
- Support multiple configured session IDs concurrently and reject duplicate
  live instances of the same session.
- Run one PowerShell/ConPTY and terminal-emulator screen per live session.
  Drain output independently, including for inactive tabs, and preserve
  terminal query/response behavior.
- Route user input only to the selected terminal. Track each viewport's
  dimensions and resize the emulator and ConPTY consistently.
- Keep start, restore, and fresh start distinct. Persist a replacement resume
  ID only after explicit fresh-start confirmation.
- Preserve graceful interrupt, bounded force termination, process-tree
  cleanup, and explicit error reporting.

**Exit criteria:** Windows automated tests demonstrate at least two fake
harness sessions with independent output/input/screen state, inactive output
draining, resize, normal exit, restore failure, graceful stop, forced process
tree cleanup, and no leaked handles/goroutines.

### Step 5 — Launcher, tabs, and terminal workspace

**Depends on:** Steps 2, 3, and 4.

- Implement Home with configured sessions, identifying details, and verified
  runtime state and detected tool availability. Restore prior tab
  arrangement/selection as available launcher state, not as automatically
  running sessions. Provide **New session** with harness selection, folder
  picker/recent folder suggestions, generated editable config preview, and
  creation; offer to start the session immediately after it is saved. Recent
  folder suggestions are implemented and limited to the five most recently
  created session folders.
- Show a clear setup path when host prerequisites are missing, and direct
  Install actions for missing harnesses and recommended tools.
- Implement a tabbed workspace with an embedded terminal for each running
  session, keyboard-accessible session selection, tab overflow handling, and
  clear active/inactive status. The current UI exposes tab semantics, arrow
  and Home/End selection, and horizontal scroll controls; verify these in the
  desktop keyboard-only walkthrough.
- Start never-used sessions explicitly; offer Restore or confirmed Start fresh
  when a resume ID exists. Selecting an already-running session focuses its
  existing tab.
- Closing a live tab offers stop or leave running in background. Background
  sessions remain listed and can be reopened without launching again.
- Provide useful states for no configured sessions, no open tabs, missing
  harness executable, missing working directory, failed launch/restore, and
  stopped sessions.
- Make the active harness terminal fully usable, including terminal keyboard
  and mouse behavior supported by the selected component.

**Exit criteria:** keyboard-only walkthrough covers first start, tab switching,
restore, fresh-start confirmation, background retention/reopen, close, error
recovery, and terminal interaction without sending keys to the wrong session.

### Step 6 — Persistence, shutdown, and migration from the TUI prototype

**Depends on:** Step 5.

- Persist open tab IDs, ordering, and last selection separately from YAML and
  harness runtime fields. Reconcile stale process IDs at startup.
- Startup opens Home and makes prior tab information available; it does not
  start or resume sessions automatically.
- When quitting with live sessions, confirm first; on approval, gracefully
  interrupt all managed sessions, wait briefly, then force-terminate
  stragglers. Report each forced outcome.
- Preserve non-interactive CLI commands and their exit/error semantics.
- Inventory the current implementation and classify each component as
  **Keep**, **Refactor**, or **Delete**:
  - Keep/retest UI-independent ConPTY/process-tree lifecycle, session YAML and
    SQLite runtime store, harness adapters, and CLI command behavior.
  - Refactor session orchestration currently coupled to the TUI so the Go
    session manager exposes UI-independent lifecycle, terminal I/O, resize,
    and status operations to the desktop bridge.
  - Delete the Bubble Tea interactive screen, TUI-only key handling/layout,
    in-process TUI VT renderer, and tests that only assert that frontend.
- In the current tree, explicitly review
  `cmd/taushell/tabs_windows.go` and `cmd/taushell/tabs_windows_test.go`.
  Extract reusable logic and behavior tests before deleting or replacing those
  files; do not carry their Bubble Tea model into the desktop application.
- Remove Bubble Tea and TUI-only terminal-rendering/input modules from
  `go.mod`/`go.sum` after replacement, but only when dependency analysis and
  tests confirm they are no longer used. Do not remove shared ConPTY or
  process-management code just because the TUI used it.
- Replace no-argument interactive startup with the desktop app. Do not retain
  the old TUI as a supported mode, fallback, feature flag, or second
  interactive frontend. Keep `list`, `start`, and `restore` CLI commands.
- Update README and V2 implementation-status documents only to describe
  behavior that is actually shipped and tested.

**Exit criteria:** restart restores Home/tab metadata without auto-launch;
confirmed app quit leaves no managed process trees; CLI recovery remains
available; no Bubble Tea/TUI frontend remains in the shipped application;
unused TUI dependencies are removed; the desktop GUI is the sole interactive
app experience.

## 3. Later UI extension: prompts and harness controls

This is explicitly not a prerequisite for the first GUI release.

- Design reusable application-level prompt presets with optional per-session
  overrides; decide the file/schema and validation then.
- Extend adapters with declared capabilities and typed, result-reporting
  operations before adding model selectors or harness command widgets.
- Do not silently send text to a terminal to simulate a control. A widget must
  call a supported adapter operation, report errors, and leave the embedded
  terminal usable as the universal fallback.
- Test each capability against supported harness versions and clearly hide or
  disable unavailable controls.
- Maintain adapter-specific supported/tested version ranges and regression
  checks as upstream harness releases evolve. Prefer capability probes over
  brittle exact-version checks, but retain minimum-version gates when a
  necessary operation is absent.

## 4. Cross-cutting requirements

- Keep PowerShell 7 and ConPTY as the session runtime; the GUI is not a shell.
- Preserve session-scoped environment injection. Do not expose secret values in
  the renderer, logs, or UI persistence.
- Keep harness conversation restore distinct from recreating PowerShell or a
  terminal buffer.
- Keep the session manager and adapters independent of React, WebView2, and
  terminal widget implementation details.
- Validate Windows 10, UTF-8, Unicode/emoji, ANSI, mouse/input, resize, and
  process cleanup end to end.
- Do not claim that the selected GUI host or terminal component is verified
  until Step 1 exit criteria pass.

## 5. Risks and gates

| Risk | Response |
|---|---|
| Go WebView2 host is fragile or poorly maintained | Evaluate concrete host libraries in Step 1; return to the user before changing languages or adopting an unmaintained wrapper. |
| Web terminal differs from real Windows terminals | Test target harness TUIs on Windows 10, including alternate screen, input modes, mouse, resize, Unicode, and terminal queries. |
| WebView bridge creates a privileged renderer boundary | Bundle assets locally, validate every message, restrict origins/navigation, and never expose arbitrary process or filesystem APIs. |
| Runtime missing on a supported machine | Detect at launch and provide explicit setup guidance; do not silently install. |
| Hidden sessions stop receiving output or have stale dimensions | Drain all outputs and keep screen models/dimensions synchronized while inactive. |
| UI widgets depend on brittle simulated typing | Require explicit adapter capabilities and typed operations; defer the widget rather than automate fragile keystrokes. |
| TUI-only code is accidentally carried into the desktop app | Require the Step 6 Keep/Refactor/Delete inventory; retain only verified UI-independent behavior and tests. |

## 6. Completion checklist

- [ ] Go/WebView2 host and terminal component pass the Windows 10 feasibility
      gate with Claude Code and Copilot CLI.
- [ ] Startup detects WebView2, PowerShell 7, supported harnesses, versions,
      and declared tool dependencies.
- [ ] Missing host prerequisites lead to a usable setup path; missing
      harnesses do not block Home.
- [ ] Installs require confirmation, use known package/source identifiers, and
      are re-verified after completion.
- [ ] Compatibility ranges and capability probes report untested versions and
      regressions without silent pinning/downgrades.
- [ ] New-session flow chooses a work folder and harness and writes editable
      YAML with the selected template's ordered launch arguments.
- [ ] Local UI bridge is narrow, validated, and restricted to trusted assets.
- [ ] Home/session launcher and tabbed embedded-terminal workspace are usable.
- [ ] Multiple sessions run concurrently with correct isolated terminal state.
- [ ] Start, restore, confirmed fresh start, close/background, and shutdown
      semantics are tested.
- [ ] Startup restores launcher/tab metadata without starting harnesses.
- [ ] `list`, `start`, and `restore` remain usable without the desktop UI.
- [ ] No-argument launch opens the desktop GUI; there is no TUI fallback or
      second interactive frontend.
- [ ] Bubble Tea and TUI-only rendering/input dependencies are removed unless
      dependency analysis proves a separate non-UI use.
- [ ] UTF-8, Unicode/emoji, ANSI, resize, input routing, and child cleanup pass
      on Windows.
- [ ] README and active product docs match the shipped implementation.
