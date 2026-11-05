# Security Model

## Principles

- Least privilege and local-first operation.
- Do not store secret values in session YAML or expose them to the UI renderer.
- Keep session environments and managed process trees isolated.
- Validate every desktop UI bridge request; expose only the operations the
  trusted local UI needs.
- Do not silently install software, elevate privileges, or execute downloaded
  scripts.

## Session environment values

Session `environment:` values are passed to the session's PowerShell process
through the child environment block (`conpty.ChildEnvironment` merged over the
host environment), not through the `pwsh -Command` script, so they do not
appear on any process command line. Tests confirm a real child receives the
value and that its `Win32_Process.CommandLine` does not contain it. Values are
still stored as plaintext in the session YAML and are visible to the session's
own process tree (and to same-user tools that read process environments), so
put real secrets in the vault instead.

## Secret vault

`secrets:` entries map an environment variable to a vault secret name
(`docs/v2/10-environment-management.md`). Values are stored as generic
credentials in the current user's Windows Credential Manager
(`internal/vault`), so Windows protects them with the user's login. They are
read only at launch, by `Config.LaunchEnvironment`, and go only into the child
environment block; they are not in YAML, `state.db`, diagnostics (only the
secret *name* is logged on failure), toasts, the command line or any bridge
payload: `ListSecrets` returns names and the sessions using them, and
`SetSecret` is write-only. A missing secret stops the launch and names the
secret. Honest limit: any process running as the same user can read the
credential store and the injected child environment, and the harness's own
child processes inherit the variable; the vault removes plaintext from YAML,
not trust in same-user code.

## Tool installation

TauShell may offer installation of known prerequisites and supported harnesses.
An install action must identify the package and source and require user
confirmation before invoking winget, npm, or a managed archive installer. If
the required package manager is unavailable, direct the user to official vendor
installation instructions. Re-detect the executable, version, and required
companions after installation; do not treat the installer's exit code alone as
proof of success.

Optional Node.js/npm and Git for Windows use per-user managed installs under
`%LOCALAPPDATA%\TauShell\tools`; they are not host prerequisites. Node's
version comes from the official Node release index, but installation verifies
the Node release-key signature on `SHASUMS256.txt` and the selected ZIP hash
before extraction. The trusted signer fingerprints are pinned to the current
Node.js release-key list and must be reviewed when updated. Git's full
PortableGit archive is selected from the official Git for Windows GitHub
release and checked against the release asset's SHA-256 digest. Downloads are
HTTPS-only with bounded response sizes and redirects restricted to the known
vendor/CDN hosts. Extractors reject absolute/traversal paths, cap extracted
size, create files only within the managed staging/version directory, and
activate a version only after runtime verification. Partial installs are
removed. Node/npm and Git paths are prepended to TauShell's process environment
only; no user or machine PATH registry value is written. Explicit session
`environment.PATH` remains a complete override.

Installing an npm harness executes its package installation scripts as the
current user. TauShell asks separately before installing a missing Node runtime
and before installing the harness. These installs do not request elevation.
Package lifecycle scripts are still third-party code and must be treated as
such; confirmation displays the package and source. Do not install packages
silently or claim that a Node ZIP or a PortableGit archive has been tested
beyond the exact versions exercised on Windows.

## Bundled ConPTY

TauShell loads `conpty.dll` only by absolute path from its own executable
directory, and only when `OpenConsole.exe` is also present there. Install
TauShell to a directory writable only by the user or an administrator so that
another process cannot plant those files. `scripts/fetch-conpty.ps1` pins the
`Microsoft.Windows.Console.ConPTY` version and verifies the package SHA-256
and Microsoft Authenticode signatures before copying. A present bundle that
fails to load is logged as a warning, and the session falls back to inbox
ConPTY.

## Harness arguments

Session YAML may contain explicit command-line arguments for a harness. Treat
them as user-authored configuration and pass them as discrete argv values, not
through shell interpolation. TauShell-managed resume IDs and required adapter
options remain protected from override. Do not put credentials or secrets in
arguments.

Generated session templates may include harness-specific permission-related
flags. They must be editable in YAML; the saved YAML is the user's explicit
choice and TauShell does not add a separate launch confirmation for those
configured flags.

## Desktop UI bridge

Wails exposes a deliberately small set of typed methods for session listing,
creation, lifecycle, terminal input/output, resize, and directory selection.
Session IDs and arguments are validated by the session layer, and the
renderer is not given arbitrary process launch, filesystem path access, or
environment values. The Windows WebView2 host cancels top-level navigations
outside `http://wails.localhost` before they load, and Wails independently
validates the origin of calls into bound methods. Keep both controls enabled:
origin validation protects the bridge, while the navigation guard keeps
untrusted pages out of the desktop surface.

WebView local storage contains presentation metadata (open tab IDs and
selection) and up to five recently created session working-directory paths.
It must not contain credentials, environment values, or other secret data.

## Attention alerts

Taskbar flash and Windows toasts are raised by Go, only when TauShell is not
the foreground window, and carry just "TauShell" and "<session name> is
waiting for your answer" / "<session name> finished": no paths, terminal
content, prompts or environment values, in toasts or logs. The UI sends only a
session id and an "input"/"done" kind. Toast support uses the vendored Wails
notification API, which registers a per-user (HKCU, no elevation) AppUserModelId
and COM class entry for `taushell.exe`. Clicking a toast reopens TauShell on
that session; a cold start from a toast arrives as a COM `-Embedding` argument
and opens the desktop. Windows Focus Assist or per-app settings can suppress
toasts; TauShell does not claim a toast was seen. If toast setup fails it is
logged and only the taskbar flash remains.

## App updates

Home offers a newer GitHub release (pre-releases included) only after a click on
**Update and restart**; nothing is downloaded or installed silently.

- Source: the project's GitHub API and asset URLs under
  `https://github.com/ma99us/tau-shell/releases/download/`; anything else is refused.
  Size caps: 150 MB ZIP, 64 KB checksum file.
- Trust: the ZIP's SHA-256 must match the release's `SHA256SUMS.txt`. This proves
  integrity of the download, not publisher identity (both files come from the same
  release); signature verification is future hardening. Extraction refuses paths
  outside the target and requires `taushell.exe`.
- Swap: files are staged in `<install>\_update` (same volume). The old files are
  renamed into `_rollback\<old-version>` (Windows allows renaming a running exe),
  the new ones moved in, and any error undoes the move. One previous version is kept;
  Home offers **Roll back**. The app never elevates: a read-only folder, a dev build
  or another instance from the same folder disables the button and the Releases page
  link remains.
- Sessions: the normal close confirmation stops running sessions first. The old
  process writes `~/.tau_shell/update-result.json` (versions, running session IDs)
  and starts the new exe; the new one reads and deletes it, restores those sessions
  that have a resume ID, and reports the rest.
- Data safety: before swapping files, the state database (`state.db` plus WAL files) is
  copied to `~/.tau_shell/backups/state-<old-version>`; if the backup fails, the update
  stops. The database records a schema version (`PRAGMA user_version`,
  `session.SchemaVersion`). A build refuses to open a database with a newer schema
  and points at the backup folder, so a rollback cannot silently corrupt it. Bump
  `SchemaVersion` for any change an older build cannot read. Builds before this guard
  (1.0.1 and earlier) do not enforce it.
- Not yet verified: the exe's embedded version is not compared with the tag.

## Tool update checks

At startup and then daily, TauShell runs `winget list --id <package> --exact
--upgrade-available` (no console window, read-only) for installed tools with a
declared winget package. A tile shows "Update to <version>" only when winget
reports one; tools not installed through winget get no button. Updating runs
`winget upgrade` in a visible console only after a native confirmation; it is
never automatic. Failed checks are logged and show nothing.

## Price updates

Claude prices are the only data TauShell fetches on its own: an HTTPS GET to
`https://platform.claude.com/docs/en/about-claude/pricing.md`, falling back to
LiteLLM's public price JSON on GitHub. Requests are fixed-URL, GET-only, have a
15 s timeout and a size cap, and send no session, path or usage data. Parsed
values are sanity-checked, and failures keep the last good cache and log one
diagnostics line. Set `usage.price_updates: false` to disable. There is no
Copilot price fetch; its cost comes from the harness log.

## Skill library

Skills are prompts that agents may follow with their own tool permissions, so
treat them as code. TauShell fetches them only from `skills.repo` (https
`github.com/<owner>/<repo>` only): a GET to `api.github.com` for the commit
sha and `codeload.github.com` for the archive, no credentials, 15 s timeout, 20
MB cap. Extraction accepts only `tau-shell.yaml` and `skills/`, rejects links,
special files, `..`, backslashes, colons and files over 1 MB, and writes to a
temp folder before renaming. Downloads are applied only when the user clicks
**Update skills** unless `skills.auto_update` is true. Set `skills.repo: ""`
to disable fetching, or `skills.enabled: false` to disable the library. The
snapshot is a moving branch, not a signed release; pin `skills.ref` to a tag or
commit to freeze it.

## Copilot quota

The Home quota chip makes one outbound call, `gh api /copilot_internal/user`,
through the user's own GitHub CLI sign-in. TauShell never reads or stores the
token and never logs response bodies; failures are logged once without detail
and the chip is hidden. It is on by default and disabled with
`quota: { copilot: false }` in `~/.tau_shell/config.yaml`.

## Control channel

Off by default (`control.enabled` in `~/.tau_shell/config.yaml`, read at
startup; the Home switch opens or closes the pipe at once and rewrites that one
line in place, keeping comments). When on, each TauShell process serves one named pipe
`\\.\pipe\taushell-ctl-<user-tag>-<pid>` created with a protected DACL that
grants only the current user, `FILE_FLAG_FIRST_PIPE_INSTANCE` (startup fails
loudly if the name is already taken) and `PIPE_REJECT_REMOTE_CLIENTS`. Clients
connect with anonymous impersonation and check that the server PID matches the
pipe name. Programs running as the same user are trusted by design; the caller
PID and image name are audit hints, not authorization. Messages are bounded
single JSON lines. `hello`, `list` and `wait` return no prompt text or terminal
content (`list` shows each session's state and whether it is confirmed). `send`
carries text one way into the window, which types it through the terminal only
if the live screen still shows a safe state; Go refuses first unless the state
is a fresh harness cue (never while a turn is working, never into a permission
prompt, never over unsent text). Prefer `--stdin` for sensitive text: command
lines are visible to other local processes. Every request is added to an
in-memory audit trail (time, caller PID and image, operation, session id,
outcome) with no text or content; operation names only reach the diagnostic
log. `read`, `stop` and agent access (`control.agents`) are not
implemented.

## Project file drawer

Read-only. `internal/projectfiles` resolves every requested path with
`EvalSymlinks` and requires it to stay inside the session's working directory:
`..`, absolute paths, volume names and links leaving the folder are refused.
Listings are capped (2000 entries) and reads are bounded (512 KB, UTF-8 text
only; other binary files are refused). Files with a png, jpg, gif, webp, bmp,
ico or svg extension (up to 8 MB) are returned as a `data:` URL and shown in an
`<img>`, never inlined, so SVG scripts do not run. File contents are never
logged. Markdown is parsed into plain UI nodes: no HTML is passed through,
links are shown as text and never followed, and images referenced from
Markdown are not loaded.

## Phone server (experimental, off by default)

`mobile.enabled` in `~/.tau_shell/config.yaml` (the Home Phone switch opens or
closes it at once and rewrites that line in place). It listens on exactly one
address, `mobile.interface`, which must be an address of this PC; the wildcard
address and an empty value are refused, so it never listens on all interfaces.
TLS 1.3 only, with no system trust store: each pairing has its own self-signed
key, the phone pins its SHA-256, and the certificate is chosen by the TLS server
name (`pair.taushell` for the one-time pairing channel, then
`d-<device id>.taushell`; the dotted form is needed because Android drops dotless
names from the handshake). A paired
phone also sends a 256-bit bearer token; only its SHA-256 is stored. Private
keys are stored sealed with the Windows user's DPAPI key in
`~/.tau_shell/mobile/devices.json`. Pairing needs the one-time code from the QR
code (single use, 5 minutes, 5 wrong codes close the window) and the user's
approval of the phone's name on the PC. Failed authentication from one address
backs off after 10 failures a minute. Requests use the same guarded `list`,
`wait`, `send` and `start` logic as the control channel, so the same refusals
apply. The in-memory audit records device id, operation, session id and
outcome, never text. A paired phone may read a bounded, read-only tail of a
running session's visible terminal text (`GET /v1/sessions/{id}/screen`, at most 120
rows and 32 KB, no scrollback). The window answers each request on demand; Go does not
store the text, and neither diagnostics nor the audit record it. The user can turn this
off with `mobile.screen: false`; the phone then shows that it is off. Terminal text can
contain secrets the harness printed, so only pair phones you trust. A phone may also
answer a Claude Code trust or permission prompt with a fixed yes (Enter) or no (Esc)
(`POST /v1/sessions/{id}/answer`). Go refuses unless the prompt is a confirmed cue
state and the window re-checks the prompt footer; Yes is sent only when the
highlighted choice is a Yes. The audit records `answer yes|no`
and the outcome, never text. A waiting Claude Code or Copilot CLI that shows something
other than a permission prompt (a question or choice list) is flagged `question` by the
window; only then does `send` accept text during the wait, so a permission prompt never
receives typed text. Choice lists are not parsed. Verified live on Claude Code. Revoking a
phone deletes its key and token; it must pair again, and pairing otherwise persists
across TauShell and app restarts (one-year expiry). The Android app (`android/`,
sideload APK) pins the certificate, keeps its token sealed with the Android Keystore
and sends the SNI name itself over its own TLS socket. Each device may send, start
or answer at most 20 times a minute (HTTP 429, audited as `rate-limited`). Not done yet: the relay. See
[docs/v3/implementation/04-mobile-companion-plan.md](../v3/implementation/04-mobile-companion-plan.md).

## Diagnostic logging

The per-run log is stored under the current user's system temp directory and
is replaced at process startup. It may contain operational errors and frontend
stack traces, plus coarse terminal protocol categories, pointer-event types,
and effective mouse-tracking mode changes without pointer coordinates. It
must not contain prompts, terminal input/output, launch argument values,
credential values, or environment values. Keep bridge failure request payloads
suppressed, bound frontend-supplied message/stack sizes, and redact
credential-like strings before writing error text.
