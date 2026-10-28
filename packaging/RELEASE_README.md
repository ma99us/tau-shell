# TauShell

TauShell is a Windows desktop app that keeps your AI coding sessions alive. Each
session is a real PowerShell 7 terminal running a tool such as Claude Code,
GitHub Copilot CLI, GSD, Codex or OpenCode. Close the app, come back later, and
restore the same sessions; where the tool supports it, the conversation resumes
too.

## Install

1. Download `TauShell-<version>-windows-x64.zip` and extract it anywhere.
2. Run `taushell.exe`. There is no installer and no admin rights are needed.

**Requirements:** Windows 10 or later, the Microsoft Edge WebView2 Runtime, and
PowerShell 7. If something is missing, TauShell offers to install it on first
run (nothing is installed without your confirmation).

**"Windows protected your PC"?** This build is not code-signed yet. Choose
**More info**, then **Run anyway**. If Windows 11 Smart App Control blocks it,
that cannot be bypassed from the prompt.

To verify the download, compare its SHA-256 with `SHA256SUMS.txt` on the release
page.

## Quickstart

1. On **Home**, create a session: pick a project folder and a tool. Missing
   tools can be installed from the same screen.
2. Click **Start**. The session opens in its own tab as a normal PowerShell
   terminal running the tool.
3. Work as usual. Use the toolbar for saved **Prompts**, **Commands**,
   **Model**, **Compact** and **Interrupt**. Ctrl+Shift+P opens the command
   palette; Ctrl+1..9 switches tabs.
4. Close TauShell when you are done. It asks for confirmation if sessions are
   running, stops them gracefully, then force-stops any that do not exit.
5. Next time, use **Restore** on a session (or **Resume All**) to bring it back.
   Tick **auto resume** on a session, or the global option next to Resume All,
   to restore sessions automatically at startup.

## Good to know

- Your sessions, settings and usage data live in `%USERPROFILE%\.tau_shell`.
  Deleting the app folder never removes them.
- Command line: `taushell list`, `taushell start <id>`, `taushell restore <id>`.
- Restoring a session brings back its PowerShell process; the AI conversation is
  resumed only when the tool supports it, and TauShell tells you when it cannot.
- TauShell never downloads or installs updates by itself. Home shows a small link
  when a newer release exists.

Full documentation: <https://github.com/ma99us/tau-shell>. License: MIT.
