# TauShell Desktop UI

**Status:** Agreed direction; the first desktop implementation is in progress and has not passed the Windows 10 feasibility gate.

This document defines the intended Phase 1 user interface: a Windows desktop
application for managing PowerShell-backed AI harness sessions. The active
product and engineering specifications remain in [`docs/v2/`](../v2/); this
document captures the agreed GUI direction and corrects the earlier TUI plan.

## 1. Product model

TauShell is a desktop session manager with a startup/session-launcher screen
and a tabbed workspace. Each session tab contains the complete terminal UI of
its AI harness, hosted through a TauShell-managed PowerShell 7 process and
ConPTY. TauShell does not replace the harness's conversation UI with its own
chat transcript.

```text
+-----------------------------------------------------------------------+
| TauShell     Home     [Backend Agent *] [Copilot / stopped]       [+] |
+-----------------------------------------------------------------------+
|                                                                       |
|       Selected session's full Claude Code or Copilot CLI TUI          |
|       rendered in an embedded terminal                               |
|                                                                       |
+-----------------------------------------------------------------------+
| Session status · working directory · terminal controls               |
+-----------------------------------------------------------------------+
```

The desktop application is the primary interactive experience. The existing
command-line operations (`list`, `start <session-id>`, and
`restore <session-id>`) remain available for scripting and recovery. The
existing interactive TUI is a disposable prototype/dead-end frontend, not a
secondary supported mode. Replace it with the desktop GUI and remove its
frontend code; do not preserve two competing interactive interfaces.

## 2. Desktop foundation

The agreed application direction is:

- Go owns session lifecycle, harness adapters, PowerShell/ConPTY processes,
  runtime persistence, and shutdown.
- A Windows WebView2 host presents a local web UI. React is the preferred UI
  implementation candidate; validate the smallest host/bridge approach before
  settling code structure.
- xterm.js is the preferred embedded terminal-rendering candidate. It must be
  evaluated with the actual supported harnesses and target Windows versions
  before adoption.
- Use the installed Microsoft Edge Evergreen WebView2 Runtime. If it is
  unavailable, show a clear setup message and instructions; do not silently
  fail or download/install it without user consent.

The WebView is a local presentation surface, not a remote dashboard or web
server. Keep application assets local. Expose a narrow, typed bridge for
session operations, terminal input/output, resize, and state events. Do not
expose arbitrary Go functions, unrestricted filesystem access, or secrets to
the web renderer. Prevent unexpected navigation and external content from
gaining access to the app bridge.

WebView2 and xterm.js are architectural candidates, not yet verified
dependencies. The host library, runtime behavior, terminal protocol, licensing,
maintenance, and Windows 10 compatibility must pass the feasibility gates in
[09-UI-Implementation-Plan.md](./09-UI-Implementation-Plan.md).

### Current feasibility evidence

The initial implementation uses Wails v2.16.0, React 19, and xterm.js 6 with
all frontend assets bundled into the Windows executable. On Windows 10 Pro
22H2 (build 19045), with Go 1.27.2, PowerShell 7.6.7, GitHub Copilot CLI
1.0.99-2, and Evergreen WebView2 Runtime 155.0.4283.60, frontend/Go tests and
`go vet` pass and a production-tagged executable opens its TauShell window.
Automated session-manager tests now cover concurrent output/input/resize,
process exit, restore failure, graceful and forced stop, and process cleanup.
This confirms the host startup, local asset path, and those process-level
behaviors only. The installed Claude Code 2.1.302 CLI help documents
`--session-id` and `--resume`; GitHub Copilot CLI 1.0.99-2 help documents the
same flags. This confirms CLI-level options only. The existing npm-installed
Claude Code 2.1.133 is found first on the current PATH, and neither Claude
version has passed a provider-backed hosted-terminal test.
A browser-canvas interaction smoke with a mocked bridge exercised xterm.js
split UTF-8/emoji writes, ANSI color, alternate-screen switching, device-status
query replies, mouse-tracking input encoding, resizing, and keyboard input.
This verifies browser-side xterm behavior only; it does not exercise the
frontend bridge, WebView2, ConPTY, or PowerShell as a complete terminal path.
The actual frontend bridge and either harness's compatibility inside the
hosted terminal have not passed end-to-end testing. Wails/React/xterm.js
therefore remain provisional, and Step 1 is still open.

Wails v2.16.0 validates both the top-level and sender origins against its
default `http://wails.localhost` origin before dispatching bound methods; the
application does not add any `BindingsAllowedOrigins`. The repository-local
Wails/WebView2 host extension also cancels top-level navigations outside that
origin in WebView2's native `NavigationStarting` event, before they load.
Origin-policy tests, a production host smoke, and a production-mode WebView2
attempted external-navigation smoke pass on the current machine. This does not
close the separate xterm/harness feasibility gate.

The checked-in `cmd/terminal-fixture/` program was built as `copilot.exe` and
run from an isolated TauShell profile through the production Wails window,
session bridge, PowerShell 7, and ConPTY. The embedded xterm rendered
Greek/Japanese text, a combining mark, emoji, ANSI colors, and cursor
movement/erase, and a four-byte emoji split across two writes 50 ms apart. It
replied to a cursor-position query, forwarded typed input, and reflected a
live resize from 117x35 to 94x28; explicit `q` exit was recorded. Mouse reports
and device-status query replies were not observed. Alternate-screen
restoration is not established.
This fixture does not replace the required non-destructive Claude Code and
Copilot CLI walkthroughs. The actual hosted-terminal path is therefore
partially verified, and Step 1 remains open.

The current Bubble Tea screen and its in-process VT renderer are not the GUI
architecture. Use the TUI only as a behavioral reference or as a source of
UI-independent regression fixtures; do not wrap or port the TUI UI into
WebView2.

## 3. Tool discovery and first-run setup

At startup, TauShell detects supported harness CLIs and their versions, plus
the runtime prerequisites declared by the relevant adapters. Show installed,
missing, unsupported, and version-outside-tested-range states in a setup
surface; discovery must not block access to Home merely because no harness is
installed.

TauShell's own host prerequisites are Microsoft Edge WebView2 Runtime and
PowerShell 7. If either is missing, open a first-run setup path before the
main workspace. Since the web UI cannot render when WebView2 is absent, the
WebView2 prerequisite error/install handoff must use a minimal native Windows
dialog or another independently available bootstrap surface.

- Required host prerequisites block terminal session launch until installed,
  but missing harnesses do not block TauShell from opening.
- A harness CLI is required only to start or restore sessions for that
  harness. If it is missing, show an Install option and keep other installed
  harnesses usable.
- Node.js, Git, Python, and other tools are classified by adapter/use case as
  required, recommended, or optional, with the effect of absence stated. Do
  not label a tool mandatory globally unless TauShell itself or every supported
  session requires it.
- `winget` is an optional installation mechanism, not a TauShell runtime
  prerequisite. If it is unavailable, provide official vendor setup links.
- Detect again after installation and report the actual installed version and
  whether the requested tool is ready. Do not claim an install succeeded based
  only on an installer exit code.

An Install action may invoke a known package through winget only after the user
confirms the named package and source. Use maintained, explicit package IDs;
do not construct package names from untrusted input, run arbitrary downloaded
scripts, silently elevate, or install tools without consent. If installation
needs elevation, explain that and let the trusted installer request it.

### Version support policy

Use the user's installed tool version; TauShell does not silently pin,
downgrade, or auto-update third-party tools. Maintain per-adapter minimum
requirements and a tested-version range, and probe the specific capabilities
needed for launch and restore. Warn when the detected version is outside the
tested range, but do not reject it solely for being newer or untested. If a
required feature is absent or a command fails, report that exact capability
failure and offer a supported alternative when available.

Package installation may select a current stable package version, but must
display the source/package before confirmation and verify the installed result.
Compatibility probes and regression tests should be tied to recorded tool
versions so upstream changes can be diagnosed without forcing users onto an
older version.

## 4. Startup and session launcher

The initial desktop window opens to a **Home** / session-launcher screen. It
lists configured session definitions with their display name, stable session
ID, harness, working directory, and truthful runtime status. Prior tab
arrangement and selected session remain available from Home, but startup does
not automatically launch or restore harnesses. Home also shows the detected
tool inventory and makes missing prerequisite/harness installation paths easy
to reach.

- Selecting a never-started session offers **Start**.
- Selecting a session with a saved harness resume ID offers **Restore** and
  **Start fresh**.
- **New session** lets the user choose a supported harness and working
  directory, then creates a human-editable YAML session definition. Generate
  its ID, name, harness-specific default arguments, PowerShell settings, and
  UTF-8 behavior from a visible template; show the resulting settings before
  saving so the user can edit them. After saving, offer to start the session
  immediately.
- Selecting an already-running session focuses its existing tab; it must not
  create a second process for the same configured session.
- **Start fresh** with an existing resume ID requires confirmation. If
  confirmed, TauShell creates and saves a new ID, replacing its reference to
  the old harness conversation. The old conversation may remain in
  harness-owned storage but is no longer tracked by this TauShell session.
- Failed launch or restore is shown with its error and an explicit retry or
  fresh-start path. PowerShell process creation alone is not proof that a
  harness conversation was restored.

The `+` action and Home screen make configured sessions reachable at any time.
The startup page is a launcher, not a project browser or a GSD-specific
dashboard. It can offer a folder picker and recent working directories as
convenience inputs for new session creation.

## 5. Tabs and live sessions

- A tab represents one configured TauShell session ID. The same session cannot
  be live in two tabs simultaneously.
- Tabs show a concise session name and runtime state; a details surface can
  show its session ID, harness, and working directory.
- Switching tabs changes which embedded terminal is visible and receives
  keyboard/mouse input. Every other live session continues running in the
  background, and TauShell continues consuming its ConPTY output.
- Each session keeps independent ConPTY, terminal-emulator state, dimensions,
  process lifecycle, and harness resume ID.
- Opening an already-running background session returns to its existing
  terminal screen without relaunching it.
- Tab ordering and the last selected tab are presentation/runtime metadata.
  Keep them out of hand-edited session YAML and separate from harness runtime
  state.
- The tab interface must remain usable with many sessions. At minimum, provide
  an overflow/list affordance and keyboard navigation; do not require every
  tab to fit at once.

## 6. Editable harness arguments

Session YAML must support one ordered `launch_args` list of additional harness
CLI arguments. Apply this same configured list to both fresh start and restore;
the adapter supplies the appropriate TauShell-managed required arguments,
including the explicit session ID. Store arguments as YAML list items, not a
shell command string, so paths and values are passed as individual process
arguments without shell interpolation.

Generated session templates may include harness-specific defaults. For
example, the Claude Code template may include
`--dangerously-skip-permissions`, and the Copilot CLI template may include
`--yolo`; users can edit or remove these in YAML. TauShell passes configured
arguments through according to the adapter's documented ordering and reports
unsupported/invalid options as harness launch errors. TauShell does not add a
separate warning or confirmation for a saved argument: the user-authored YAML
is the explicit configuration.

Keep TauShell-managed arguments, including exact resume/session IDs, under
adapter control. Validate that user arguments cannot silently override the
managed session identity or duplicate mutually exclusive adapter-owned options.
Do not place secrets in command-line arguments or configuration.

## 7. Embedded terminal behavior

The harness continues to run as an interactive console application inside its
own PowerShell/ConPTY. An embedded terminal component interprets its VT/ANSI
screen-control stream and renders the result in the selected tab. Raw pass
through to the desktop window is not sufficient because a harness could clear
or overwrite TauShell's surrounding UI.

- Preserve the harness's visible text, supported colors, cursor behavior,
  alternate screen, input modes, and resize behavior. This is terminal
  emulation, not message parsing or chat reconstruction.
- Keep TauShell-owned labels and status controls clear and accessible. Do not
  rewrite provider output or substitute unsupported characters.
- Route keyboard and mouse input to only the selected terminal, subject to
  documented TauShell shortcuts.
- Continue draining each background session's output, maintaining its screen
  model, and responding to terminal queries even while its tab is hidden.
- Keep the ConPTY dimensions consistent with the embedded viewport. Handle
  active and inactive session resize when the window or layout changes.
- Report unsupported required terminal behavior instead of silently
  corrupting output.
- Restore local console state on exit for any CLI path that changes it. Desktop
  operation must not depend on an outer console being attached.

Visual glyph compatibility still depends on the user's installed fonts and
WebView/terminal renderer. Unicode and ANSI handling must be verified end to
end; encoding settings alone do not establish correct terminal behavior.

## 8. Truthful session state

Display states derived from TauShell's process lifecycle and adapter outcome:

| State | Meaning |
|---|---|
| `STARTING` | TauShell is creating PowerShell/ConPTY or starting the harness. |
| `RUNNING` | The managed process is alive; this does not claim the agent is generating a response. |
| `STOPPED` | The managed process exited normally or was deliberately stopped. |
| `FAILED` | Launch, terminal transport, or resume failed; show the relevant error. |
| `NOT STARTED` | The configured session has no live managed process. |

Do not show “thinking,” “waiting on you,” or “auto-run” unless a future,
documented adapter capability reliably reports that state. The active tab is
not itself a harness state.

## 9. Closing tabs and application shutdown

Closing a non-running tab removes it from the open tab list. Closing a running
tab asks whether to **Stop session** or **Leave running in background**. A
background session stays managed and discoverable through Home and the session
chooser.

Quitting TauShell while sessions are running requires confirmation. On
confirmation, TauShell attempts graceful interruption, waits briefly, then
force-terminates remaining process trees. It visibly reports when forced
termination was needed. TauShell's process job manages PowerShell and child
processes that inherit that job. A child launched outside the job through shell
or broker semantics (for example, PowerShell `Start-Process`) is detached and
is not managed or guaranteed to stop with TauShell; that limitation must be
documented, not addressed by guessing at process ancestry and terminating
possibly unrelated processes. On cancellation, the window and all managed
sessions remain available.

## 10. Harness widgets

Running sessions have optional quality-of-life widgets in the terminal toolbar:
Prompts (saved prompt presets), Commands (adapter slash commands), Model,
Compact, and Interrupt, plus a command palette (Ctrl+Shift+P) and shortcuts
(Ctrl+1..9, Ctrl+Tab / Ctrl+Shift+Tab). They are built on adapter-declared
capabilities and the rules below.

- Show a widget only when the harness adapter declares it
  (`Spec.Widgets`); the Model menu also requires the `--model` capability
  probe. The terminal remains fully usable without any widget.
- Widgets type into the terminal through the same input path as the keyboard,
  so they work whether or not the terminal has focus. Delivery follows what the
  harness itself shows (the attention cues):
  - A permission or selection prompt is open: refuse and tell the user, because
    typed text could answer the prompt.
  - The harness is idle: type the text, then press Enter as a separate write.
  - Working or no cue: insert the text only; the user presses Enter. (Copilot
    CLI queues a command sent while it is working, so submitting blindly is
    unsafe.)
  - Multi-line text is pasted rather than typed so newlines do not submit early.
  - Interrupt is an explicit user action and is always sent (Copilot CLI needs
    Esc twice).
- Report "sent" or "inserted", never "applied": the harness gives no
  confirmation. A model switch reports the request, not the current model.
- Text is appended to anything already typed at the harness prompt.
- Prompt presets are user-authored, live in `~/.tau_shell/prompts.yaml` with
  per-session overrides in the session YAML, and are kept separate from secrets
  and runtime data. They are never presented as validated harness commands. See
  [`docs/v2/18-yaml-schema.md`](../v2/18-yaml-schema.md#prompt-presets).
- Out of scope for now: context/cost/quota gauges, permission or autonomy
  selectors, permission-prompt buttons, drafts and sent-prompt history,
  transcript export and search, and a model picker in the Create dialog.

## 11. Phase 1 non-goals

- Replacing harness TUIs with a TauShell chat UI or parsing conversation text.
- Chat history, message search/export, context/cost/quota gauges, permission
  or autonomy selectors, or GSD-specific progress views. (Prompt presets,
  harness command and model widgets, the command palette, and tab shortcuts are
  implemented; see section 10.) Editable per-session CLI arguments and
  generated defaults are included.
- Remote access, mobile apps, web dashboards/server mode, notifications, or
  cloud synchronization.
- Multiple TauShell windows, detached tabs, split terminal panes, or tray-first
  operation.
- Automatic session or conversation restore on application startup.
- Replacing PowerShell, silently changing machine-wide settings, or claiming
  conversation restore without adapter/harness evidence.

## 12. Inspiration from the discontinued GSD-Tau docs

The discontinued `D:\Projects\gsd-tau\docs\` project suggested useful
application patterns: a clear launcher, familiar tabs, keyboard navigation,
truthful state indicators, visible failures, and separating presentation state
from session intent. Its chat-first Pi RPC design, message/tool cards, GSD
progress panel, detached windows, notifications, and mobile/dashboard ideas do
not define TauShell's Phase 1 GUI.
