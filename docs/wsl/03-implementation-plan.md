# 03 - Implementation plan

No dates. Sizes: S small, M medium, L large. Each phase leaves the app working
and PowerShell behavior unchanged. A phase gate is a set of checks that must
pass before the next phase starts.

## Phase 0 - Spike (prove the risky claims first) - M

Status: partly run. Results and the remaining items are in
[`05-spike-results.md`](05-spike-results.md).

Throwaway program, not product code.

1. Host `wsl.exe -d <distro> --exec <native harness or bash>` in the existing
   ConPTY fixture (`cmd/terminal-fixture`, `internal/conpty`). Verify input,
   resize, Ctrl+C, Unicode and box drawing, bracketed paste, 256-color, exit codes.
2. Run each harness (Claude Code, Copilot CLI, GSD, Codex, OpenCode) natively in
   a distro; record whether the manifest cues work and where logs live.
3. Kill `wsl.exe` while a harness has subprocesses (shell tools, MCP servers,
   dev servers); list survivors and decide the cleanup handshake.
4. Measure UNC read cost of tailing large `events.jsonl` and listing session folders.
5. Test a stopped distro, a missing distro, a systemd failure, `wsl --shutdown`
   during a session, and Windows sleep and resume.

Exit: results table appended to `01-feasibility.md`; decide the termination
handshake and whether UNC reads are acceptable.

## Phase 0b - Install-flow spike - S/M

WSL is also a managed Tool with installation help; see
[`04-wsl-as-a-tool.md`](04-wsl-as-a-tool.md). On a disposable VM or a machine
without WSL, record `wsl --install`, `--update`, `--list --online` and winget
behavior: elevation, reboot, UAC, output encoding and localization. Exit: the
install steps in 04 are confirmed or corrected.

## Phase 1 - Runtime seam, no behavior change - M

Status: done for launch. `internal/runtime` has `Runtime` (`Kind`, `Available`, `Command`, `HomeDir`) and `powershellRuntime`; `sessionhost` launch and CLI `start` use it. Tool lookup and git are not routed yet (they move in Phase 3.5 and 5, when a second runtime exists).

- Add `internal/runtime` with `Runtime` and `powershellRuntime`.
- Route `sessionhost` launch, `cmd/taushell` `start`, tool lookup and git
  through it; the PowerShell implementation reproduces today's code exactly.
- Tests: existing suites unchanged and green; runtime unit tests with a fake
  runtime (launch argv, `HomeDir`, path mapping).
- Gate: `go test ./...`, golden fixtures unchanged, `gofmt -l cmd internal` clean.

## Phase 2 - Home and path abstraction for state readers - M

Status: done as a single seam. All harness and usage readers call `harness.HomeDir` (a replaceable var). Per-session threading (a root passed from the session's runtime) is deferred to Phase 3, when a second runtime exists; the test `home_test.go` proves readers follow an alternate home. `quota`, `manifest/validate` and `cmd/taushell` home uses are unchanged.

- Replace direct `os.UserHomeDir()` use in harness, usage and quota readers with
  a per-session root from the runtime.
- Keep every `...In(root, ...)` seam; add tests with a temp directory acting as a distro home.
- Gate: reader tests parametrized over two roots.

## Phase 3 - WSL runtime: launch and lifetime - L

Status: done for launch and lifetime (`internal/runtime/wsl.go`, `sessionhost/launch_target_windows.go`, `session.Config` `runtime`/`wsl.distro`). Live-verified on Ubuntu with Claude Code and Copilot CLI launch, env forwarding and marker cleanup. Codex and OpenCode (discovered session IDs) are rejected for WSL; GSD is unverified. Not done: resume/model/title/usage readers for the distro home (resume check is skipped, so WSL sessions always start fresh), git, files drawer and skills on Linux paths. CLI `start` rejects WSL sessions.

- `wslRuntime`: availability, distro list (UTF-16 decode), user lookup, command
  build with `--exec`, environment plus `WSLENV`, path conversion, cleanup helper.
- Config: `runtime` and `wsl.distro` with validation; YAML schema docs; round trip; reject unsafe values.
- Launch, restore and resume through the runtime; runtime-neutral exit and
  failure messages; forced-stop cleanup reported visibly.
- Tests: a fake `wsl.exe` test helper for argv, env and exit codes; config validation; cleanup logic.
- Gate: live check on a real distro with one harness (opt-in `TAUSHELL_LIVE`
  extended with runtime selection).

## Phase 3.5 - WSL as a Tool: detection and inventory - M

Status: done as a separate readiness model, not a `tools.Specs()` entry (winget install of WSL needs UAC and a reboot; adding it would change the Tools panel and golden spec). `runtime.Detect` (`readiness.go`) is exposed through the `GetWSLStatus` bridge method; the Tools panel card is Phase 4.

- `tools.WSL` spec, UTF-16 probe reader, structured version and distro-list
  parsers (never match localized prose), readiness checks, distro inventory
  service and a bridge method for the Tools panel.
- Tests: fake `wsl.exe` output fixtures (UTF-16, several locales, stub-only,
  feature-off, no distro, stopped distro).

## Phase 4 - Detection, setup and create-session UI - M

- Tools panel WSL card: version, kernel, WSLg, default distro, distro list and
  state, readiness checklist, warnings, "update available".
- Confirmed install, distro install and update flows (UAC-confirmed, no silent
  elevation, no automatic reboot, pending-restart state); guidance-only
  harness installs inside the distro.

- Detection inside the distro with `/mnt/*` rejection; versions and capability
  probes through `Runtime.Run`.
- Setup and Home: WSL readiness row; PowerShell no longer blocks Home when only
  WSL sessions exist; guidance-only install instructions with a copy button.
- Create-session dialog: runtime and distro picker; `/mnt/c` working-directory warning.
- Tests: frontend tests for gating and rows; Go detection tests with a fake runtime.

## Phase 5 - Git, files drawer, paths, skills - M

- Git functions through `Runtime.Run`; Git tab and sync UI unchanged.
- Files drawer and reveal over `\\wsl.localhost` paths with containment checks.
- Path insertion and pasted-image paths converted with `ToNativePath`.
- Skills: Linux-visible directories; capability gating kept.
- Tests: git parsing unchanged; new path mapping tests; drawer containment on UNC-shaped paths.

## Phase 6 - Hardening and docs - M

- Edge cases: distro stopped mid-session, `wsl --shutdown`, Windows sleep, locale.
- Update `AGENTS.md` scope and architecture rules, `docs/v2` (architecture,
  lifecycle, bootstrap, YAML schema, environment, security model, harness
  guide) and the README.
- Extend the e2e smoke test so the WSL setup row renders (no distro required).
- Gate: the full release checklist (`go test ./...`, `npm test`, `tsc`,
  `check:tokens`, smoke test).

## Effort and risk summary

| Phase | Size | Main risk | Mitigation |
|---|---|---|---|
| 0 Spike | M | `wsl.exe` under ConPTY misbehaves (resize, Unicode, paste) | Do first; adjust or stop before coding |
| 1 Runtime seam | M | Regressions in the PowerShell path | No behavior change; golden tests |
| 2 State readers | M | UNC slowness, stopped distro | Bounded tail reads; "no data" when stopped |
| 3 WSL launch | L | Orphaned Linux processes, quoting bugs | `--exec`, survivor cleanup, fake-wsl tests |
| 4 Detection and UI | M | `/mnt` PATH shims reported as installs | Reject `/mnt/*`; show the reason |
| 5 Git, files, skills | M | `safe.directory`, permissions on `/mnt/c` | Run git in the distro; copy fallback for skills |
| 6 Hardening and docs | M | Rule drift in docs | Update `AGENTS.md` and `docs/v2` in the same change |

## Verification plan

Automated:
- Go unit tests with a fake `wsl.exe` (argv, env, `WSLENV`, exit code, encoding).
- Reader tests with a temp "distro home" root.
- Config validation tests (unsafe distro and path values, option-like input).
- Frontend tests for gating, rows and the create dialog.
- Golden fixtures regenerated only for intended manifest changes.

Live (opt-in; disposable directories; never print credentials; clean up
transcripts and temp files):
- `TAUSHELL_LIVE=<harness>` extended with a runtime selector.
- Manual matrix: each harness x {fresh, resume, queue send, Git sync, usage
  totals, attention cue, forced stop, close with a running session}.
- Rendering in a real window (screenshots): Unicode and box drawing, wide
  characters, bracketed paste, resize, clipboard text and image.

Not verifiable in CI: real distros, Windows sleep and resume, corporate WSL
policies. These stay manual and are recorded in the engineering handoff.

## Open questions for the product owner

1. Is the goal "PowerShell optional" (recommended) or a distributed WSL-only
   package? The plan supports a later packaging profile without a fork.
2. Which harnesses must work in WSL for a first release? (Cues and logs differ per harness.)
3. Is guidance-only install acceptable for WSL?
4. Should sessions on Windows paths (`/mnt/c`) be supported, or only Linux-filesystem projects?
5. Do the engineers use one distro or several? (Affects the default and the create dialog.)
