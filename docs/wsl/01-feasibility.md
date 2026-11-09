# 01 - Feasibility

Status: historical investigation (before the build). Current status: [README](README.md) and [06-backlog](06-backlog.md).

## 1. Probe results

Read-only probes on a development machine. Nothing was installed or changed.

Environment: Windows 10.0.26200.9711; WSL 2.6.2.0, kernel 6.6.87.2-microsoft-standard-WSL2,
WSLg 1.0.71; distros `Ubuntu` (default, WSL2) and `docker-desktop`.

| Probe | Result | Consequence |
|---|---|---|
| `wsl.exe --status`, `-l -v`, `--version` | Works; output is UTF-16LE | Decode UTF-16; do not parse localized text. Prefer `wsl -l -q` for names |
| Environment forwarding | A variable listed in `WSLENV` reaches the Linux process | Session environment can be forwarded without putting values in argv: export into the `wsl.exe` child environment and name it in `WSLENV` |
| `wsl.exe --cd C:\path -- pwd` | Prints `/mnt/c/...` | A Windows working directory is auto-translated for drive paths. A Linux-native directory needs `--cd /home/...` |
| `wslpath -u` via `wsl.exe -- wslpath ...` | Backslashes are eaten (`C:AlturaProjects...`); forward slashes work (`C:/Altura/...` -> `/mnt/c/Altura/...`) | `wsl.exe` re-parses arguments through a Linux shell. Never pass backslash paths or shell text; use `--exec` and forward slashes |
| `wsl.exe --exec printf '%s\|' 'a b' '$HOME'` | `a b\|$HOME\|` (no expansion, spaces kept) | `--exec` gives true argv passing: the only safe way to launch a harness with discrete argv values, matching the "user harness arguments are discrete argv values" rule |
| Piping a script into `wsl.exe -- bash -l` | Failed on this machine: "Failed to start the systemd user session" | Do not depend on a login shell. Resolve PATH explicitly. Surface the distro error text |
| Exit code | `bash -c 'exit 7'` -> `wsl.exe` exits 7 | Harness exit codes propagate, like `pwsh -Command` today |
| UNC `\\wsl.localhost\Ubuntu\home` | Listing works while the distro runs | Windows-side readers can read distro files. Slower for many small files, and the distro must be running |
| `git -C \\wsl.localhost\...` | Non-repo failed as expected; Git ownership checks (`safe.directory`) apply to UNC repos | Prefer running git inside the distro for Linux paths |
| Kill behavior | Terminating `wsl.exe` also ended its `sleep` child | ConPTY teardown plus the job object likely ends the foreground tree; verify with real harnesses that spawn daemons |
| Harness presence in the distro | `claude`, `copilot`, `codex`, `gsd`, `opencode` resolved via `command -v`, but to `/mnt/c/Users/.../npm/*` and a VS Code `copilot` shim (which prints that the VS Code Server is not installed), not native Linux binaries. `node` was native (`/usr/bin/node`) | WSL imports the Windows PATH (`appendWindowsPath`). Detection must reject `/mnt/*`, otherwise Windows shims look like native installs |
| Data folders in the distro home | `.copilot/session-state` and `.gsd` present; `.claude`, `.codex`, `.local/share/opencode` absent | Harness state lives in the Linux home, not `%USERPROFILE%`; every reader needs a base-directory abstraction |

Not probed (see README): live harness sessions, ConPTY hosting of `wsl.exe` by
TauShell itself, resize and Unicode behavior, WSL1, older WSL builds.

## 2. Coupling inventory

Size: S small, M medium, L large. Paths are relative to the repo.

### 2.1 Launch and process lifetime

| Area | Where | Today | For WSL | Size |
|---|---|---|---|---|
| Shell launch | `internal/sessionhost/manager_windows.go` (~517-579), `cmd/taushell/main_windows.go` (~263-334) | `pwsh.exe -NoLogo -NoProfile -Command <script>` | `wsl.exe -d <distro> --cd <dir> --exec <harness> <argv...>`; no shell script | M |
| Command building | `internal/harness/adapter.go` `PowerShellInvocation`, `QuotePowerShell` | PowerShell quoting, UTF-8 prologue | Not needed with `--exec`; kept for the PowerShell runtime | S |
| Process ownership | `internal/conpty/process_windows.go` | Job object kills the `pwsh.exe` tree | Job object owns `wsl.exe`; the child died with it in the probe; add a distro-side process-group kill as defense | M |
| Graceful stop | manifest exit and interrupt keys (`copilot-cli.yaml`) | Keys written to the pty | Unchanged | S |
| Exit reporting | `manager_windows.go` (~949-1115) | "PowerShell exited with code N" | Runtime-neutral wording; distro start failure text | S |
| UTF-8 | `config.go` `PowerShell.UTF8`, `UTF8Enabled()` | UTF-8 prologue | Linux is UTF-8; `wsl.exe` output through ConPTY must still be proven end to end | S |

### 2.2 Harness state readers (Windows home assumption)

All use `os.UserHomeDir()` plus a harness folder:

| Reader | Where |
|---|---|
| Copilot events, model, title, resume check | `internal/harness/adapter.go` (~115-144), `activity.go`, `model.go`, `titles.go` |
| Claude transcripts, titles, model | `adapter.go`, `model.go`, `titles.go`, `usage/parse.go` (~160) |
| GSD / Codex sessions | `harness/gsd_codex.go` |
| OpenCode database | `harness` OpenCode readers, `usage/parse_harnesses.go` |
| Claude LiteLLM quota config | `internal/quota/litellm.go` |
| Usage ingestion roots | `internal/usage/parse.go` |

Fix: a `HomeDir` from the session's runtime: the Windows profile for
PowerShell, `\\wsl.localhost\<distro>\home\<user>` for WSL. The readers already
take a root in tests (for example `copilotModelIn(root, ...)`), so most changes
are small (S each, M together). Risks: UNC reads are slower; tail reads of big
`events.jsonl` need care; the distro may be stopped (do not start it just to read).

### 2.3 Detection, install and capability probes

| Area | Where | For WSL | Size |
|---|---|---|---|
| Harness lookup | `harness/adapter.go` `FindExecutable` (`exec.LookPath`), `cmd/taushell/harnesses_windows.go` | `command -v` inside the distro; reject `/mnt/*` | M |
| Versions and capability probes (`--help`, skill dir option) | `harness`, `tools` | Run inside the distro | M |
| Tool installs and updates | `cmd/taushell/tool_updates_windows.go`, `tools_windows.go`, `internal/tools` (winget, managed Node/npm, PortableGit) | Guidance only (npm/apt/vendor docs) with a copyable command; no silent install; no elevation | M |
| Prerequisite model | `tools.PowerShell`, first-run setup, `main_windows.go` | PowerShell required only for PowerShell sessions. New prerequisite: WSL available plus a chosen distro | M |

### 2.4 Git, files, paths

| Area | Where | For WSL | Size |
|---|---|---|---|
| Branch, status, changes, fetch, sync | `internal/presets/presets.go` (`git -C dir ...`) | Run git inside the distro (same parsers); credentials and line endings come from the distro | M |
| Working directory | session YAML `working_dir`; `projectfiles` | Store distro plus Linux path, or a `\\wsl.localhost` path; reveal and the files drawer need UNC conversion | M |
| Files drawer reads | `internal/projectfiles` (EvalSymlinks, root containment) | Works on UNC; re-check symlink containment; stays read-only | S-M |
| Inserted paths, pasted images | `terminalPane.tsx` insert, clipboard image save | Convert to Linux paths before insertion | M |

### 2.5 Environment, secrets, skills

| Area | Where | For WSL | Size |
|---|---|---|---|
| Session environment | `config.LaunchEnvironment`, vault | Export into the `wsl.exe` environment plus `WSLENV` names; never argv; never log values | M |
| Skills injection | `sessionhost.skillArgs`, `internal/skills` | Snapshots live on Windows; pass a Linux-visible path (`/mnt/c/...`) or copy into the distro; capability gate stays | M |
| Claude settings overrides | adapter code reading `~/.claude` | Same home abstraction as 2.2 | S |

### 2.6 UI, config, docs

| Area | Notes | Size |
|---|---|---|
| Session YAML schema | New optional `runtime: powershell or wsl` and `wsl.distro`; `powershell:` block only for PowerShell sessions; update `docs/v2/18-yaml-schema.md` | S |
| Create-session dialog | Runtime and distro picker; distro list from `wsl -l -q` | S-M |
| Home and Setup | WSL readiness; PowerShell no longer a hard Home gate when only WSL is used | M |
| Strings | "PowerShell exited", CLI banner, setup copy become runtime-neutral | S |
| Docs and rules | `AGENTS.md`, `docs/v2` (architecture, lifecycle, security, yaml, env, bootstrap) | M |

### 2.7 Not affected

ConPTY plumbing and xterm.js rendering (`wsl.exe` is another ConPTY child),
YAML plus SQLite storage, screen-cue attention detection, prompt queue and Send,
Git tab UI, workspaces, theme, toasts, phone server and pairing, update mechanism.

## 3. Product shape comparison

| | A. Per-session runtime in the same app | B. Separate WSL-only build |
|---|---|---|
| Code | One `Runtime` abstraction shared by all subsystems | Same abstraction needed, plus build tags or a fork to remove PowerShell |
| Release | One installer, one update feed, one smoke test | Two artifacts, two feeds, two smoke tests, version drift |
| User impact | Engineers who dislike PowerShell never need it; mixed teams work | Clean "no PowerShell anywhere" story |
| Risk | Setup and Home must stop hard-requiring PowerShell | Divergence over time; double the CI matrix |
| Rule change | Additive: PowerShell stays first-class, WSL a second runtime | Contradicts "Keep PowerShell first-class" for that flavor |
| Reversibility | Can ship off by default | Hard to merge back |
| Effort | Baseline | Baseline plus build, CI, packaging and update work |

Rejected option C: launch `wsl.exe` from inside `pwsh.exe`. It keeps the
PowerShell dependency the engineers want to remove and adds a second quoting layer.

**Recommendation: A.** If a "no PowerShell" distribution matters, add a
packaging profile later (same binary, different setup page and default
runtime), not a fork.

## 4. Difficulty summary

| Dimension | Rating | Reason |
|---|---|---|
| Terminal and ConPTY | Low | `wsl.exe` hosts through ConPTY like `pwsh.exe`; needs a live spike for resize, Unicode and clipboard |
| Launch | Low | One code path; `--exec` gives safe argv |
| State readers | Medium | Many small readers; a base-directory abstraction; UNC performance |
| Detection and installs | Medium | PATH-interop pitfall; guidance-only installs; distro states (stopped, missing, systemd failures) |
| Git | Medium | Run in the distro; keep parsers |
| Process lifetime | Medium | Needs proof with real harnesses that spawn subprocesses |
| Environment and secrets | Medium | `WSLENV` works; the no-argv, no-log rule must hold |
| UI, schema, docs | Low-medium | Small surface but touches setup gating |
| Overall | Medium-large | Mostly breadth, not depth; no fundamental blockers found |
