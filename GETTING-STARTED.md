# Getting Started with TauShell

Install TauShell, add one AI tool, and run your first session. Budget about 10 minutes.

TauShell is a Windows desktop app that runs AI coding tools (Claude Code, GitHub Copilot CLI, GSD, Codex CLI, OpenCode) in persistent sessions. Each session is a normal PowerShell 7 terminal. TauShell adds tabs, resume, a prompt queue, Git sync, and usage tracking. It does not sign you in to anything and does not replace your tools.

## 1) Check requirements

- Windows 10 or later
- Microsoft Edge WebView2 Runtime (already on current Windows 11; first run tells you if it is missing)
- PowerShell 7 (first run offers to install it) or a ready WSL2 distro

You do not need admin rights.

## 2) Install

1. Download `TauShell-<version>-windows-x64.zip` from the [releases page](https://github.com/ma99us/tau-shell/releases).
2. Unblock and extract it anywhere:

   ```powershell
   Unblock-File .\TauShell-<version>-windows-x64.zip
   Expand-Archive .\TauShell-<version>-windows-x64.zip -DestinationPath $HOME\TauShell
   ```

3. Run `taushell.exe`. There is no installer.

Builds are unsigned pre-releases. If SmartScreen says "Windows protected your PC", choose **More info**, then **Run anyway**. Windows 11 Smart App Control blocks unsigned apps and cannot be bypassed from the prompt. To check a download, see [Install](README.md#install).

Your data lives in `%USERPROFILE%\.tau_shell`. Delete the extracted folder to uninstall.

## 3) First run

TauShell opens on **Home**.

1. If it offers to install PowerShell 7, accept. It asks before installing anything and never elevates silently.
2. Open the **Tools** panel. It lists each supported tool with its detected version and state.
3. Install **one** AI tool from the panel (see the next section). Add others later, only when a task needs them.

A missing tool never blocks Home. A note at the bottom of the Tools panel says whether this account could elevate to admin; TauShell uses that only to pick the best install route.

## 4) Install one harness

| If you want | Start with |
|---|---|
| A general-purpose default | GitHub Copilot CLI |
| Deeper debugging and careful review | Claude Code |
| Large, staged work | GSD |
| Several model providers | OpenCode |

Click **Install** next to the tool and confirm. TauShell uses winget, falls back to npm when that fails and Node.js is usable, and re-checks the version afterwards. If Node.js or Git is missing, it can install them into its own per-user folder. Those copies are used only by TauShell sessions and do not change your system PATH. If an install still fails, use the vendor link the panel shows, then press refresh.

Sign in once, inside the tool, the first time you start a session. TauShell never sees your credentials.

## 5) Start your first session

1. On Home, choose **Create session**.
2. Pick the harness. Unavailable ones are greyed out with a reason.
3. Pick the project folder. This is the folder the AI will work in.
4. Create it. The session starts immediately in a new tab.
5. Sign in if the tool asks, then type a prompt.

Check that it works with a small, safe task: "Summarize this repository in five bullets."

## 6) Daily flow

- **Stop and resume.** Stop a session when you are done. Restore it later to continue the conversation where the tool supports resume. TauShell only claims a resume when the tool confirms it. Sessions that were running when you closed TauShell come back on the next launch.
- **Several sessions.** Run one session per task or repository. Each has its own color so you do not type into the wrong one. Press Ctrl+1..9 or Ctrl+Tab to switch tabs. Ctrl+Shift+P opens the command palette.
- **Know who needs you.** Tab indicators, taskbar flash, and Windows toasts tell you when an agent asks for input or finishes a turn.
- **Queue follow-ups.** Open the prompt queue in the right-side toolbar and add tasks while the agent works. TauShell sends the next one only when the tool is confirmed idle. Press Ctrl+Enter in the terminal to move what you typed into the queue instead of sending it.
- **Saved prompts and skills.** The **Prompts** menu inserts reusable prompts. Skills from [ai-cookbook](https://github.com/mgerdov-igt/ai-cookbook) load into supported tools on launch; toggle them on Home.
- **Review and save work.** The Files drawer has a **Git** tab with only the changed files. **Sync changes** in the top bar stages, commits, pulls with rebase, and pushes, so check the diff first.
- **Open in your IDE.** The top bar offers your installed IDE when the folder has matching project files.

## 7) Recommendations

- Start with one tool and one session. Add complexity after the first success.
- Give each parallel session its own folder, or opt in to a Git worktree at creation so sessions cannot overwrite each other's files.
- Commit or sync before a long autonomous run, so you can review and revert.
- Treat permission prompts as yours to answer. TauShell never answers them for you and never queues prompts into one.
- Keep tool updates on your schedule. TauShell offers updates but never applies them silently.
- Closing the app or a running tab asks first, interrupts gracefully, then force-stops what remains. A graceful stop does not guarantee the conversation can resume.
- Only pair a phone you trust. The Android companion is experimental and off by default; see [Phone setup](android/README.md).

## 8) If something goes wrong

| Problem | Fix |
|---|---|
| SmartScreen or Smart App Control blocks the app | Unblock the ZIP before extracting, or use the tools directly in your own terminal |
| "WebView2 runtime required" | Install the [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) and start again |
| A tool shows "not installed" after installing | Press refresh in the Tools panel; open a new session so it sees the updated PATH |
| Install fails | Use the vendor link in the Tools panel, then refresh |
| A session will not resume | Use **Start fresh**; your usage history is kept |
| Something else | See the diagnostics log location in [Diagnostics](README.md#diagnostics) |

## Next

- [README](README.md): all features and status
- [YAML schema](docs/v2/18-yaml-schema.md): edit sessions by hand
- [WSL sessions](README.md#wsl-sessions-experimental): run a harness inside a WSL2 distro (experimental)
- [ai-cookbook](https://github.com/mgerdov-igt/ai-cookbook): prompts, skills, and tool guides
