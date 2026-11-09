# 04 - WSL as a managed Tool

Status: built as a separate readiness model with a Tools panel row, confirmed install/update and update detection (see [03-implementation-plan.md](03-implementation-plan.md) Phase 4). Unverified flows are in [06-backlog.md](06-backlog.md).

Addendum to the design. WSL is modeled as one of the Tools (like PowerShell 7,
Node.js and Git), so it appears in the Tools panel with detection, version
tracking, installation help and a summary of the current installation.

Not verified: the install flows below were not run (they change the machine).
Command behavior is from Microsoft's documented `wsl.exe` interface and the
read-only probes in `01-feasibility.md`; confirm each flow in Phase 0b.

## 1. Tool model

Add `tools.WSL` (`ID = "wsl"`) as a host tool beside `PowerShell`:

| Field | Value |
|---|---|
| Name | WSL 2 |
| Executable | `wsl.exe` |
| Required | false (only needed for sessions whose runtime is `wsl`) |
| VersionArgs | `--version` (UTF-16LE output; parse the "WSL version" line, not localized labels) |
| Minimum / tested range | decided in the spike; probed machine: WSL 2.6.2.0, kernel 6.6.87.2 |
| Capabilities | `--exec`, `--cd`, `--distribution`, `--list` probed from `wsl.exe --help` |
| Install | `WingetID: Microsoft.WSL` (verify), vendor URL `https://learn.microsoft.com/windows/wsl/install` |

Because `wsl.exe` can exist as an inbox stub without the WSL package or
features, "installed" needs more than a path: `wsl.exe --status` or `--version`
must succeed. Report the failure reason (stub only, feature off, virtualization
off) instead of a generic "missing".

`Detect` needs a UTF-16 aware probe reader and a localized-output-safe parser.
Per-adapter version ranges, warnings outside the tested range, and "never
silently pin, downgrade or auto-update" apply unchanged.

## 2. Tools panel: what it shows for WSL

A WSL card with an expandable "Current installation" section:

| Item | Source |
|---|---|
| WSL version, kernel version, WSLg version | `wsl.exe --version` |
| Default WSL version and default distro | `wsl.exe --status` |
| Installed distros: name, state (running/stopped), WSL version (1/2), default flag | `wsl.exe -l -v` (UTF-16LE) or `-l -q` |
| Selected distro for TauShell | stored setting; defaults to the default distro |
| Per distro: user, distro OS release, native harness and Node found (reject `/mnt/*`), PATH-interop shims detected | `--exec` probes; only on a running distro, never start one just to inspect |
| Features/virtualization state | see 3.1; read-only checks |
| Update available | Compare against `wsl --update` availability or the published WSL release; show, never auto-apply |
| Warnings | WSL1 distro, WSL older than the tested range, `/mnt` shim shadowing, systemd failure text, stopped distro |

Per-tool rows inside the distro (Claude Code, Copilot CLI, Node, Git...) reuse the
existing harness rows, shown per runtime ("Windows" vs "WSL: Ubuntu"), so the
panel answers "is this harness usable in the WSL runtime".

## 3. Installation assistance

Principles (from `AGENTS.md`): offer confirmed installs, never install or
elevate silently, verify, then re-detect the requested version before reporting
success. Prefer winget, fall back to vendor links.

### 3.1 Diagnose first (read-only)

A "Check WSL readiness" action reports, with a next step for each failure:

1. `wsl.exe` present and recognized (stub vs real).
2. Windows build supports WSL2 (build number check).
3. Optional features Virtual Machine Platform and Windows Subsystem for Linux
   (read-only query; elevation may be required to read; if so say "unknown").
4. Virtualization enabled in firmware (cannot fix; explain and link).
5. At least one WSL2 distro registered.
6. Chosen distro starts and runs a trivial `--exec` command (catches the
   systemd failure seen in probing).

### 3.2 Install steps (each separately confirmed)

| Step | Action | Needs elevation | Reboot |
|---|---|---|---|
| Install or update WSL | `winget install Microsoft.WSL` (verify), or `wsl.exe --install --no-distribution`, or `wsl.exe --update` | Usually yes | Sometimes |
| Install a distribution | `wsl.exe --install -d <name> --no-launch` using a name from `wsl.exe --list --online` | Often yes first time | Sometimes |
| First-run user creation | Cannot be automated safely: opens the distro for the user to choose a username and password. Offer "Open distro" and wait | No | No |
| Set WSL2 as default | `wsl.exe --set-default-version 2` | No | No |
| Install harnesses in the distro | Guidance only: show the vendor command and a copy button (no `sudo`, no silent install). Node via the distro's package manager or nvm is the user's choice | No | No |

Elevation: show a plain-language confirmation (what, why, that Windows will
show a UAC prompt), then start the command through a UAC prompt the user
approves. If the user declines or policy blocks it, fall back to a copyable
command plus the vendor link. Never reuse an elevated process. Never run `sudo`
in the distro.

Reboot: if the install reports that a restart is required, show "Restart
Windows, then reopen TauShell" and never reboot automatically. Persist
"WSL install pending restart" so setup resumes the check on the next launch.

Progress and results are shown as toasts (app scope) and in the card; command
output goes to a bounded log with no secrets. Partial failure (for example the
feature installed but no distro) leaves the card in a clear "next step" state.

### 3.3 Version tracking

Detection runs at startup and on a manual refresh, like the other tools. The
card shows installed vs. latest known WSL version and "update available" with a
confirmed `wsl.exe --update`. Distro OS updates (`apt upgrade`) are out of
scope and never run.

## 4. Impact on the plan

- **Phase 0b (new, spike):** run the install and readiness commands on a
  disposable VM or a machine without WSL; record elevation, reboot, UAC and
  output behavior; confirm the `Microsoft.WSL` winget id and the
  `--install --no-distribution` flag; capture localized output differences.
- **Phase 3.5 (new, M):** `tools.WSL` spec, UTF-16 probe reader, readiness
  checks, distro inventory service (`wsl.exe -l -v`) with a Wails bridge method.
- **Phase 4 (extended):** Tools panel WSL card, readiness list, confirmed
  install and update flows, pending-restart state, per-runtime harness rows.
- **Phase 6:** `docs/v2` Tools/setup docs and `AGENTS.md` gain the WSL tool
  rules (never auto-install, confirmed elevation, no automatic reboot, no
  starting a distro for inspection).

## 5. Risks

| Risk | Mitigation |
|---|---|
| Elevation and reboot cannot be simulated in CI | Fake `wsl.exe` tests for parsing and state machine; manual VM checklist |
| Localized `wsl.exe` text | Parse structured pieces (versions, list columns); never match prose |
| Corporate policy blocks WSL or winget | Detect and show the policy-style failure; vendor links |
| Installing a distro needs interactive user creation | Hand off to the user; wait and re-detect |
| Distro start failures (systemd) | Surface the raw error; do not retry silently |
