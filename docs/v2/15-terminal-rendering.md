# Terminal Rendering

## Phase 1: Windows 10 local terminal compatibility

- Preserve ANSI colors and correctly pass Unicode text through the ConPTY/PowerShell pipeline.
- Validate UTF-8 input/output and code-page configuration, including the PowerShell bootstrap's `chcp 65001` setting.
- Keep TauShell-owned prompts, controls, and picker decorations ASCII so they do not depend on a special font; session IDs remain available alongside user-defined names as a readable selection fallback.
- Treat session names and harness output as user/provider text: pass them through without font-dependent substitutions, and do not silently alter system font settings or promise glyphs the installed font cannot render.
- Nerd Font glyphs are relevant only when user/provider text includes them; document that their appearance depends on the terminal font.
- Test on the project's Windows 10 development environment as well as any later supported Windows versions.

Mobile-friendly rendering is outside Phase 1.

The CLI sets UTF-8 console code pages around interactive commands and restores the previous values afterward. The current full-screen TUI prototype uses one VT emulator per tab, drains every session's output continuously, and renders only the selected screen; resize events update selected and inactive ConPTY instances. The TUI and its renderer are disposable and are not the target desktop implementation. Automated stream/process checks cover supplementary-plane emoji input, Unicode/emoji output bytes, ANSI output, resizing, and two concurrently running fake harness sessions across tab switches. These checks do not establish compatibility for the planned WebView2 desktop terminal component. That component must be evaluated separately with supported harnesses on Windows 10. TauShell does not perform font-coverage detection or rewrite provider text; visual compatibility remains dependent on the terminal renderer and installed fonts.
