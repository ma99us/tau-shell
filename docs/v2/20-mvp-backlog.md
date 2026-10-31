# MVP Backlog

## Phase 1: local Windows desktop GUI (TUI removed)

- Completed migration: the Bubble Tea TUI and its TUI-only dependencies were removed. `list`, `start <session-id>`, and explicit `restore <session-id>` remain available as CLI operations; no TUI fallback is planned.
- In progress: a Go/Wails WebView2 host, bundled React/xterm.js launcher, local tabs, session creation, tool inventory, session lifecycle bridge, and shutdown confirmation are implemented. A user-authorized, live-credential walkthrough exercised both supported harnesses (Claude Code, GitHub Copilot CLI) through the real hosted terminal on this Windows 10 machine; a clean-environment test without WebView2 will not be performed (user decision); the missing-runtime prompt is accepted as working. See [`docs/v1/08-UI-Design.md`](../v1/08-UI-Design.md) and [`docs/v1/09-UI-Implementation-Plan.md`](../v1/09-UI-Implementation-Plan.md).
- Security hardening: the Windows host now cancels top-level WebView2 navigations outside the bundled `http://wails.localhost` origin before they load. Wails' bound-method origin validation remains a separate defense.
- Session-manager lifecycle coverage is implemented for concurrent independent fake sessions, input/output/resize, process exit, failed restore preserving the resume ID, graceful and forced stop, and cleanup. This does not cover a real hosted xterm or harness.
- Implemented executable detection, resolved version parsing, minimum/tested-version classification, and per-adapter capability probes (via `--help` option matching) for PowerShell 7 and supported harnesses; Home shows each tool's resolved executable path, version, support classification, and any unmet capability. Optional Node.js/npm and Git for Windows are also detected in the Tools panel; their absence never gates Home. Node reports npm as a required companion. A first-run Setup screen still gates only on missing/unusable PowerShell 7, with a "continue without" escape hatch. Do not silently pin or downgrade.
- Implemented schema: strict YAML accepts ordered harness `launch_args`; managed session/resume arguments cannot be overridden. Implemented generated per-harness launch-argument templates (Claude Code → `--dangerously-skip-permissions`, Copilot CLI → `--yolo`, via `Adapter.DefaultLaunchArgs()`), prefilled editably in the New Session dialog without overwriting user edits, plus a live launch preview (`PreviewSessionLaunch`) showing the resolved executable and full effective argv and warning on options the installed harness does not support.
- **Tool setup:** WebView2 and PowerShell 7 remain host prerequisites; missing harnesses, Node/npm, or Git do not block Home. Winget installs still require confirmation and run in a visible console. Node uses the current supported LTS Windows ZIP after verifying Node's signed SHA-256 manifest; Git uses the full PortableGit archive after verifying the GitHub release digest. Both install under `%LOCALAPPDATA%\TauShell\tools`, require no admin rights, and only affect TauShell-launched processes. Existing PATH tools are used unless a TauShell-managed copy exists, which takes precedence. npm-backed harness installs ask to install Node first when Node/npm is unusable, then ask separately before installing the harness. No automatic updates or registry PATH writes.
- ConPTY host for PowerShell 7 on Windows 10, with automated Windows tests for output, exit codes, resizing, and child-process cleanup.
- Implemented per-run JSON diagnostics at `%TEMP%\TauShell\taushell.log`; startup truncates the file and the UI exposes its location. Host/lifecycle, Wails, frontend runtime, React render, and operation failures are logged with sensitive payload boundaries. Prompt/terminal contents, credentials, and environment values are intentionally excluded.
- Session registry and human-edited, per-session YAML definitions; keep mutable runtime state separate.
- Start, interact with, stop, and explicitly restore TauShell-launched sessions.
- Harness-specific launch/resume command construction for Claude Code and GitHub Copilot CLI uses explicit session IDs. A disposable Copilot CLI 1.0.100-1 provider-backed test confirmed conversation recovery via TauShell `restore`. A later live hosted-terminal walkthrough (real, user-authorized credentials) restoring a session that had zero real conversation turns found Copilot CLI rejects `--resume=<uuid>` (`Error: No session, task, or name matched`), and TauShell correctly reports this as a failed restore rather than a false success; most likely because Copilot CLI never registers a turn-less session as resumable. Restore is now disabled for Copilot sessions without `events.jsonl` in Copilot's session store (fails open if the store is absent); Start fresh remains available. Claude Code's restore re-renders its first-run wizard when the original session never completed login, which is expected; a real completed-login Claude Code restore remains unverified.
- Confirm before application shutdown when sessions are running; keep the Wails close callback nonblocking while confirmation and stopping run asynchronously. Attempt graceful interruption, then force-terminate any remaining processes in TauShell's job after a brief wait. A production-window smoke with an inert fixture verified that No leaves the app and session running and Yes closes the app after the fixture process exits; the fixture's PowerShell process exited with code 1; graceful-stop success and forced-termination reporting are now covered by real-ConPTY manager tests rather than that fixture.
- Automated session-manager tests cover concurrent session I/O/resize, process exit, failed restore, graceful/forced stop, and inherited child cleanup. Real-ConPTY manager tests (`manager_conpty_windows_test.go`) additionally confirm that a harness exiting on its graceful-exit input is reported as a clean stop, and that a process ignoring the interrupt is force-terminated with the visible "force-terminated" notice persisted in runtime state. Runtime recovery also treats a recorded PID whose process was created after the runtime row was marked running as a reused PID and fails the stale session (`session.RuntimeProcessAlive`). Shutdown tests also cover supervisor completion, idempotent close, rejecting starts after shutdown, retry after stop timeout or database-close failure, and keeping the window open when session checks or forced-stop reporting fail. Detached shell/broker launches such as PowerShell `Start-Process` can escape the ConPTY job; by design, those processes are unmanaged and are not subject to ancestry-based termination.
- Windows 10 terminal compatibility: CLI and hosted sessions temporarily use UTF-8 console code pages and restore prior settings; session hosting also enables/restores virtual-terminal output mode. ConPTY tests cover supplementary-plane emoji input, Unicode/emoji output bytes, ANSI output, resizing, and process cleanup. The desktop embedded-terminal path needs separate end-to-end verification; TauShell must not rewrite arbitrary harness output.

### Open Phase 1 backlog

- **Feasibility gate:** a checked-in deterministic fixture has verified
  Unicode/emoji, ANSI colors, cursor movement/erase, cursor-position query
  replies, split UTF-8 output, typed input, explicit exit, and live resize
  through the production WebView2, xterm.js, PowerShell 7, and ConPTY path.
  The fixture's explicit exit restored the preserved primary-screen marker.
  Inbox Windows 10 ConPTY consumed mouse-mode requests, answered cursor
  reports itself, and swallowed device-status replies. The bundled-ConPTY
  spike passed: `Microsoft.Windows.Console.ConPTY` forwards all of these, and
  a production smoke delivered real WebView clicks and the device-status reply
  to the fixture. TauShell now prefers the bundled ConPTY
  (`scripts/fetch-conpty.ps1`) and falls back to inbox. Done: user-authorized
  non-destructive Claude Code and Copilot CLI hosted-terminal walkthroughs
  (Claude restore after a completed login remains open; see
  [`24-claude-code-verification-plan.md`](./24-claude-code-verification-plan.md)).
- **Host prerequisite:** registry-detection unit tests cover Phase 1; the
  missing-WebView2 native prompt and official download handoff are deferred to
  a manual Windows Sandbox/VM check (the current machine has WebView2).
- **Tool discovery/setup** and **session authoring** proceed in parallel with
  the feasibility gate (decided); they do not depend on the terminal path.
- **Tool discovery/setup: done.** `internal/tools` declares minimum/tested
  version ranges and per-adapter capability lists, probes installed versions
  and `--help` output, and `cmd/taushell` wires confirmed winget installs
  (visible console, re-detection-gated success) with official vendor-link
  fallbacks when winget is absent or a tool has no package. Verified via unit
  tests and a production Wails smoke of the Home/Setup rendering; a live
  winget install run itself was not exercised.
- **Optional Node/Git portable setup: implemented.** The app detects Node plus
  npm and Git for Windows, uses existing PATH installs when no managed copy is
  active, and prefers managed copies thereafter. Confirmed per-user installers
  use the official Node current-LTS ZIP (Node 26.13.0) with Node.js release-key
  signature verification and the full Git for Windows PortableGit archive
  (2.56.0.windows.4) with GitHub release SHA-256 verification. A non-elevated
  Windows smoke in a disposable path containing spaces and Unicode verified
  both installs, a local npm CLI shim through PowerShell 7, and `git init`;
  machine and user PATH registry values remained unchanged. This verifies those
  exact artifacts and paths, not all Node/Git releases or npm packages. Node
  below 22 is below minimum; newer Node/Git versions remain untested until
  tested ranges are established. Missing Node for npm harness installation
  prompts separately for Node and then the harness.
- **Session authoring:** generate harness-specific editable YAML defaults,
  preview and validate effective discrete argv before saving, and offer to
  start after creation.
- **Workspace UX:** verify recent folder suggestions and keyboard-only tab
  navigation/overflow controls in the desktop host, persisted-tab restoration
  across app restarts, and background terminal behavior through the actual
  Wails/ConPTY path. A mocked-browser smoke verified Home/End selection,
  arrow-key wraparound, focus movement, and synchronized selected-tab state;
  a production-host smoke found and fixed the missing tab-strip flex layout,
  then verified Home/Right focus and selection switching between Home and one
  stopped fixture tab. With eight inert fixture tabs open, native left/right
  controls scrolled through the overflow. A complete keyboard-only
  walkthrough (zero mouse clicks) is now verified: window launch seeds
  tab-strip focus, `Tab`/`Enter`/typed text can create a session end to end,
  and multi-tab switching between Home and a running session tab works via
  the left-sidebar list entries, verified with the harmless `terminal-fixture`
  binary standing in for a real harness. Basic accessible tab
  navigation and horizontal overflow controls are implemented in the frontend.
  The session form suggests up to five recently used folders, persisted in
  WebView local storage.
- **UI polish (implemented):** Refresh button on Home; New Session dialog with
  side-by-side harness toggle buttons (official Simple Icons marks, Copilot
  first and default); session tiles use project-initial icons tinted with the
  session color; Home, sidebar and tabs share one session order; Home is a
  house icon in the tab strip and in the expanded and collapsed sidebar.
- **Harness widgets (implemented):** terminal toolbar (Prompts, Commands,
  Model, Compact, Interrupt), command palette (Ctrl+Shift+P), tab shortcuts
  (Ctrl+1..9, Ctrl+Tab), prompts in `~/.tau_shell/prompts.yaml`. Delivery
  rules and verification limits: [`08-UI-Design.md`](../v1/08-UI-Design.md#10-harness-widgets)
  and the handoff. Claude Code slash commands, `/model` and Esc interrupt are
  unverified (provider 401). No model field in the New Session dialog.
- **UI improvements (implemented, not live-driven):**
  - Drag-and-drop reordering from the left menu (expanded and collapsed), the
    tab strip and Home tiles. One order is stored in `state.db`
    (`session_order`) and shared by all three; it is not derived from file
    names. No keyboard reorder yet (accessibility gap). Drag was verified by a
    Go test of the order store/manager and a type-checked build; the packaged
    window could not be driven.
  - The session top bar shows the harness's current model right of the git
    branch. It is read-only, from the harness's own log (Claude Code
    `projects/*/<id>.jsonl`, Copilot CLI `session-state/<id>/events.jsonl`),
    polled every 5 s; empty when unknown. Copilot verified against a real log,
    Claude against a prior transcript; not yet seen live in the window.
  - Home tile buttons are green and ordered Start fresh / Open (or Start),
    Resume, Clone, pinned to the tile bottom. "Open terminal" is now "Open".
    A gear button in the sidebar footer opens `config.yaml`; the collapse
    toggle sits at the right of the footer.
  - Clone from the right-click menu (sidebar row or tab) and a Home tile
    button: the copy reuses all definition fields, gets a unique "Name (n)"
    and id, sits right after the source, and starts immediately. Runtime
    state is never copied. YAML comments are not preserved in the copy.
  - Local attention alerts: see
    [`22-local-attention-notifications.md`](./22-local-attention-notifications.md).
  - Token and cost usage: per-session never-reset totals and monthly totals
    (all sessions, per harness; deleted sessions included) read from harness
    logs; Copilot cost exact, Claude cost a list-price estimate with daily
    web price refresh (opt-out). See
    [`12-storage-layout.md`](./12-storage-layout.md) and
    [`11-security-model.md`](./11-security-model.md). Design note: the ledger
    lives in `internal/usage` with its own connection to `state.db`; ingest runs
    on UI polls (throttled ~2 s), not a background loop; Copilot records
    without `totalNanoAiu` are unpriced rather than estimated.
- **Shutdown/recovery:** a repeated native test verifies process, job, and
  input/output pipe handles are invalidated after ConPTY close; the opaque
  pseudoconsole is checked through successful close and cleared state. Wails-
  host close integration tests remain open. The per-cycle handle to the
  exited console host is inbox-ConPTY behavior; the bundled ConPTY showed no
  handle growth over 10 cycles. Stale-runtime reconciliation now covers the
  abandoned-starting, recent-starting, missing/exited-PID, reused-PID, and
  live-other-window cases.
- **Security fix (done):** session `environment:` values now travel in the child
  environment block (`conpty.ChildEnvironment`, `StartPreferredEnv`,
  `terminal.Options.Env`) instead of `$env:` lines in the `pwsh -Command`
  argument; `harness.PowerShellInvocation` no longer takes the environment.
  Tests (`TestSessionEnvironmentReachesChildButNotItsCommandLine`,
  `TestManagerPassesSessionEnvironmentOutsideTheCommandLine`) show a real child
  receives the value and its command line lacks it. Values remain plaintext in
  YAML; the V3 secret vault
  ([`docs/v3/features/08-secret-vault.md`](../v3/features/08-secret-vault.md))
  can now build on this.
- **Harness compatibility:** provider-backed Claude Code and Copilot CLI
  hosted-terminal walkthroughs are done (both render correctly end to end,
  user-authorized, live credentials); hosted-terminal restore for a session
  with at least one real conversation turn (as opposed to zero-turn sessions)
  remains unverified for both harnesses.
  Update: Copilot CLI 1.0.104 restore with real turns is verified through the
  session manager on both inbox and bundled ConPTY (the model recalled a word
  given before Stop; stop 1.4-2.2 s, not forced). Repeated runs
  (`-count=3`) of the conpty, sessionhost and cmd/taushell tests pass. The
  packaged window itself could not be scripted: WebView2 ignores the
  `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` env var and an HKCU policy here (Wails passes explicit
  browser arguments), so a DevTools port is unavailable without an app change.

The detailed step dependencies and acceptance criteria remain in
[`docs/v1/09-UI-Implementation-Plan.md`](../v1/09-UI-Implementation-Plan.md).

### Copilot quota chip (done)

Home shows `Copilot: used / entitlement credits (N%), resets DATE, on pace for ~N`
(amber when the projection exceeds the entitlement). It runs
`gh api /copilot_internal/user` using the user's own GitHub CLI sign-in
(`internal/quota`), caches for 5 minutes and polls every 5 minutes; nothing is
stored. It is hidden silently when `gh` is missing, signed out, or the plan has no
quota. Opt out with `quota: { copilot: false }` in `~/.tau_shell/config.yaml`.
Risks: the endpoint is undocumented and may change, and usage-based billing may
replace quotas. Verified in the real window with live numbers.

## Later phases

### Deferred alternative spike: direct ConPTY harness launch

**Not Phase 1 scope or an approved implementation change.** Keep the current
PowerShell 7 session host. Later, compare it with direct `ConPTY -> harness`
launch, using ConPTY's existing executable/working-directory/argv process
interface.

- **Potential wins:** PowerShell would not be required for harness-only
  sessions; remove one process and the PowerShell command-string construction
  from that path; launch can pass discrete argv directly. A correctly built
  child environment block could also keep session environment values out of a
  command string. These are hypotheses to verify, not guaranteed performance
  or security improvements.
- **Tradeoffs:** the terminal would belong to the harness, not an interactive
  shell. Users would lose the PowerShell prompt and ability to run ad-hoc shell
  commands in that session. WebView2 remains required. Decide whether the
  product needs an explicit shell-vs-harness session mode before changing
  prerequisite or setup UX.
- **Windows gotchas:** npm-installed CLIs can resolve to `.cmd` shims, which
  `CreateProcess` cannot run as normal executables. Compare resolving the
  runtime/script with `cmd.exe /d /s /c`; test robust quoting, `%`/`!`, spaces,
  Unicode paths, exact discrete argv, exit propagation and wrapper child
  lifetime. Direct launch must explicitly pass the session environment via a
  Unicode environment block rather than interpolating values into a command
  line. Re-establish the UTF-8/code-page contract currently provided by the
  PowerShell bootstrap; there is no universal bootstrap for arbitrary native
  and runtime-based harnesses. Re-test Job Object membership, graceful stop,
  process-exit detection, restore IDs and failed-launch reporting.
- **Spike sequence:** first add opt-in test-only launch plumbing without
  changing the YAML schema or default launch mode. Prove behavior with native
  `.exe` and `.cmd` fixtures: exact argv/environment, Unicode/emoji input and
  output bytes, ANSI, resize, terminal queries/mouse, graceful exit, child
  cleanup and accurate errors. Then run one authorized real-harness probe in a
  disposable project/session; never log credentials or retain transcripts.
- **Decision gate:** compare the fixture and real-harness results against the
  existing PowerShell-hosted flow. Adopt only if supported harnesses have
  reliable executable resolution, identical session/environment semantics,
  truthful lifecycle/restore results, and acceptable loss or explicit
  replacement of the interactive-shell experience. Otherwise retain the
  current architecture and record blockers. Detailed constraints are in
  [`02-conpty-integration.md`](02-conpty-integration.md#deferred-alternative-launch-the-harness-directly).

- Workspaces and group restore.
- Remote/mobile access, web dashboards, and remote push notifications. Local attention alerts (taskbar flash and Windows toast) are in Phase 1: [`22-local-attention-notifications.md`](./22-local-attention-notifications.md).
- General plugin SDK, rich attention detection, multi-agent orchestration, distributed execution, and cloud sync.
