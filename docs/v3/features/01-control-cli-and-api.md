# 01 - Control CLI and Local API

Status: **future planning**. Rank 1. Sources: Orca CLI; user decision (opt-in, current user only).

## Problem

TauShell's CLI has only `list`, `start` and `restore`. They run in their own console and
cannot reach sessions that are open in the desktop app. Scripts, other agents and the
mobile companion therefore cannot read a session's state, send it a prompt or wait for it to
finish. Orca's CLI (`terminal list/read/send/wait --for tui-idle`, `--json`) shows the
value of letting agents and scripts drive the host.

## User stories

- As a user, I want to script my sessions (`list`, `start`, `stop`, `send`, `read`, `wait`)
  from any PowerShell, so that I can automate repeated workflows.
- As a user, I want an agent running in one session to hand work to another session, so that
  a planner can feed an implementer. This must be **explicitly enabled**.
- As a user, I want machine-readable (`--json`) output, so that scripts do not parse text.
- As a user, I want to see every programmatic action in the app, so that I know what was
  typed into my sessions and by whom.

## In scope

- A local control channel to the **running** TauShell app, usable by the CLI.
- Commands: list sessions and their state; start, restore and stop; send text (with or
  without submit); read a bounded recent transcript; wait until a session is idle or has
  ended, with a timeout.
- `--json` output and stable exit codes.
- A setting that is **off by default** to enable the channel, and a separate setting for
  whether agents running inside TauShell sessions may use it.
- An audit view of programmatic input: source, session, time. No prompt text in logs.
- An optional TauShell skill that teaches agents the commands (only when enabled).

## Out of scope

- Any network listener; remote access belongs to [feature 04](04-mobile-companion.md).
- Creating or deleting session definitions from the API (phase two question below).
- Interrupting a running turn on behalf of another session.
- Driving the embedded UI itself (clicks, screenshots).

## Acceptance criteria

- With the channel disabled, no CLI command can reach the app, and the CLI says why.
- With it enabled, a process of the **same Windows user** can list sessions and see the same
  state the UI shows; a process of another user cannot connect.
- `send` to a session that is not idle returns a clear refusal unless the caller asks for
  insert-only; nothing is typed into a waiting or permission prompt.
- `wait --for idle --timeout` returns success only on a confirmed idle state, and times out
  with a distinct exit code otherwise.
- Every programmatic send appears in the audit view with its source; logs contain no prompt
  text or terminal content.
- Agent-initiated calls are rejected unless the "agents may drive sessions" setting is on.
- `list` and `start` keep working as they do today when the app is not running.

## Dependencies

- Start gate and [finding 2.2](../02-cross-cutting.md#22-programmatic-input-reaches-agents-that-run-without-permission-checks).
- [Feature 02](02-reliable-state-and-inbox.md): a trustworthy idle state for `wait` and `send`.
- Consumers: [03](03-prompt-dispatch.md), [04](04-mobile-companion.md).

## Risks

- Programmatic input into agents running with skipped permissions is equivalent to remote
  command execution as the user. Mitigation: opt-in, same-user only, audit.
- How to tell an agent caller from a user script is not reliable. The setting may have to
  gate the whole channel rather than distinguishing callers.
- Transcript reads can expose secrets that appear in terminal output.

## Spikes

- **Done: same-user-only local channel (named pipe versus loopback TCP + token).**
  Run on Windows (Entra-joined account, not Windows 10 specifically), Go + `x/sys/windows`;
  a throwaway program in the ignored `spikes/ctlchan`.

  | Experiment | Result |
  |---|---|
  | Pipe, DACL = current user only, `FILE_FLAG_FIRST_PIPE_INSTANCE`, `PIPE_REJECT_REMOTE_CLIENTS` | DACL read back as exactly one ACE for the user SID. Same-user client connected. |
  | Pipe name squatting (second first-instance on the same name) | Refused (access denied). A malicious pre-created pipe is only possible if it exists before TauShell starts, so TauShell must fail loudly if the first instance cannot be created. |
  | Same-user low-integrity client (sandboxed process) to the pipe | Denied by the OS (error 5), even when the DACL grants Everyone. |
  | Loopback TCP with a token, same-user and low-integrity clients | Both connected. Only the token stops a request; the OS does not. Any same-user process that can read the token file/env gets in. |
  | Caller identification via `GetNamedPipeClientProcessId` + parent-process walk | Works for a live descendant chain. If the parent exits first, the chain is cut to one dead PID, so ancestry says "unknown". |

  Conclusions: (1) use a named pipe with an explicit current-user DACL and first-instance
  creation; it gives OS-enforced identity and also excludes sandboxed processes, which TCP
  cannot. (2) Treat the caller PID as a hint only: ancestry is spoofable by re-parenting,
  breaks on orphans and PID reuse, so "agent inside TauShell" must not be a security boundary.
  Use an explicit per-session token injected into the session environment (enabled by the
  opt-in setting) to authorize agents, and treat same-user programs as trusted by design.
  (3) Loopback TCP is only justified for the mobile transport and then needs its own pairing
  auth (feature 04).
  Not tested: a different Windows user account (the pipe DACL is expected to block it, but
  this was not observed), elevated versus non-elevated clients, and a Windows 10 machine.
- How the CLI finds the running app, including several TauShell windows.
- What "recent transcript" can be read without keeping scrollback (the terminal has none).

## Open questions

- Should the CLI be able to create sessions, or only drive existing ones?
- One instance or several app windows: which one answers?
- Does `read` return screen text or the raw stream?

## Docs to update when implemented

README (CLI section), `11-security-model.md`, `18-yaml-schema.md` (settings), the contributor guide.
