# Session Restoration

TauShell stores editable session configuration separately from changing runtime state. A restored session is a newly launched PowerShell/ConPTY process; it is not the original process or a replay of its terminal buffer.

## Phase 1 flow

1. Load one session definition from its YAML file.
2. Load its persisted harness conversation ID from runtime state, when one is available.
3. Start a new PowerShell 7 ConPTY in the configured working directory and apply the session environment/bootstrap.
4. Invoke that harness adapter's explicit resume command for the persisted ID.
5. Report launch and resume outcomes separately. Unsupported or failed resume must not be represented as a successful conversation restore.

Restoration is explicit by default; opt-in per-session `auto_resume` and
global `auto_resume_all` settings restore eligible sessions on startup. The
CLI exposes `taushell start <session-id>` and `taushell restore <session-id>`.
The Bubble Tea TUI was removed; the local desktop GUI has Home/setup and
embedded terminal tabs. It opens Home by default and auto-resumes only stopped
sessions with a saved harness ID and no resume warning. Otherwise the user
chooses Start or Restore; Start fresh requires confirmation before replacing a
saved ID. Live sessions continue running when their tab is not selected, and
closing a live tab stops it only after confirmation. A running session always
has a tab and sidebar item.

Only TauShell-launched sessions are managed. Concurrent starts of the same
session are rejected using a SQLite write transaction; active state is checked
against the recorded PowerShell process. The UI hides Restore when an adapter
already reports the saved conversation unavailable. If a restore is invoked
and its resume check rejects the ID, the session instead starts a new
conversation and the desktop shows "Could not resume: ... Started a new
conversation instead."; the CLI prints a corresponding warning to stderr.

The managed PowerShell process exits when its harness command exits. Restoration recreates the configured working directory and environment; it does not preserve arbitrary shell variables, functions, or the original PowerShell process.

## Harness resume IDs

Claude Code and Copilot CLI use a TauShell-generated UUID, persisted before
launch; their current manifests pass `--session-id <id>` on start and
`--resume <id>` on restore. GSD uses a TauShell-named session file. Codex and
OpenCode choose their own IDs, which TauShell discovers after launch and claims
only when the match is unambiguous. Do not use a harness's "continue most
recent" behavior for automated restore because it can select a different
conversation.

Earlier help probes on Claude Code 2.1.263 and Copilot CLI 1.0.96-2 established
CLI-level options, not provider-backed restore. The earlier Copilot
implementation used `--session-id` for both start and resume; the current
manifest uses `--resume`. A fake-executable smoke verified persisted-ID reuse
and child environment delivery. A disposable provider-backed Copilot CLI
1.0.100-1 test restored the exact session and recalled a prior response; it
used a temporary working directory and made no file changes. A later
manager/ConPTY run also verified Copilot 1.0.104 recall. Claude Code 2.1.316
restored and recalled a real conversation through the session manager and
ConPTY on 2026-11-02; this was not a full hosted-xterm restore test. See the
[Claude verification results](24-claude-code-verification-plan.md#5b-re-run-with-a-working-login-2026-11-02-claude-code-21316-windows-11)
and [handoff](21-engineering-handoff.md#recommended-next-actions).

## Choosing an earlier conversation

A session that has had several conversations (Start fresh rotates the ID) lists
them under the History icon in the side rail. Resuming a chosen one runs the
harness resume check, records the outgoing conversation's usage, swaps the saved
`harness_session_id` (refused while running or when another session owns that
ID) and restores. If the launch fails the previous ID is put back and the error
is shown; success is still only claimed as far as the harness confirms it. Only
conversations the usage ledger or the saved ID already knows are offered, and
titles come from the harness's own files (Claude Code, Copilot CLI).

## Limits

- A clean Ctrl+C and process shutdown may improve the harness's ability to persist its session but cannot guarantee that in-progress work or uncheckpointed state is saved.
- PowerShell restoration and AI conversation restoration are distinct outcomes.
- Record/report failures and allow a user to retry or start a fresh harness session explicitly.

## WSL sessions

A WSL session restores the same way: a new `wsl.exe` ConPTY process in the same
distro and Linux working directory, never the original process. The harness
resume check, model, title, usage and activity readers use the distro home
learned at launch and only read while the distro is running; a stopped distro
is not started to read state, so final usage is ingested on the next launch. Launch
or restore failures, including an unavailable distro, are reported, not hidden. Harnesses that
discover their own session ID (Codex, OpenCode) are rejected for WSL, and Clone
of a WSL session is refused for them. Design, status and backlog: [`docs/wsl/`](../wsl/).
