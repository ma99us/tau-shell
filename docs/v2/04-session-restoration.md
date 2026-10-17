# Session Restoration

TauShell stores editable session configuration separately from changing runtime state. A restored session is a newly launched PowerShell/ConPTY process; it is not the original process or a replay of its terminal buffer.

## Phase 1 flow

1. Load one session definition from its YAML file.
2. Load its persisted harness session UUID from runtime state.
3. Start a new PowerShell 7 ConPTY in the configured working directory and apply the session environment/bootstrap.
4. Invoke that harness adapter's explicit resume command for the persisted UUID.
5. Report launch and resume outcomes separately. Unsupported or failed resume must not be represented as a successful conversation restore.

Restoration is explicit; TauShell does not automatically relaunch sessions on startup. The CLI exposes `taushell start <session-id>` and `taushell restore <session-id>`. The current interactive Bubble Tea TUI is a disposable prototype scheduled for removal; the planned primary and sole interactive UI is a Windows desktop GUI with Home/setup and embedded terminal tabs. In the target UI, startup shows Home with prior tab metadata available but does not start or restore harnesses. The user explicitly chooses Start or Restore; when a saved harness ID exists, Start fresh is also offered and requires confirmation before replacing that ID. Live sessions continue in the background when their tab is hidden, and closing a live tab asks whether to stop or retain it in the background. Only TauShell-launched sessions are managed. Concurrent starts of the same session are rejected using a SQLite write transaction; active state is checked against the recorded PowerShell process.

The managed PowerShell process exits when its harness command exits. Restoration recreates the configured working directory and environment; it does not preserve arbitrary shell variables, functions, or the original PowerShell process.

## Harness resume IDs

Generate a UUID for each managed harness conversation and persist it in runtime state before the first harness launch. Pass it to the harness on initial launch so it can later be resumed by that exact ID. Do not use a harness's “continue most recent” behavior for automated restore because it can select a different conversation.

The locally installed Claude Code 2.1.263 and GitHub Copilot CLI 1.0.96-2 help output documents explicit session creation/resume options. This confirms the CLI-level capability, not that a remote-backed conversation was successfully resumed. End-to-end behavior still needs a deliberate integration test that does not silently send prompts or modify user work.

The initial implementation passes `--session-id <uuid>` on start and `--resume <uuid>` on restore for Claude Code, and `--session-id <uuid>` for both new and resumed Copilot CLI sessions. An interactive smoke test with a fake executable verified that start and restore use the same persisted UUID and that session environment values reach the child process. A separate disposable provider-backed test with Copilot CLI 1.0.100-1 confirmed that TauShell's `restore` command reopened the exact session: after a no-tools prompt returned `TAUSHELL_RESTORE_SMOKE_OK`, the resumed conversation returned the same token when asked about its prior response. The test used a temporary working directory and made no file changes. Claude Code provider-backed restoration remains unverified.

## Limits

- A clean Ctrl+C and process shutdown may improve the harness's ability to persist its session but cannot guarantee that in-progress work or uncheckpointed state is saved.
- PowerShell restoration and AI conversation restoration are distinct outcomes.
- Record/report failures and allow a user to retry or start a fresh harness session explicitly.
