# V3 Cross-Cutting Rules, Findings and Spec Template

Status: **future planning**. Nothing here is a current requirement. Phase 1 scope in
[`AGENTS.md`](../../AGENTS.md) and [`docs/v2/`](../v2/) remains authoritative.

## 1. Start gate

V3 work starts only after the remaining Phase 1 verification items are closed, chiefly:

- Claude Code checks that need a model reply: restore after a completed login, attention
  cues, widgets, `/model` and Esc interrupt. See
  [`24-claude-code-verification-plan.md`](../v2/24-claude-code-verification-plan.md).
- Live smoke tests not yet run in the packaged window: attention toast and flash, drag
  reorder, the usage top bar and a winget install. See
  [`21-engineering-handoff.md`](../v2/21-engineering-handoff.md).
- The fix for environment values leaking on the command line (section 2.1).

## 2. Critical findings from the plan review

These findings come from checking the V3 ideas against the code. Feature specs reference
them by number.

### 2.1 Environment values are visible on the process command line (Phase 1 bug)

`harness.PowerShellInvocation` (`internal/harness/adapter.go`) writes each session
`environment:` entry as `$env:NAME = 'value'` into a script. The session host passes that
script as an argument: `pwsh -NoLogo -NoProfile -Command <script>`
(`internal/sessionhost/manager_windows.go`). Any process running as the same user can read
process command lines, for example with `Get-CimInstance Win32_Process`, and so can read the
values.

- This is a Phase 1 fix, tracked in [`20-mvp-backlog.md`](../v2/20-mvp-backlog.md) and
  [`11-security-model.md`](../v2/11-security-model.md).
- Expected direction, not a design: pass values through the child process environment
  block, not through argv.
- The secret vault ([feature 08](features/08-secret-vault.md)) **depends on** this fix.
  Without it, vault secrets would leak the same way.

### 2.2 Programmatic input reaches agents that run without permission checks

TauShell's default launch templates use `--yolo` (Copilot CLI) and
`--dangerously-skip-permissions` (Claude Code). Anything that can type into a session can
therefore run commands as the user. The control API, prompt dispatch and mobile quick reply
must:

- be **opt-in** and off by default;
- accept callers from the **current Windows user only**;
- show every programmatic input in the UI with its source and time, as an audit trail that
  leaves the text out of logs;
- never write prompt text or terminal content to diagnostics.

### 2.3 Idle detection is a heuristic

Working, idle and waiting states come from screen and title cues
(`frontend/src/attention.ts`). A quiet terminal does not prove the harness is ready for a new
prompt. Automatic sending in prompt dispatch therefore depends on
[feature 02](features/02-reliable-state-and-inbox.md). Until feature 02 exists, queued items
need a manual "send next".

### 2.4 First network surface

Apart from update and price checks, the mobile companion would be the first TauShell
component to talk to anything beyond localhost. It needs device pairing, transport
encryption, a way to revoke devices, and explicit consent before any session content passes
through a third-party relay.

### 2.5 Undocumented harness formats

Several features depend on harness log files, CLI output or internal endpoints that the
vendors do not document: conversation history, resume checks declared in adapter manifests,
quota readers, and the existing usage and model readers. Each of these readers needs:

- a tested version range and a capability probe;
- a visible "unavailable" state instead of a silent empty result;
- no claim of success that the harness has not confirmed.

### 2.6 The terminal keeps no scrollback, by design

The terminal sets `scrollback: 0`, and the harness owns scrolling (see the README and the
handoff). Orca-style scrollback search and persistence would conflict with this, so they are
rejected ([parked and rejected](03-parked-and-rejected.md)).

### 2.7 Restore truthfulness

The rule "do not claim a session was restored unless the harness confirms it" applies to
every new resume path: adapter manifests, resuming from history and group resume. Failed and
unsupported resumes stay visible for each session.

### 2.8 Windows-specific mechanisms

DPAPI and Credential Manager, toasts, `FlashWindowEx`, named pipes, ConPTY, WebView2 and
winget exist only on Windows. New V3 work keeps OS-specific code behind small interfaces so
the portability track stays possible. This is not a promise of macOS or Linux support.

## 3. Portability track (not ranked)

Groundwork only: whenever a feature touches the OS, leave a seam that a macOS or Linux host
could implement later.

| Area | Windows today | Possible macOS/Linux equivalent |
|---|---|---|
| PTY | ConPTY | Unix PTY |
| Local IPC | Named pipe | Unix socket |
| Secret store | Credential Manager | Keychain or libsecret |
| Notifications | Windows toast | Platform notifications |
| Tool installs | winget | brew or apt |

PowerShell 7 already runs cross-platform. There is no platform commitment until a separate
decision is made.

## 4. Security and privacy rules for all V3 features

- Keep secret values out of YAML, argv, logs, toasts, mobile pushes and the UI renderer.
- Diagnostics never record prompt text, terminal bytes, environment values or credentials.
  This is unchanged from Phase 1.
- Every new listener, IPC endpoint or outbound call gets a section in
  [`11-security-model.md`](../v2/11-security-model.md) before it ships.
- External links open through the bound Go opener, which is restricted to known URLs. Never
  use `target="_blank"`.
- Never install, elevate or auto-update anything silently.

## 5. Spec template

Each feature file in [`features/`](features/) has these sections:

1. **Problem**: what hurts today, with evidence from the current implementation.
2. **User stories**: "As a TauShell user, I want ..., so that ...".
3. **In scope** and **Out of scope**.
4. **Acceptance criteria**: observable, testable outcomes, not implementation steps.
5. **Dependencies**: other features, the Phase 1 gate, and the findings above.
6. **Risks**.
7. **Spikes**: bounded experiments to run before design.
8. **Open questions**.
9. **Docs to update when implemented**.
10. **Sources**: the V3 ideas summary, Orca, or the user.

Specs describe *what* and *why*. Technical design happens when a feature is scheduled.
