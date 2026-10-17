# ConPTY Integration

ConPTY hosts the normal PowerShell 7 process used by a TauShell session:

```text
TauShell -> ConPTY -> PowerShell 7 -> AI harness
```

PowerShell and harnesses must continue to behave as interactive console applications. TauShell owns the pseudoconsole, passes terminal input to it, drains its virtual-terminal output, and resizes it when the viewport changes. The current interactive TUI implementation is a disposable prototype; its screen renderer is not the target architecture. The desktop GUI needs an independent terminal screen per session, rendered by a verified embedded terminal component inside WebView2. Output and terminal replies must be drained independently; only the selected tab receives user input. Rendering interprets terminal controls; it does not rewrite harness text or replace the harness interface with TauShell-owned chat UI.

## Phase 1 constraints

- Target Windows 10 (the current development system is Windows 10 22H2, build 19045) and PowerShell 7.
- Use the Windows ConPTY APIs; do not replace PowerShell or depend on a custom shell.
- Keep the host ends of the input/output pipes alive for the session and close the ConPTY-side ends after child process creation. Drain output concurrently with input handling to avoid pipe-buffer deadlocks.
- Apply UTF-8 console encodings and code page 65001 inside the managed PowerShell process, not as a machine-wide setting. Restore any host console modes/code pages changed by the CLI on exit.
- Forward keyboard input, including Ctrl+C, and propagate local terminal size changes using `ResizePseudoConsole`.
- Attempt graceful interruption during confirmed shutdown, then force-stop the session process tree after a bounded wait. Use a Windows Job Object to prevent orphaned harness child processes.

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
