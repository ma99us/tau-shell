# V3 Parked and Rejected Ideas

Status: **future planning**. Reasons are recorded so the decisions are not reopened by accident.

## Already implemented (not ranked)

| Idea | Where it exists |
|---|---|
| Fleet restore | Home **Resume All**, per-session `auto_resume`, global "Auto-resume all on startup" |
| Usage and cost tracking | `internal/usage` ledger; per-session, monthly and per-harness totals |
| Tool action abstraction | Toolbar widgets, command palette, `prompts.yaml` |
| Terminal compatibility layer | UTF-8 setup, bundled ConPTY, Unicode and ANSI tests |
| Skills registry | Skills library with update and toggle |
| Tool and app updates, session clone | `GetToolUpdates`, `UpdateTool`, `CheckAppUpdate`, `DuplicateSession` |

## Honorable mentions (candidates after the top 10)

- **Settings UI.** `config.yaml` is edited as a file only today.
- **Link an issue or PR to a session** (GitHub or Jira), shown on the tile. Orca does this
  inside worktree cards; here it would be a label and link only.
- **Multi-account usage and switching** for Claude and Codex. Higher risk: credential handling.
- **Tool-action expansion** for harnesses beyond Copilot and Claude.
- **Per-session notes** and tags.
- **Keyboard reordering** of sessions (accessibility gap noted in the backlog).

## Conflicts with a design decision

| Idea | Why it conflicts |
|---|---|
| Scrollback search and persistence | The terminal uses `scrollback: 0` on purpose; the harness owns scrolling. Reopen only if that decision changes. |

## Rejected (out of scope by user decision or fit)

| Orca or V3 idea | Reason |
|---|---|
| Git worktrees per task, parallel fan-out in isolated checkouts | User chose light git only. Dispatch warns about shared folders instead. |
| Annotate AI diff and diff review | Requires git and diff UI; TauShell is not a review tool. |
| Embedded editor | Not an IDE. The file drawer is read-only. |
| Embedded browser, Design Mode | Not an IDE; large surface and security cost. |
| SSH worktrees, remote hosts | Remote access is limited to the mobile MVP; no remote execution. |
| Computer use, mobile emulator bridge | Outside the session-manager purpose. |
| Public artifact sharing | Sends content to a hosted service; conflicts with local-first. |
| Linear and Jira boards in the app | Issue tracking is not TauShell's job. |
| Time-based schedules and recurring prompts | User chose queue-only dispatch; unattended spend risk. |
| Full REST and WebSocket service for web dashboards | Phase 1 excludes web dashboards; mobile MVP decides its own transport. |
| "Kubernetes for agents" distributed execution | Too far from the local single-user scope. |
| Cloud sync and accounts | Local-first principle. |
