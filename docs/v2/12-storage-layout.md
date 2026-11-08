# Storage Layout

Configuration is human-edited YAML; mutable runtime/session state is stored separately.

## Phase 1 subset

```text
~/.tau_shell/
├─ sessions/
│  └─ <session-id>.yaml
│     (temporary <session-id>.yaml.deleting during recoverable deletion)
├─ prompts.yaml          (optional, hand-edited prompt presets)
├─ config.yaml           (optional, hand-edited app settings: notifications, usage)
├─ prices.json           (cached Claude and OpenAI prices fetched from the web; safe to delete)
├─ logs/
└─ state.db              (runtime state, order, usage ledger and deletion journal)
```

## Token and cost usage

`state.db` also holds `app_settings` (UI preferences such as the global
`auto_resume_all` flag), `usage_ledger` (per session, harness conversation, model,
day and log file: input, output, cache-read and cache-write tokens, plus the
exact Copilot USD) and `usage_files` (per log file: read offset and the last
streamed Claude message). Totals are read incrementally from the harness's own
logs (Claude transcripts including subagents; Copilot `events.jsonl`
`session.usage_record`), on demand from the UI and before Start fresh/Delete.
Per-session totals never reset; resumes, restarts and Start fresh keep adding.
Deleting a session renames its rows (`deleted:<id>:<time>`) so it leaves the
session list but still counts in monthly totals (reset on
`usage.month_start_day`). Copilot USD is exact (`totalNanoAiu` / 1e11); Claude
USD is a list-price estimate computed at read time from the price table, so a
price update restates history. Records with no known price are marked
unpriced and shown with a trailing `+`. No prompt text is stored.

The YAML file is the source of intended session configuration. `internal/session/` stores TauShell-managed runtime metadata in SQLite, including harness conversation identifiers and launch/restore outcomes. Identifiers are TauShell-generated for some adapters and discovered after launch for others; see [session restoration](04-session-restoration.md#harness-resume-ids). Session start and discovered-ID claims use immediate SQLite transactions across TauShell processes. Session deletion renames YAML to a same-directory `.deleting` tombstone, then removes runtime/order state, detaches usage history, and records a journal row in one transaction. Startup restores a staged file without a journal or finalizes a journaled deletion. TauShell config creation/updates/deletion use a per-file Windows mutex; updates also recheck file bytes before replacement and return a conflict if an external edit was observed. This is not a cross-process compare-and-swap against editors that do not honor TauShell's mutex. Do not write runtime IDs back into hand-edited YAML.

Named secret values are stored separately in the current user's Windows
Credential Manager, not in this directory or session YAML. YAML may reference
their names for injection into a session environment; see the
[security model](11-security-model.md#secret-vault). A workspace/project
registry, general plugin-package registry, and terminal snapshots remain
later-scope storage.

## Skill library

Under the TauShell data directory: `skills-state.json` (applied commit and
disabled skill names; JSON rather than `app_settings`), `sources/<owner>-<repo>/<sha>/`
(downloaded snapshots), and `plugins/<harness>/<hash>/` (immutable plugin
folders passed to `--plugin-dir`; old ones are garbage-collected).

The UI persists open tab IDs/order/selection, collapsed workspace groups and the
Home "Group by workspace" switch (`taushell.home-grouped.v1`) in WebView `localStorage`; live
xterm buffers and terminal state are process-local and are not persisted across
restarts. Presentation state does not start or restore a session by itself.
The desktop frame defaults to a normal 1280x920 window. Its normal bounds and
maximized state are saved on confirmed close in `window-placement.json` under
the TauShell data directory, separately from YAML and harness runtime fields.
Minimized windows reopen in their previous normal or maximized state. Native
Windows placement restoration adjusts an off-screen window when monitors change.
Missing or invalid placement falls back to the default window.

The desktop opens Home by default; opted-in auto-resume settings may restore
eligible sessions. Persisted tab IDs/order/selection remain presentation state
separate from YAML and harness runtime metadata. See the
[system architecture](01-system-architecture.md); the
[V1 UI design](../v1/08-UI-Design.md) is historical context.

The isolated [`spikes/session-config/`](../../spikes/session-config/) prototype verified strict YAML parsing, runtime UUID persistence across database reopen, and that runtime-state operations leave the original YAML file unchanged. The application now implements this separation; the schema and driver remain subject to Phase 1 validation.
