# PowerShell Bootstrap

Phase 1 targets Windows 10 with PowerShell 7. Bootstrap each managed shell with consistent UTF-8 input/output and code-page settings (including `chcp 65001`), while preserving normal PowerShell behavior and session-scoped environment variables.

Validate the actual encoding behavior through ConPTY, not only the PowerShell process settings. Cover Unicode input/output and common line-drawing characters; detect and clearly report compatibility problems instead of silently corrupting text. Avoid changing machine-wide settings.

PSReadLine configuration, aliases, and session metadata variables should be added only when needed and must not interfere with AI harness startup or normal interactive use.
