# V3 Research and Gap Analysis

Status: **future planning**. Nothing here is a current requirement.

## 1. Sources

### 1.1 TauShell V3 ideas summary

[`docs/V3_Ideas_Summary (out of scope future).md`](../V3_Ideas_Summary%20(out%20of%20scope%20future).md)
describes TauShell growing from a session manager into a local "AI workspace orchestrator".
Its themes:

1. Workspace-centric design: groups of sessions restored as a unit.
2. AI fleet restoration: restore everything with one click after a reboot.
3. Secret vault: `secret:` references instead of plaintext API keys.
4. Tool action abstraction: harness-independent buttons such as model and compact.
5. Terminal compatibility layer: UTF-8, ANSI, fonts, emoji, bootstrap.
6. Agent state machine: Working, Waiting For Input/Approval, Failed and similar states.
7. Rich mobile companion: overview, live view, prompts, notifications.
8. Intelligent attention detection: several signals combined.
9. Project-aware workspaces: sessions organized around repositories.
10. Vision: "Kubernetes for local AI coding agents".

The summary also lists deliverables such as REST and WebSocket specs, an SQLite schema and a
plugin SDK. Those are design artifacts, deferred until features are scheduled.

### 1.2 stablyai/orca

[Orca](https://github.com/stablyai/orca) is an "AI orchestrator" IDE (MIT license, Electron,
macOS/Windows/Linux). It runs many CLI agents side by side, each in its own git worktree.
The table summarizes the features we reviewed in its README and
[docs](https://www.onorca.dev/docs).

| Orca feature | What it does |
|---|---|
| Parallel worktrees | Gives each task a `git worktree` with its own branch and terminals; sends one prompt to several agents so the user can keep the best result |
| Mobile companion | iOS/Android app that notifies when an agent finishes and sends follow-ups |
| Terminal | xterm.js with splits, WebGL, scrollback kept across restarts, find, link actions, OSC 52, copy context, reset terminal, file drop |
| Notifications | Pings when an agent finishes; a bell with unread items across worktrees; mark unread; per-category and custom sounds |
| Usage tracking | Per-provider usage against plan limits, reset times, 80% warning, multi-account switcher, cost estimates |
| Orca CLI | `orca worktree/terminal list/read/send/wait --for tui-idle`, `--json`; an agent skill so agents can drive Orca |
| Annotate AI diff | Line comments on agent diffs, sent back to the agent as one batch |
| GitHub/Linear/Jira | Browse PRs and issues in the app; link them to worktrees |
| SSH worktrees | Agents on remote machines, with port forwarding |
| Editor, file explorer | VS Code editor; drag files or images into prompts; Markdown and PDF preview |
| Browser, Design Mode | Embedded Chromium; click an element to send its HTML/CSS and a screenshot to the agent |
| Automations | Scheduled recurring prompts against a repo or worktree |
| Quick open | Search across worktrees, files, agents and commands |
| Many agents | Works with "any CLI agent"; lists more than 30 |

### 1.3 Fit lens

TauShell principles (from [`AGENTS.md`](../../AGENTS.md)):

- PowerShell is first-class.
- TauShell models persistent **sessions**, not tabs.
- The core is tool-independent; harness-specific behavior lives in adapters.
- Restoring the process is separate from resuming the conversation, and a resume is never
  claimed without confirmation.
- Interruptions are visible.
- YAML holds intent; runtime state is stored separately.
- No secrets in configuration or logs.

User decisions for V3:

- Light git awareness only.
- Remote and mobile access are in scope.
- A simple file tree, Markdown viewer and copy-path, but **not an IDE**.
- Windows first.
- Agent automation is allowed when the user opts in.

Orca is an IDE built around worktrees; TauShell is a session manager. We adopt Orca's ideas
about controlling sessions, getting attention and dispatching prompts. We leave out its IDE,
git review and remote-host features.

## 2. Status of candidate ideas (checked against the code)

Status values: **Done** exists today. **Partial** exists in part. **Gap** is missing.
**Conflicts** contradicts a design decision. **Out** was excluded by a user decision.

| Idea | Source | Status | Evidence |
|---|---|---|---|
| Fleet restore after reboot | V3 #2 | **Done** | Home **Resume All**; per-session `auto_resume`; global "Auto-resume all on startup" (`README.md`, `sessionhost` `AutoResumeAll`, `app_settings` in `state.db`) |
| Usage and cost tracking | Orca | **Done** (no quota windows) | `internal/usage` ledger with tokens and cost per session, month and harness (`frontend/src/usage.ts`) |
| Tool action abstraction | V3 #4 | **Done** | Toolbar (Prompts, Commands, Model, Compact, Interrupt), Ctrl+Shift+P palette, `prompts.yaml` ([handoff](../v2/21-engineering-handoff.md#harness-widgets-implemented)) |
| Terminal compatibility layer | V3 #5 | **Done** | UTF-8 code pages, bundled ConPTY, Unicode/ANSI tests ([02-conpty](../v2/02-conpty-integration.md)) |
| Skills registry | Orca | **Done** | Skills library with update and per-skill toggle (`cmd/taushell/skills_windows.go`) |
| Tool and app updates | Orca | **Done** | `GetToolUpdates`, `UpdateTool`, `CheckAppUpdate` |
| Session clone | Orca (fork) | **Done** | `DuplicateSession` |
| Agent state machine and attention | V3 #6, #8 | **Partial** | Working, quiet, unseen, waiting and failed icons, inferred from screen and title cues (`frontend/src/attention.ts`) |
| Notifications | Orca | **Partial** | Toast, taskbar flash and click-to-open exist; no inbox, sound, per-session mute or tray icon ([22-doc](../v2/22-local-attention-notifications.md)) |
| Git awareness (light) | user | **Partial** | `GetGitBranch` shows the branch in the top bar; no dirty or ahead/behind status |
| Quota and rate-limit awareness | Orca | **Partial** | Research note only ([backlog](../v2/20-mvp-backlog.md#research-copilot-quota-chip-not-started)) |
| Quick open | Orca | **Partial** | The palette searches commands, prompts and sessions, but not files |
| Conversation history | Orca | **Partial** | Harness logs are already parsed for usage and model (`SessionUsage.conversations`); no browsing UI |
| Declarative harness support | V3 plugin SDK, Orca | **Partial** | 5 adapters written in Go; [`23-adding-a-harness.md`](../v2/23-adding-a-harness.md) still requires Go code |
| Workspaces / project grouping | V3 #1, #9 | **Partial** | One flat, ordered list; the only group action is Resume All |
| Control CLI/API for agents | Orca CLI | **Gap** | The CLI has only `list`, `start` and `restore`; they run in their own console and cannot reach sessions open in the app |
| Prompt queue | Orca (automations) | **Gap** | Presets and idle-gated submit exist and can be reused |
| Broadcast and side-by-side view | Orca | **Gap** | One terminal per tab |
| Mobile companion | V3 #7, Orca | **Gap** | None (outside Phase 1 scope) |
| File drawer, Markdown viewer, copy path | user, Orca | **Gap** | None |
| Secret vault | V3 #3 | **Gap** | `environment:` values are plaintext in YAML (no longer visible on the command line; [finding 2.1](02-cross-cutting.md#21-environment-values-are-visible-on-the-process-command-line-phase-1-bug-fixed) fixed) |
| Portability | user | **Gap** | The host runs on Windows only |
| Scrollback search and persistence | Orca | **Conflicts** | `scrollback: 0` by design; the harness owns scrolling |
| Worktrees, diff review, editor, browser, SSH | Orca | **Out** | User decision; see [parked and rejected](03-parked-and-rejected.md) |

## 3. Ranking

Features are ranked by **user value**, then **fit with principles**, then **risk**, then
**effort**. Dependencies affect the delivery order, not the rank. The result is in the
[README](README.md#top-10).
