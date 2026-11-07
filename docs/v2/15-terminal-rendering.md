# Terminal Rendering

## Phase 1: Windows 10 local terminal compatibility

- Preserve ANSI colors and correctly pass Unicode text through the ConPTY/PowerShell pipeline.
- Validate UTF-8 input/output and code-page configuration, including the PowerShell bootstrap's `chcp 65001` setting.
- Keep TauShell-owned prompts, controls, and picker decorations ASCII so they do not depend on a special font; session IDs remain available alongside user-defined names as a readable selection fallback.
- Treat session names and harness output as user/provider text: pass them through without font-dependent substitutions, and do not silently alter system font settings or promise glyphs the installed font cannot render.
- Nerd Font glyphs are relevant only when user/provider text includes them; document that their appearance depends on the terminal font.
- Test on the project's Windows 10 development environment as well as any later supported Windows versions.

General mobile-friendly terminal rendering is outside Phase 1. The
experimental phone companion can show a bounded, read-only view of visible
terminal text; it is not a full terminal renderer.

The CLI sets UTF-8 console code pages around interactive commands and
restores the previous values afterward. The Bubble Tea TUI was removed; the
desktop uses WebView2 and one xterm.js instance per open session, continuing
to drain output when its tab is not selected. Automated stream/process checks
cover supplementary-plane emoji input, Unicode/emoji output bytes, ANSI,
resize, and concurrent fake sessions. Those checks alone do not prove desktop
rendering. A separate Windows 10 production-window fixture verified Unicode,
split UTF-8, ANSI, cursor movement, input, and resize through the full path;
hosted Claude Code and Copilot CLI walkthroughs were also recorded. See the
[backlog](20-mvp-backlog.md#open-phase-1-backlog) and
[ConPTY fixture results](02-conpty-integration.md#repeatable-hosted-terminal-fixture)
for backend and verification limits. TauShell does not detect font coverage or
rewrite provider text; glyph appearance depends on the renderer and installed
fonts.

Clipboard: Ctrl+C copies when text is selected, Ctrl+V pastes, both through the native clipboard. Harnesses that select text themselves (Claude Code, Copilot CLI) copy with the OSC 52 escape; the embedded terminal writes that text to the clipboard and refuses read requests (`frontend/src/osc52.ts`). Manually confirmed on Claude Code and Copilot CLI.
