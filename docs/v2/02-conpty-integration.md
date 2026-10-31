# ConPTY Integration

ConPTY hosts the normal PowerShell 7 process used by a TauShell session:

```text
TauShell -> ConPTY -> PowerShell 7 -> AI harness
```

PowerShell and harnesses must continue to behave as interactive console applications. TauShell owns the pseudoconsole, passes terminal input to it, drains its virtual-terminal output, and resizes it when the viewport changes. The current interactive TUI implementation is a disposable prototype; its screen renderer is not the target architecture. The desktop GUI needs an independent terminal screen per session, rendered by a verified embedded terminal component inside WebView2. Output and terminal replies must be drained independently; only the selected tab receives user input. Rendering interprets terminal controls; it does not rewrite harness text or replace the harness interface with TauShell-owned chat UI.

## Phase 1 constraints

- Target Windows 10 (the current development system is Windows 10 22H2, build 19045) and PowerShell 7.
- Use the Windows ConPTY APIs; do not replace PowerShell or depend on a custom shell. Prefer the bundled `Microsoft.Windows.Console.ConPTY` (`conpty.dll` + `OpenConsole.exe` next to `taushell.exe`) and fall back to inbox kernel32 ConPTY when it is absent or fails to load; see "Bundled ConPTY" below.
- Keep the host ends of the input/output pipes alive for the session and close the ConPTY-side ends after child process creation. Drain output concurrently with input handling to avoid pipe-buffer deadlocks.
- Apply UTF-8 console encodings and code page 65001 inside the managed PowerShell process, not as a machine-wide setting. Restore any host console modes/code pages changed by the CLI on exit.
- Forward keyboard input, including Ctrl+C, and propagate local terminal size changes using `ResizePseudoConsole`.
- Attempt graceful interruption during confirmed shutdown, then force-stop the session process tree after a bounded wait. Use a Windows Job Object to prevent orphaned harness child processes.

## Deferred alternative: launch the harness directly

This is a research spike for a later phase, not a Phase 1 change. The supported
architecture remains `TauShell -> ConPTY -> PowerShell 7 -> AI harness`;
PowerShell stays first-class and remains the session shell for now.

ConPTY itself is shell-agnostic. `internal/conpty.StartPreferred` accepts an
executable, working directory, and argv, so a harness with a directly
launchable Windows executable could instead run as
`TauShell -> ConPTY -> AI harness`. This may remove PowerShell as a prerequisite
for harness-only sessions and removes one process and the PowerShell command
string from that launch path. It also removes shell parsing from the normal
launch path. It does not provide an interactive PowerShell prompt or let the
user run shell commands in the same terminal; it changes a session from a
shell-hosted harness to a harness-owned terminal.

The spike must resolve these Windows-specific boundaries before any product
decision:

- **Executable resolution:** `exec.LookPath` may resolve an npm-installed
	harness to a `.cmd` shim rather than a PE executable. `CreateProcess` cannot
	launch a batch shim as an ordinary executable. Compare resolving the actual
	runtime plus script with invoking the shim through `cmd.exe /d /s /c`;
	document quoting, `%`/`!`, spaces, Unicode paths, argument preservation, and
	wrapper exit/child-process behavior. Do not construct an unvalidated shell
	command from YAML arguments.
- **Environment:** the current PowerShell invocation applies session variables
	in its script. A direct launcher must construct and pass a Unicode Windows
	environment block, define whether it inherits the TauShell environment, and
	test overrides and empty values. Keep secrets out of command lines and
	diagnostics. A `cmd.exe` shim adds another command-line boundary and must not
	undo that property.
- **Encoding and terminal setup:** the PowerShell bootstrap currently sets
	UTF-8 encodings and code page 65001. Direct launch has no universal shell
	bootstrap that guarantees equivalent behavior for every runtime. Establish
	a documented supported contract for each harness/runtime, then verify raw
	Unicode/emoji input and output, ANSI, resize, terminal queries and mouse
	tracking through the bundled and inbox ConPTY backends.
- **Lifecycle and recovery:** confirm the tracked process is the one whose
	lifetime represents the harness session, its exit code/state is truthful,
	its children stay in TauShell's Job Object, and graceful interruption and
	forced termination work. A wrapper that exits separately from its child
	could otherwise make a live harness appear stopped. Re-run start, restore,
	start-fresh and resume-ID discovery tests; direct launch must not imply that
	a terminal buffer or conversation was restored.
- **Product boundary:** compare a harness-only TUI session with today's
	PowerShell prompt experience. Decide whether this is an opt-in launch mode,
	a per-harness adapter capability, or a separately named session kind. Keep
	the WebView2 requirement; only reconsider PowerShell setup gating if the
	product intentionally supports sessions that do not use PowerShell.

Start with a deterministic native `.exe` terminal fixture and a `.cmd` shim
fixture; require exact argv/environment assertions, Unicode and ANSI bytes,
resize, graceful exit, child cleanup, and failure reporting. Then run one
user-authorized real-harness compatibility probe in a disposable project and
session, without printing credentials or retaining transcripts. Compare
against the existing PowerShell-hosted path and record measured outcomes and
unsupported cases. See the [deferred backlog item](20-mvp-backlog.md#deferred-alternative-spike-direct-conpty-harness-launch).

## Spike findings

The executable proof of concept is in [`spikes/conpty/`](../../spikes/conpty/). The Windows application host is implemented under `internal/conpty/` and `internal/terminal/`; it uses Go with `golang.org/x/sys/windows` and `golang.org/x/term`.

Verified on Windows 10 build 19045 with PowerShell 7.6.6:

- ConPTY startup, PowerShell input/output, Ctrl+C interruption, and live resize.
- UTF-8 text, box drawing, emoji, and a Nerd Font private-use code point pass through the PowerShell/ConPTY stream.
- ANSI color sequences pass through; ConPTY may normalize equivalent sequences (for example, reset `ESC[0m` to `ESC[m`).
- Set `STARTF_USESTDHANDLES` in the extended startup info so PowerShell's standard streams are routed through the pseudoconsole in this host.
- Wait for the child terminal to initialize before relying on a resize; an early resize can be superseded by the child's initial console setup.
- Assign the suspended PowerShell process to a kill-on-close Job Object before resuming it. The cleanup fixture now launches a direct child process and verifies job termination stops it. A PowerShell `Start-Process` experiment showed a broker-style detached child can escape the job; such detached processes are intentionally unmanaged.

The CLI host applies/restores UTF-8 code pages, enables/restores virtual-terminal output mode on the local console, and hosts ConPTY processes for sessions. Automated Windows tests in `internal/conpty/process_windows_test.go` launch PowerShell through ConPTY, assert supplementary-plane emoji input and Unicode/emoji output, verify ANSI output and exit codes, resize the pseudoconsole, and confirm job termination stops a direct child fixture. Process-close tests also verify owned handles/files are cleared and close is idempotent. Existing TUI-specific session-manager code and tests are migration references only. These stream/process tests remain useful where UI-independent; the Bubble Tea UI and its VT-screen rendering path are not part of the desktop app. The desktop terminal component must pass its own Windows 10 compatibility gate before readiness is claimed. A separate controlled Copilot restore smoke test is documented in the session restoration notes. Visual glyph compatibility remains dependent on the selected terminal component and available fonts.

## Repeatable hosted-terminal fixture

`cmd/terminal-fixture/` is a deterministic fake harness for checking the
production desktop terminal path without contacting an AI provider. On Windows,
build it to a temporary directory as `copilot.exe` and put that directory first
on `PATH` before launching TauShell:

```powershell
go build -o "$env:TEMP\taushell-fixture\copilot.exe" ./cmd/terminal-fixture
```

Create a disposable `copilot-cli` session with an existing working directory
and set `TAUSHELL_FIXTURE_LOG` in that session's environment to a temporary log
file. The fixture reports a test version for `--version`, enters the alternate
screen, writes Unicode/emoji and ANSI color samples, exercises cursor movement
and erase, emits a four-byte emoji split across two writes 50 ms apart, requests
device-status and cursor-position reports, and enables mouse tracking. In the
hosted terminal, verify the rendered screen, type `tau-proof` followed by
Enter, click in the terminal, press `m` to re-enable mouse tracking, press `c`
or `d` to repeat the cursor-position or device-status query after startup,
resize the window, and press `q` to exit.
The event log records input lines, terminal query responses, mouse reports,
observed dimensions, and byte counts for each input read without recording
input contents.

On Windows 10 build 19045, the fixture passed through the production Wails
window, session bridge, PowerShell 7, ConPTY, and xterm.js. The embedded
terminal rendered split UTF-8 as `Split UTF-8: 🌍 [end]`, Greek/Japanese text,
a combining mark, emoji, ANSI colors, and cursor movement/erase. The cursor
position query returned `ESC[10;1R`; `tau-proof` input, a resize from 117x35 to
94x28, and the explicit `q` exit were recorded. The exit restored the preserved
primary-screen marker. A follow-up session captured the cursor report as a
terminal-control input, confirming the response reaches the fixture parser.
In a later run, the fixture's `c` key sent a new cursor-position query and
logged the reply `ESC[6;15R`. Its `d` key sent a new device-status query;
xterm's `ESC[0n` response was recognized at the Go bridge and the ConPTY input
write succeeded, but no corresponding input chunk reached the fixture.
The repeated cursor reply reached the fixture as a seven-byte input chunk. A
native-window click at a visible terminal cell produced no mouse report at
the Go bridge or fixture.

The latest production run added payload-free markers for DOM `pointerdown`
and `mousedown` events and xterm's effective mouse-tracking mode. A click
reached the terminal container, but xterm remained in `none` mode and emitted
no mouse report. Pressing the fixture's `m` key to resend `CSI ? 1000 h` and
`CSI ? 1006 h` did not change that state. A separate temporary xterm-side
probe that enabled those modes directly reached `vt200` and forwarded an SGR
mouse report to the Go bridge, confirming the WebView click and xterm input
path work when tracking is active.

A direct ConPTY probe wrote `CSI ? 1000 h`, `CSI ? 1006 h`, and a text marker
from PowerShell. The captured ConPTY stream retained the marker but omitted
both mouse-mode sequences. This shows that Windows ConPTY consumes these DECSET
controls before xterm receives the stream. Unconditionally enabling xterm
mouse tracking would change normal terminal selection/input behavior, so it is
not an acceptable silent workaround. Supporting mouse modes across this
PowerShell/ConPTY boundary needs an explicit architecture decision. The
decided next step is a bundled-ConPTY spike: load a newer `conpty.dll` with
`OpenConsole.exe` behind a loader switch, keep inbox ConPTY as fallback, and
compare both for mouse-mode passthrough, device-status/cursor-report delivery,
and the residual console-host process handle. Adopt it only if mouse modes
pass through. Device-
status input still reaches the Go bridge but is not observed by the fixture;
its loss and the source of the fixture-visible cursor report remain unresolved
(hypothesis: inbox ConPTY answers these queries itself).
Real-harness walkthroughs are also not established by these fixture runs.

An isolated browser-side xterm check confirmed that xterm.js emits
`ESC[0n` for a device-status request and SGR mouse reports after mouse tracking
is enabled and the terminal is clicked. This isolates component capability
only; it does not exercise TauShell's Wails bridge or ConPTY path. Payload-free
diagnostics record the first successfully forwarded protocol category per
session, pointer-event categories without coordinates, and effective xterm
mouse-tracking mode changes. The device-status response reaches the Go bridge
but is not observed by the fixture. Native pointer events reach the terminal
container, but no mouse report reaches the bridge unless tracking is enabled
directly in xterm; ConPTY's filtering of DECSET mouse-mode controls is
confirmed. The source of the fixture-visible cursor-position reply is also
unresolved because no matching bridge marker appeared. Resolve the protocol
boundary before closing the feasibility gate. Fixture input-chunk byte counts
are test-only metadata and do not include the input bytes themselves.

This fixture tests the WebView2/xterm/bridge/PowerShell/ConPTY transport; it
does not test either real harness's UI, CLI compatibility, authentication, or
conversation restore. Do not count it as a provider walkthrough.

## Bundled ConPTY

The findings above are inbox Windows 10 ConPTY behavior. A spike compared it
with `Microsoft.Windows.Console.ConPTY` 1.25.260930003 (MIT, Microsoft-signed,
documented for Windows 10 build 17763+), loaded through `StartWithLibrary` in
`internal/conpty`. `TestConPTYBackendSpike` (opt-in through
`TAUSHELL_CONPTY_SPIKE_DLL`) drives the terminal fixture directly, replies to
queries the way xterm.js would, injects SGR mouse reports, and counts process
handles across ten start/close cycles. On Windows 10 build 19045:

| Behavior | Inbox ConPTY | Bundled ConPTY |
|---|---|---|
| `CSI ? 1000 h` / `CSI ? 1006 h` reach the host | No (consumed) | Yes |
| SGR mouse input reaches the fixture | No | Yes |
| Cursor-position query (`CSI 6 n`) | Answered by ConPTY with its own position; never reaches the host | Forwarded; the host reply reaches the fixture |
| Device-status query (`CSI 5 n`) | Forwarded, but the host reply is swallowed | Forwarded; the host reply reaches the fixture |
| Handle-count delta after 10 cycles | +16 (exited console-host handles) | 0 |

This confirms the hypothesis that inbox ConPTY answers cursor reports itself
and explains the earlier device-status loss. It also resolves the residual
per-cycle console-host handle: it does not occur with the bundled host.

TauShell therefore adopts the bundled ConPTY. `conpty.StartPreferred` uses
`conpty.dll` when both it and `OpenConsole.exe` are present in the executable's
directory and falls back to inbox ConPTY otherwise. A present bundle that fails
to load is reported as a diagnostics warning, not silently ignored. Both the
desktop session manager and the CLI terminal host log the selected backend.
`scripts/fetch-conpty.ps1` places the pinned, hash- and signature-verified x64
files next to the build; they are not committed.

A production Wails smoke with the bundled files and an isolated profile logged
`ConPTY backend selected backend=bundled`. The startup device-status reply from
xterm.js reached the fixture, and native clicks in the WebView terminal reached
the fixture as SGR press/release reports. After the app process was stopped,
no `OpenConsole.exe`, PowerShell, or fixture processes remained. xterm.js now
enables mouse tracking only when the harness requests it, so normal selection
is unchanged for applications that do not.
