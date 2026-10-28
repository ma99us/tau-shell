# Storage Layout

# Storage Layout

Configuration is human-edited YAML; mutable runtime/session state is stored separately.

## Phase 1 subset

```text
~/.tau_shell/
├─ sessions/
│  └─ <session-id>.yaml
├─ prompts.yaml          (optional, hand-edited prompt presets)
├─ config.yaml           (optional, hand-edited app settings: notifications, usage)
├─ prices.json           (cached Claude prices fetched from the web; safe to delete)
├─ logs/
└─ state.db              (runtime state, shared session display order, usage ledger)
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

The YAML file is the source of intended session configuration. The initial implementation in `internal/session/` stores TauShell-managed runtime metadata in SQLite, including the harness session UUID generated before first launch and the outcomes of launch/restore attempts. Session start uses an immediate SQLite write transaction to prevent two TauShell processes from launching the same session concurrently. Do not write runtime IDs back into hand-edited YAML.

Secret vault, workspace/project registry, plugin packages, and terminal snapshots remain later-scope storage until their behavior is specified.

The tab manager currently keeps its open tabs and selected tab in memory for
the duration of the process. It does not persist the UI layout across
restarts; adding that UI metadata remains separate from session YAML and
harness runtime fields.

The planned desktop GUI will open to Home and may make the previous tab layout
available without relaunching sessions. Persisted tab IDs/order/selection, if
implemented, remain presentation state separate from YAML and harness runtime
metadata. See [`docs/v1/08-UI-Design.md`](../v1/08-UI-Design.md).

The isolated [`spikes/session-config/`](../../spikes/session-config/) prototype verified strict YAML parsing, runtime UUID persistence across database reopen, and that runtime-state operations leave the original YAML file unchanged. The application now implements this separation; the schema and driver remain subject to Phase 1 validation.
