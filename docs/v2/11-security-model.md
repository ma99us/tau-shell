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
