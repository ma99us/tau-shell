# 06 - Verification backlog (WSL gaps)

Items that could not be verified in the spikes (`05-spike-results.md`). Each row
is a task to close before a harness or flow is called supported in WSL. Status:
`open` until someone runs the check and records the result.

Not supported in WSL (rejected at launch or skipped): Codex and OpenCode (they discover their conversation ID from Windows folders), and the CLI `start` command. GSD launches but its readers (model, activity, resume) skip WSL homes.

Verified so far in WSL (for reference): Copilot CLI and Claude Code, both
signed in: launch, real conversation, resume, readers over UNC, graceful and
forced stop with no survivors. Codex launched to its login screen only.

## 1. Harness gaps

Common checks for every harness unless noted: install in the distro (user
prefix, no `sudo`), launch through `wsl.exe --exec` with an explicit PATH, first
run and trust prompts, a real prompt and reply, `--resume`, state files and
their `cwd` or project-path encoding (Linux path), readers over UNC with tail
reads, attention cues (working, idle, input), model and title detection, usage
parsing, forced kill mid-turn with marker cleanup, and the Tools panel version
and capability probe inside the distro.

| Harness | Status in WSL | Blocker | What is left |
|---|---|---|---|
| OpenAI Codex CLI | Installed (0.167.0); launches to the sign-in chooser, exits on Ctrl+C, 0 survivors | No login available | Sign in (device code or API key); real conversation; `--resume`; session files under `~/.codex/sessions` and their `cwd`; `existingCodexSessionIDsIn` and `discoverCodexSessionIn` with a Linux working dir; usage parsing; cue check against the real TUI |
| GSD | Not installed in the distro | No login/credentials; not installed | Install (user prefix); first-run; real conversation; resume; GSD session discovery in `gsd_codex.go` against `~/.gsd` (a `.gsd` folder already exists in the distro home, origin unknown); cues; usage; settings and skill paths |
| OpenCode | Not installed in the distro | No login; not installed | Install; SQLite database location in the distro (`~/.local/share/opencode/opencode.db`); opening a SQLite file over UNC (locking, WAL files, read-only open, speed) versus copying a snapshot; `existingOpenCodeSessionIDsIn`, `discoverOpenCodeIn`, `openCodeMessagesIn` with Linux paths; cues; usage |
| Claude Code | Verified | - | Plugins and MCP servers are not copied from Windows (only `settings.json`); corporate CA behavior (`NODE_USE_SYSTEM_CA`, `NODE_EXTRA_CA_CERTS`); status-line or hook commands that call Windows executables |
| Copilot CLI | Verified | - | Per-folder trust on every new Linux path; `/mnt/c` working directories; skills injection path |
| Future harnesses | - | - | Re-run this checklist when adding one |

Harness-specific risks to check first:
- **Codex:** `cwd`-based session discovery compares paths; the stored `cwd` is
  a Linux path (as for Copilot and Claude).
- **OpenCode:** SQLite over a `\\wsl.localhost` path may fail or be slow and can
  corrupt nothing only if opened read-only; decide between read-only UNC open
  and running a reader inside the distro.
- **GSD:** its home folder layout and skill directories were only seen on the
  Windows side; confirm they match inside the distro.

## 2. Environment and platform gaps

| Gap | Status | What is left |
|---|---|---|
| Real `wsl --install`, `--update`, distro install, UAC prompt, reboot, pending-restart resume | open | Disposable VM without WSL (Phase 0b) |
| Stopped distro, `wsl --shutdown` during a session, systemd start failure, Windows sleep and resume | open | A disposable distro (`wsl --import` / `--unregister`) so the user's distros are not interrupted |
| WSL1, old WSL builds, localized `wsl.exe` output, corporate-locked WSL | open | VM matrix and fake-output fixtures |
| Multiple distros and multiple Linux users | open | Second disposable distro |
| Rendering in a real TauShell window (Unicode, wide characters, bracketed paste, resize, clipboard text and image) | open | Manual matrix in Phase 3 |
| Ctrl+C and interrupt keys from the manifest for each harness | open (Copilot and Claude partly checked) | Per-harness live check |
| Corporate TLS inspection in the distro (Claude reported `SELF_SIGNED_CERT_IN_CHAIN`) | open | Document CA install and `NODE_EXTRA_CA_CERTS`; test with the corporate CA |
| Git inside the distro: Git tab, fetch, sync, `safe.directory`, credentials | partly verified | Branch, status, changes and a full sync (commit, rebase, push to a local bare remote) ran inside Ubuntu through `wsl.exe --exec git`. Not checked: credential helpers and SSH prompts, `/mnt/c` repositories, large repositories |
| Files drawer over UNC | partly verified | Listing and reading a Linux folder worked and a symlink pointing outside the root was refused. Not checked: very large folders, Explorer reveal, open-with. Copied full paths are Linux paths |
| Pasted-image and file-picker path conversion | closed (no surface) | Audited in Phase 5: no file-drop or image-paste feature exists; drawer paths are Linux paths. Pasting a Windows path as text is not converted. Harness-native image paste from the Windows clipboard inside WSL not tested |
| First-run setup without PowerShell 7 for WSL-only users | implemented, unverified | Setup is skipped when WSL has a distro; needs a real run on a machine without pwsh |
| WSL and distro install or update flows | implemented, unverified | Not run for real because they change the machine; needs a disposable VM. Update-available detection compares with the latest microsoft/WSL release (unit tested with a fake fetcher; not run against a real update) |
| Skills injection into a WSL session | implemented, unverified | The Windows skill folder is passed as `/mnt/c/...`; the option probe runs inside the distro. Needs a live run with a skill library, and a distro with automount disabled |
| Resume check, model, title, usage and activity readers for WSL sessions | partly verified | Claude resume check against the distro home was seen refusing a conversation-less session. Readers run only while the distro is running (they never start it); final usage of a stopped distro is ingested on the next launch. Model, title and usage on a real conversation not re-checked end to end |
| Phone server with a WSL session | open | Confirm guards and queue behavior are unchanged |

## 3. How to close an item

1. Run the check on a disposable or approved distro; never print credentials.
2. Record the result and date in `05-spike-results.md`.
3. Change the row status here (or remove it) and, if behavior differs from the
   design, update `02-design.md` and `03-implementation-plan.md`.
4. Clean up transcripts, temporary directories and test sessions afterward.
