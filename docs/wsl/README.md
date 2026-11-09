# TauShell for WSL - investigation and plan

Status: investigation and planning only. Nothing here is implemented, and nothing
here is a requirement for the current Phase 1 scope (see `AGENTS.md`). Like
`docs/v3/`, treat these documents as future-only planning unless asked.

Some engineers do not want the PowerShell 7 dependency and prefer to run their
AI harnesses inside WSL2. This folder answers: how hard would a WSL-based
TauShell be, and how should it be built?

| Document | Contents |
|---|---|
| [`01-feasibility.md`](01-feasibility.md) | Probe results from a real WSL2 machine, the PowerShell/Windows coupling inventory, and the product-shape comparison |
| [`02-design.md`](02-design.md) | Proposed design: runtime abstraction, paths, environment, process lifetime, git, detection, skills, YAML, UI |
| [`03-implementation-plan.md`](03-implementation-plan.md) | Phases, effort sizes, risks, verification plan, open questions |
| [`04-wsl-as-a-tool.md`](04-wsl-as-a-tool.md) | WSL as a managed Tool: Tools panel card, version tracking, installation assistance (adds Phases 0b and 3.5) |
| [`05-spike-results.md`](05-spike-results.md) | Measured results from the Phase 0 spike on a real WSL2 machine, what is still unverified, and the decisions they change |
| [`06-backlog.md`](06-backlog.md) | Verification backlog: WSL gaps per harness (Codex, GSD, OpenCode) and per platform area, with what is left to close each |

## Summary

- **Feasible, and moderate in effort.** The terminal stack (ConPTY, xterm.js),
  storage (YAML intent, SQLite runtime), attention cues, the prompt queue, the
  Git tab, the UI and the phone server do not depend on PowerShell. `wsl.exe`
  runs under ConPTY exactly as `pwsh.exe` does.
- **The launch itself is one code path** (`sessionhost` launch plus the CLI
  `start` command). That part is small.
- **The real cost is everything that assumes the harness runs on Windows**:
  harness log/usage/resume readers under the Windows home folder, harness
  detection and install, git, environment and secrets, skills paths, the files
  drawer, and killing the process tree. About a dozen subsystems need a
  "where does this session live" answer.
- **Recommendation: a per-session runtime in the same app, not a separate
  build.** Add a `runtime` to the session config (`powershell` default, or
  `wsl` with a distro). PowerShell becomes optional (required only when a
  PowerShell session is used or the user has no WSL). A separate WSL-only build
  doubles the release, update, test and smoke pipeline for little gain, and
  forks the code that the abstraction would otherwise share. A WSL-only
  *packaging profile* (same binary, setup page that does not ask for PowerShell)
  can be added later if wanted.
- **Rough size:** a usable first slice (launch, resume, detection, status,
  usage, git) is a medium-to-large feature (a spike plus six phases, mostly Go
  work in `internal/harness`, `internal/sessionhost`, `internal/presets`,
  `internal/tools`, plus a small UI change). Hardening (process cleanup, edge
  cases, docs) is a further medium chunk.
- **Rule change needed if adopted:** `AGENTS.md` says "Keep PowerShell
  first-class; do not replace it". The plan keeps PowerShell first-class and adds
  WSL as a second runtime, but the Phase 1 scope text and "Detect PowerShell 7"
  setup rule must be updated deliberately.

## What was and was not verified

Verified on one machine (Windows 10.0.26200, WSL 2.6.2, Ubuntu distro):
`wsl.exe` status and listing, environment forwarding through `WSLENV`, path
translation, working-directory translation, exit-code propagation, UNC access
to the distro, `--exec` argument passing, that killing `wsl.exe` also ends its
Linux child, and the PATH-interop pitfall.

Not verified: a live harness session inside WSL, ConPTY hosting of `wsl.exe`
inside TauShell itself, resize/Unicode/clipboard behavior through `wsl.exe`,
older WSL builds, WSL1, multiple users, and corporate lockdown scenarios. The
plan starts with a spike for these.
