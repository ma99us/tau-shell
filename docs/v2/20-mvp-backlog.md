# MVP Backlog

## Phase 1: local Windows desktop GUI (TUI removed)

- Completed migration: the Bubble Tea TUI and its TUI-only dependencies were removed. `list`, `start <session-id>`, and explicit `restore <session-id>` remain available as CLI operations; no TUI fallback is planned.
- In progress: a Go/Wails WebView2 host, bundled React/xterm.js launcher, local tabs, session creation, tool inventory, session lifecycle bridge, and shutdown confirmation are implemented. A user-authorized, live-credential walkthrough exercised both supported harnesses (Claude Code, GitHub Copilot CLI) through the real hosted terminal on this Windows 10 machine; a clean-environment test without WebView2 will not be performed (user decision); the missing-runtime prompt is accepted as working. See [`docs/v1/08-UI-Design.md`](../v1/08-UI-Design.md) and [`docs/v1/09-UI-Implementation-Plan.md`](../v1/09-UI-Implementation-Plan.md).
- Security hardening: the Windows host now cancels top-level WebView2 navigations outside the bundled `http://wails.localhost` origin before they load. Wails' bound-method origin validation remains a separate defense.
- Session-manager lifecycle coverage is implemented for concurrent independent fake sessions, input/output/resize, process exit, failed restore preserving the resume ID, graceful and forced stop, and cleanup. This does not cover a real hosted xterm or harness.
- Implemented executable detection, resolved version parsing, minimum/tested-version classification, and per-adapter capability probes (via `--help` option matching) for PowerShell 7, Claude Code, and Copilot CLI; Home shows each tool's resolved executable path, version, support classification, and any unmet capability. A first-run Setup screen blocks on a missing/unusable PowerShell 7 with a "continue without" escape hatch. Do not silently pin or downgrade.
- Implemented schema: strict YAML accepts ordered harness `launch_args`; managed session/resume arguments cannot be overridden. Implemented generated per-harness launch-argument templates (Claude Code → `--dangerously-skip-permissions`, Copilot CLI → `--yolo`, via `Adapter.DefaultLaunchArgs()`), prefilled editably in the Create Session dialog without overwriting user edits, plus a live launch preview (`PreviewSessionLaunch`) showing the resolved executable and full effective argv and warning on options the installed harness does not support.
- **Planned setup:** WebView2 and PowerShell 7 are TauShell host prerequisites; missing harnesses do not block Home. winget is optional; installation requires a confirmation dialog showing the exact winget command before it runs in a visible console (never silent/elevated), and the tool is always re-detected after winget exits — success is reported only once it becomes usable. When winget is unavailable or a tool has no winget package, Home instead links to the vendor's official setup page.
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
  (`scripts/fetch-conpty.ps1`) and falls back to inbox. Remaining: conduct
  user-authorized non-destructive Claude Code and Copilot CLI hosted-terminal
  walkthroughs using the user's existing sign-ins.
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
- **UI improvements (planned, not implemented):**
  - Drag-and-drop reordering of sessions with the mouse, from either the left
    menu or the tab strip. Order must stay identical across Home tiles, left
    menu items, and tabs, so the order needs to be persisted and shared rather
    than derived from file names.
  - Rename the "Open terminal" action on Home session tiles to "Open session".
  - Duplicate a session from the right-click menu on a left-menu item or tab,
    and from a button on the Home session tile. The copy reuses the source
    session's working directory and harness and is opened as a new session
    immediately, with no prompts. It must not copy runtime state (resume IDs,
    restore outcome), and the YAML id and name must stay unique.
- **Shutdown/recovery:** a repeated native test verifies process, job, and
  input/output pipe handles are invalidated after ConPTY close; the opaque
  pseudoconsole is checked through successful close and cleared state. Wails-
  host close integration tests remain open. The per-cycle handle to the
  exited console host is inbox-ConPTY behavior; the bundled ConPTY showed no
  handle growth over 10 cycles. Stale-runtime reconciliation now covers the
  abandoned-starting, recent-starting, missing/exited-PID, reused-PID, and
  live-other-window cases.
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

## Later phases

- Workspaces and group restore.
- Notifications, mobile dashboard, remote interaction, and remote/web dashboard. A local-only slice (tab marker, taskbar flash) is pre-planned in [`22-local-attention-notifications.md`](./22-local-attention-notifications.md).
- General plugin SDK, rich attention detection, multi-agent orchestration, distributed execution, and cloud sync.
