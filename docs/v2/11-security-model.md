# Security Model

## Principles

- Least privilege and local-first operation.
- Do not store secret values in session YAML or expose them to the UI renderer.
- Keep session environments and managed process trees isolated.
- Validate every desktop UI bridge request; expose only the operations the
  trusted local UI needs.
- Do not silently install software, elevate privileges, or execute downloaded
  scripts.

## Tool installation

TauShell may offer installation of known prerequisites and supported harnesses.
An install action must identify the package and source and require user
confirmation before invoking winget. If winget is unavailable, direct the user
to official vendor installation instructions. Re-detect the executable and
version after installation; do not treat the installer's exit code alone as
proof of success.

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

## Diagnostic logging

The per-run log is stored under the current user's system temp directory and
is replaced at process startup. It may contain operational errors and frontend
stack traces, plus coarse terminal protocol categories, pointer-event types,
and effective mouse-tracking mode changes without pointer coordinates. It
must not contain prompts, terminal input/output, launch argument values,
credential values, or environment values. Keep bridge failure request payloads
suppressed, bound frontend-supplied message/stack sizes, and redact
credential-like strings before writing error text.
