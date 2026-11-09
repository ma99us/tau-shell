# ADR 0001: Per-session runtime (PowerShell or WSL)

Status: Proposed (drafted from the WSL investigation and implementation; needs
the decision owner's approval before it is treated as Accepted).
Date: 2026-11-09.

## Context

Some engineers do not want the PowerShell 7 dependency and prefer to run their AI
harnesses inside WSL2. The harness, git, skill and file surfaces all assumed a
Windows PowerShell 7 process hosted through ConPTY. The investigation is in
[`docs/wsl/`](../wsl/README.md) (feasibility, spike results, design).

## Decision

Add a per-session `runtime` (`powershell`, the default, or `wsl` with a distro)
to the session config and implement it behind a small `internal/runtime` seam
(`Kind`, `Available`, `Command`, `HomeDir`). One app, one release. PowerShell
stays first-class and the default; PowerShell 7 becomes required only when a
PowerShell session is used or the user has no WSL. Clone freezes the original
session's runtime and workspace path.

## Alternatives considered

- A separate WSL-only build: doubles the release, update, test and smoke
  pipeline and forks code the abstraction would otherwise share. A WSL-only
  packaging profile (same binary, setup that does not ask for PowerShell)
  remains possible later.
- Replacing PowerShell with WSL or a custom shell: rejected by the AGENTS.md rule
  that PowerShell stays first-class.

## Consequences

- Every subsystem that read the Windows home, `exec.LookPath` or `git -C` must
  take the session's runtime; session-scoped readers use the distro home learned
  at launch and only read while the distro runs.
- Harness support differs per runtime: Claude Code and Copilot CLI are verified
  in WSL; Codex, OpenCode and GSD are not (see the backlog).
- The runtime is experimental. Unverified areas and gaps are tracked only in
  [`docs/wsl/06-backlog.md`](../wsl/06-backlog.md).
- The Phase 1 scope text and the "Detect PowerShell 7" setup rule in AGENTS.md
  were updated deliberately for this decision.
