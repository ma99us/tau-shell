# Terminal Rendering

## Phase 1: Windows 10 local terminal compatibility

- Preserve ANSI colors and correctly pass Unicode text through the ConPTY/PowerShell pipeline.
- Validate UTF-8 input/output and code-page configuration, including the PowerShell bootstrap's `chcp 65001` setting.
- Exercise common box-drawing characters, emoji, and Nerd Font glyphs in the local CLI/TUI.
- Detect or document font limitations and provide a readable fallback; TauShell should not silently alter system font settings or promise glyphs that the installed font cannot render.
- Test on the project's Windows 10 development environment as well as any later supported Windows versions.

Mobile-friendly rendering is outside Phase 1.
