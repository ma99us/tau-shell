# 05 - Spike results (Phase 0 and read-only Phase 0b)

Run on one machine: Windows 10.0.26200, WSL 2.6.2.0, Ubuntu (WSL2) plus
docker-desktop. The throwaway test used TauShell's own `internal/conpty`
package (`Start` and `StartPreferredEnv`), so these results describe the real
hosting path. The spike test file was deleted afterward; no product code
changed.

## 1. Results

| # | Question | Result | Verdict |
|---|---|---|---|
| 1 | Does `wsl.exe --exec` run under TauShell's ConPTY host? | Yes. Output, input echo and exit code 5 all passed through | Pass |
| 2 | UTF-8 and box drawing end to end through ConPTY | `h\u00e9llo`, box-drawing and CJK characters round-tripped in both directions (output and typed input). This proves the byte path into the pty, not xterm.js rendering; rendering still needs a real-window check | Pass (bytes) |
| 3 | Initial size and resize | `stty size` printed `30 100`; after `Resize(120,40)` the program saw `40 120` and ConPTY repainted | Pass |
| 4 | Exit code propagation | Child exit 5 became process exit 5 | Pass |
| 5 | Forced termination: which Linux processes survive? | Plain background child died. **`setsid` and `nohup` children survived** (2 of 3) | Needs cleanup handshake |
| 6 | Marker-based cleanup | Launch with env `TAUSHELL_LAUNCH=<id>` forwarded via `WSLENV`; afterward scan `/proc/*/environ` inside the distro and kill matches: killed 2, none left | Pass; recommended |
| 7 | Launch latency | `wsl --exec true` about 220 ms (cold and warm); `--version` and `-l -v` about 100 ms | Acceptable; cache results |
| 8 | UNC read cost (`\\wsl.localhost\Ubuntu\...`) | List home 30 ms; walk the whole Copilot session-state tree 109 ms; **read last 64 KB of a 16 MB `events.jsonl`: 21 ms; read the whole file: 7060 ms** | Tail and offset reads only; never full reads |
| 9 | Missing distro | `wsl -d NoSuchDistro` exits -1 with `WSL_E_DISTRO_NOT_FOUND` (UTF-16 text) | Parse the error code, not prose |
| 10 | Non-login PATH | Plain `--exec sh -c` PATH includes `/mnt/c/...` entries (Windows PATH import) | Confirms the `/mnt/*` rejection rule |
| 11 | Native harnesses in the distro | Initially none. After a user-approved install (section 5) all three tested harnesses ran and exited cleanly | Pass, see section 5 |

## 2. Read-only install-flow facts (Phase 0b subset)

| Fact | Result | Effect on the plan |
|---|---|---|
| `wsl --help` documents `--install [Distro]`, `--no-distribution`, `--no-launch/-n`, `--web-download`, `--update [--pre-release]`, `--status`, `--set-default-version`, `--list --online/-o`, `--list --running`, `--list --quiet`, `--shell-type`, `--user`, `--system` | Confirmed | The flags in `04-wsl-as-a-tool.md` exist. Probe them from `--help` as capabilities |
| winget package `Microsoft.WSL` | Exists, latest 2.7.14; `Microsoft.WSL.PreRelease` 2.9.10 | Confirmed id. This machine has 2.6.2, so the "update available" path is real. Do not offer pre-release by default |
| winget installation notes | A restart is required if the Virtual Machine Platform feature was enabled | Pending-restart state is needed |
| `wsl --list --online` | Works; table output with NAME and FRIENDLY NAME columns (about 25 entries, `Ubuntu` first) | Use for a distro picker; parse columns, cache, handle no network |
| Windows optional-feature query | `Get-WindowsOptionalFeature` is unavailable without elevation here | Report "unknown" instead of failing; rely on `wsl --status` and a trivial `--exec` |
| Firmware virtualization flag (`Win32_Processor.VirtualizationFirmwareEnabled`) | Reported `False` although WSL2 runs, because a hypervisor is already active | Do not use it as a gate. Only use "WSL2 distro starts" as proof |
| `--exec` / `--cd` / `-d` / `--user` options | In help as expected | `--user` can pin the Linux user |

## 3. Not run, and why

| Item | Reason | Needed to close |
|---|---|---|
| Real harness inside WSL | Done for Claude Code, Copilot CLI and Codex (launch, render, exit, no survivors); not done: login, conversation, GSD, OpenCode | Sign in inside the distro; install the rest |
| `wsl --install`, `--update`, distro install, UAC, reboot, first-run user creation | Changes the machine and may elevate or reboot | Disposable VM with no WSL |
| Stopped distro, `wsl --shutdown`, systemd failure, sleep/resume | Would interrupt the user's running distros and sessions | A disposable distro, `--import` a tiny rootfs |
| xterm.js rendering of the above bytes | Needs a real TauShell window | Manual matrix in Phase 3 |
| Ctrl+C and interrupt keys through `wsl.exe` | Not run in this spike | Phase 3 live check |
| WSL1, older WSL, localized Windows | Not available | Fake-output fixtures plus a VM |

## 4. Decisions this changes

1. **Process cleanup (resolves the open handshake in `02-design.md` section 7):**
   per-launch marker environment variable (forwarded with `WSLENV`) plus a
   bounded `/proc/*/environ` scan inside the distro after forced termination.
   Wrap it in a `Runtime.Cleanup` call; ignore permission errors on other users'
   processes; never kill without a matching marker.
2. **State readers (resolves the UNC question):** acceptable only with
   offset/tail reads. The existing incremental readers (`usage_files` offsets,
   tail reads) must be the only path for WSL homes; a full read of a large
   transcript over UNC takes seconds. Add this as a hard rule in Phase 2.
3. **Readiness check:** use "distro starts and runs `--exec true`" as the proof,
   not firmware or optional-feature queries.
4. **Update path:** `Microsoft.WSL` is confirmed in winget, and an update is
   currently available here, so the Tools panel "update available" state can be
   tested against this real machine.

## 5. Live harness spike (existing Ubuntu distro, user-approved)

Installed without `sudo` into `~/.local/node` inside the distro: a user-level
Node `v26.13.0` (the distro's own `/usr/bin/node` is 18.19.1, too old) and, with
`NPM_CONFIG_PREFIX` set there, Claude Code 2.1.319, Copilot CLI 1.0.107 and
Codex CLI 0.167.0. Nothing else in the distro or on Windows was changed. They
can be removed with `rm -rf ~/.local/node`.

Each harness was launched through TauShell's ConPTY host with
`wsl.exe -d Ubuntu --cd /tmp --exec env PATH=<node bin>:/usr/bin:/bin <harness>`,
an env marker, 110x32 size, then Ctrl+C twice, then a marker scan.

| Harness | Result | Cleanup |
|---|---|---|
| Copilot CLI | Full TUI rendered (tabs, box drawing, banner); Ctrl+C x2 exited, code 1 | 0 survivors |
| Codex CLI | Login chooser rendered; Ctrl+C x2 exited, code 0 | 0 survivors |
| Claude Code | Welcome screen rendered, then a network error; Ctrl+C x2 exited, code 1 | 0 survivors |

Not logged in to any harness, so no real conversation, resume or log-reader check
was possible; credentials were not entered or read.

New findings:

1. **`npm` inside WSL can be the Windows npm.** `npm config get prefix` printed
   `C:\Users\...\AppData\Roaming\npm` because of PATH interop, so a plain
   `npm install -g` would install onto Windows. Installs must use the
   distro's own npm and an explicit prefix (as done here), with `/mnt/*`
   rejected like harness lookup.
2. **A harness's Node shebang picks the first `node` on PATH.** The distro's
   Node 18 would run the harness. Launch with an explicit PATH (via `env PATH=...`
   or the `PATH` forwarded env) that puts the harness's own Node first, and
   detect the Node version inside the distro against each harness's minimum.
3. **npm now blocks install scripts by default.** Claude Code's `postinstall`
   (native binary) did not run until `--allow-scripts=@anthropic-ai/claude-code`
   was passed; before that `claude --version` failed with "native binary not
   installed". Guided-install text must include the flag. This matches the
   `EALLOWSCRIPTS` note in `AGENTS.md`.
4. **Corporate TLS inspection breaks WSL harnesses.** Claude Code reported
   `SELF_SIGNED_CERT_IN_CHAIN` for `api.anthropic.com`: the distro does not trust
   the corporate root CA that Windows trusts. Detection should surface this
   and the guidance should cover `NODE_EXTRA_CA_CERTS` and installing the CA
   in the distro. This is a likely top support issue.
5. Install time was long (about 8 and 6 minutes for the npm installs, probably
   the `/mnt/c` interop and network); the install UI needs progress and a
   long timeout.

Still not verified: login and a real conversation per harness, `--resume`,
the Claude and Codex readers, stopping the distro, and WSL1.

### 5.1 Existing readers over a UNC root

An in-package throwaway test called the real Copilot readers
(`checkCopilotResumeIn`, `copilotModelIn`, `copilotTitleIn`) with the root
`\\wsl.localhost\Ubuntu\home\<user>\.copilot\session-state` against 8 real
sessions in the distro (events files from 4 KB to 16 MB). Every call returned in
4-20 ms with no errors, the resume check correctly reported "no recorded
conversation" for sessions without an events file, and a model was read from the
16 MB file. So the current readers need no code change except taking a
configurable root; their existing tail reads already meet the UNC rule above.
The test was deleted.

### 5.2 Live Copilot CLI session in WSL (user signed in)

Copilot CLI (installed in section 5) was driven through TauShell's ConPTY host
in a scratch directory `/tmp/tau-spike` with a real prompt ("reply with exactly
the word PONGTEST"). Results:

| Check | Result |
|---|---|
| Real conversation | Worked: model replied `PONGTEST`; UI showed the working spinner and `ctrl+c interrupt` footer while busy |
| Terminal protocol | Copilot emitted its OSC 7501 state marker (`state=done:app=copilot`) through `wsl.exe` and ConPTY intact, so screen-cue detection input is unchanged |
| Folder trust prompt | Appeared in the distro for the new Linux directory (and again on resume); TauShell's launch note/cue for it must work for Linux paths |
| Distro state files | `~/.copilot/session-state/<id>/` with `events.jsonl`, `workspace.yaml` (`cwd: /tmp/tau-spike`, a **Linux path**, plus an auto title) |
| Existing readers over UNC | `checkCopilotResumeIn` returned no error, `copilotModelIn` returned `claude-sonnet-5.5`, `copilotTitleIn` returned the title, in about 10 ms |
| Resume | `copilot --resume=<id>` in WSL restored the previous conversation on screen |
| Graceful exit | Ctrl+C twice exited with code 0, 0 survivors |
| Forced kill mid-turn | `Terminate` while the model was streaming: 0 survivors (Copilot spawned no detached children here; the marker scan remains the safety net) |

New findings:

1. **`workspace.yaml` `cwd` is a Linux path.** Session discovery that matches a
   harness session to a TauShell session by working directory
   (`discover...In(root, workingDir, ...)`) must compare in the runtime's
   native path space (`ToNativePath`), not the Windows path.
2. **Trust prompts** are per Linux folder, so every new WSL project triggers
   the first-run trust flow, as it does on Windows.
3. Copilot's login state in the distro is independent of Windows; the Tools panel
   should show per-runtime harness readiness, not only "installed".

Cleanup: the test session folder in the distro and `/tmp/tau-spike` were
removed afterward, and the test files were deleted.

Not verified (unchanged): Claude Code and Codex logged-in sessions, GSD and
OpenCode, stopping the distro, sleep/resume, the real install flows, and
rendering in a real TauShell window.

### 5.3 Live Claude Code session in WSL (user signed in, settings copied)

Same method as 5.2, with Claude Code 2.1.319 in `/tmp/tau-spike`.

| Check | Result |
|---|---|
| Real conversation | Worked: replied `PONGCLAUDE`; spinner and `esc to interrupt` visible while busy; footer showed `done` afterward |
| Onboarding | A fresh distro home shows first-run screens (theme picker, security notes, folder trust) even when signed in. The trust dialog highlights `No, exit` by default, so a plain Enter did not advance; Arrow Down then Enter did. TauShell must not auto-confirm these; the existing input cues and the terminal stay the user's tool |
| State files | `~/.claude/projects/-tmp-tau-spike/<id>.jsonl` (cwd encoded as dashes; about 150 KB after one turn) |
| Existing readers over UNC | `checkClaudeResumeIn` no error; `claudeModelIn` returned `claude-sonnet-5-5`; `claudeTitleIn` returned `PONGCLAUDE reply`; about 10 ms |
| Resume | `claude --resume <id>` restored the session and accepted a new prompt |
| Forced kill mid-turn | `Terminate` while streaming: 0 survivors |
| Ctrl+C twice, 1.5 s apart, while idle | Did not exit within 8 s (Claude's own double-press window); needed a forced stop. Graceful-stop timing for Claude should use the manifest's exit text (`/exit`), as on Windows |

Findings: the project folder name is the Linux cwd with `/` replaced by `-`, so
the Claude readers' path encoding must use the runtime's native path, not the
Windows path. Copying `~/.claude/settings.json` (see the earlier note) was
enough for sign-in to work. Test data in the distro (`projects/-tmp-tau-spike`,
`/tmp/tau-spike`) was removed; Claude also recorded the folder-trust decision
for `/tmp/tau-spike` in `~/.claude.json`, which was left in place.

## 6. Suggested remaining spike (needs a disposable distro)

`wsl --import tau-spike <dir> <rootfs.tar>` a minimal distro, then run: a native
Node-based harness, stop and shutdown while a session runs, Ctrl+C and
interrupt keys, and a systemd-less start. Remove it with `wsl --unregister
tau-spike`. This was not done autonomously because it creates and deletes a
distro on the user's machine.
