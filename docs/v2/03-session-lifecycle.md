# Session Lifecycle

TauShell manages persistent session definitions, not terminal tabs. Session
intent is stored in YAML; mutable process and harness-resume state is stored
separately in SQLite. A tab is only a presentation reference to a configured
session and never starts a process by itself.

## Phase 1 runtime states

| State | Meaning |
|---|---|
| `never-started` | No harness conversation has been created for this session. |
| `starting` | TauShell has persisted the launch intent and is creating PowerShell/ConPTY and starting the harness. |
| `running` | The TauShell-managed PowerShell process is alive. This does not claim that the harness is generating a response. |
| `stopped` | The managed process exited normally or was deliberately stopped. |
| `failed` | Launch, terminal transport, restore, or shutdown failed, or startup recovery found an abandoned process record. The error remains visible for recovery. |

Thinking, waiting-for-user, frozen, suspended, snapshot, and archived states
are not part of the Phase 1 contract. The UI must not infer harness activity
from terminal text or whether a tab is selected.

## Start, restore, and fresh start

1. Load and validate the session's human-edited YAML definition.
2. Read runtime state and reconcile a stale process record without replacing
   the saved harness session ID.
3. Persist `starting` and a harness session ID before invoking the adapter.
4. Launch PowerShell 7 through ConPTY in the configured working directory,
   applying the session-scoped environment and UTF-8 setup.
5. Pass configured harness arguments as discrete argv values, followed by the
   adapter-managed session identity and mode-specific arguments.
6. Report process launch and harness restore outcomes separately. A new
   PowerShell process is not proof that the prior conversation was restored.

**Start** creates a harness session only when no saved harness session ID
exists. **Restore** explicitly asks the adapter to resume the saved ID.
**Start fresh** is a separate, user-confirmed operation that replaces the
saved ID before launching a new conversation. Failed launch or restore remains
visible and must not be reported as successful recovery.

Restore starts a new PowerShell/ConPTY process. It does not recreate the prior
PowerShell process, its environment mutations, or its terminal buffer.
Automatic harness restore on application startup is not supported.

## Runtime recovery

The runtime database records the TauShell-managed process ID and last update.
On listing or starting a session, TauShell checks a recorded running process
and reconciles it to `failed` when it is no longer alive. A recent `starting`
record is allowed a short recovery window; an abandoned startup is then marked
failed with an actionable retry message. Reconciliation uses a conditional
update so it cannot overwrite a newer process state, and preserves the saved
harness session ID.

Two TauShell processes cannot start the same configured session concurrently.
The runtime store uses a SQLite write transaction to reject duplicate live
starts. Do not infer process ownership from ancestry or terminate a process
that is not managed by TauShell's ConPTY job.

## Stop and application shutdown

Stopping a session requests graceful interruption, waits for the configured
grace period, and force-terminates remaining processes in TauShell's job when
needed. Forced termination and cleanup failures are reported. If shutdown
fails or times out, the manager remains retryable and the UI must not silently
claim that all sessions stopped.

Before closing with managed sessions active, ask for confirmation. Cancelling
keeps the window and sessions available. Confirming attempts to stop all
managed sessions and reports forced outcomes before closing. Shell or broker
processes explicitly detached outside the job are unmanaged and are not
guaranteed to stop with TauShell.

## CLI recovery

The standalone `list`, `start <session-id>`, and `restore <session-id>`
commands remain available without the desktop UI. They use the same YAML,
runtime store, and harness adapter semantics.
