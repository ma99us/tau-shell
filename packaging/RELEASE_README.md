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
PowerShell 7. If PowerShell 7 is missing, TauShell offers a confirmed install;
if WebView2 is missing, TauShell directs you to Microsoft's official runtime
installer. Nothing is installed without your confirmation.

Administrator rights are not required to run TauShell. Node.js/npm and Git for
Windows are optional tools shown on Home, not TauShell prerequisites. TauShell
can install Node.js LTS and Git for Windows PortableGit into the current user's
profile after confirmation. Node.js is needed only for harnesses installed
through npm, such as GSD; other supported harnesses can be installed without
Node. Existing tool installations are detected and reused.

### Unsigned app warning

TauShell is not code-signed yet, so Windows may warn you the first time you run
it. This is expected for unsigned apps and does not mean the file is damaged.

- **"Windows protected your PC" (SmartScreen):** click **More info**, then
  **Run anyway**.
- **Skip the prompt:** before extracting, right-click the ZIP, choose
  **Properties**, tick **Unblock**, and click OK. Or in PowerShell:
  `Unblock-File .\TauShell-<version>-windows-x64.zip`
- **Windows 11 Smart App Control:** if it is on, it blocks unsigned apps with no
  "Run anyway" option. You would have to turn Smart App Control off in Windows
  Security (this cannot be undone without resetting Windows), so only do that if
  you trust the download.

Only run builds downloaded from this repository's Releases page.

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
