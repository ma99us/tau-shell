# 02 - Design (recommended: per-session runtime)

Goal: let a session run its harness in a WSL2 distro instead of PowerShell 7,
without changing what the rest of TauShell does. PowerShell stays first-class and
stays the default.

## 1. Runtime abstraction

Introduce one small interface in a new package (for example `internal/runtime`),
chosen per session. Subsystems ask the runtime instead of assuming Windows.

```go
type Runtime interface {
    Kind() string                                   // "powershell" | "wsl"
    Available(ctx context.Context) error            // pwsh on PATH / distro exists and starts
    Command(spec LaunchSpec) (exe string, args []string, env []string, err error)
    HomeDir(ctx context.Context) (string, error)    // where harness state lives, readable from Windows
    FindTool(ctx context.Context, name string) (string, error)
    Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) // git, --version, --help
    ToNativePath(windowsPath string) (string, error) // path as the harness sees it
    ToWindowsPath(nativePath string) (string, error) // path TauShell can open
    Cleanup(ctx context.Context, launch LaunchInfo) error // extra cleanup after the job object
}
```

- `powershellRuntime` wraps today's behavior unchanged (`pwsh.exe -NoLogo
  -NoProfile -Command`, `PowerShellInvocation`).
- `wslRuntime{distro, user}` implements the same calls through `wsl.exe`.
- `session.Config` gains `Runtime` (default `powershell`) and `WSL.Distro`.
- Subsystems that call `os.UserHomeDir()`, `exec.LookPath`, or `git -C` take the
  session's runtime. Subsystems that are not session-scoped (global detection)
  report per runtime.

This follows the existing pattern: core stays tool-independent, and specifics
live in one adapter.

## 2. Launch

```
wsl.exe -d <distro> --cd <linux dir> --exec <harness path> <argv...>
```

- `--exec` avoids a shell, so the existing rule holds: managed session and resume
  arguments stay under adapter control, and user arguments are discrete argv
  values (probe: `--exec` kept `a b` and `$HOME` literal).
- No PowerShell quoting and no UTF-8 prologue. Linux defaults to UTF-8. Prove
  end-to-end rendering (ConPTY plus xterm.js) in the spike; process-level UTF-8
  alone is not proof.
- `--cd` accepts a Linux path. A Windows working directory is auto-mapped to
  `/mnt/<drive>/...` (probe), but projects in the distro filesystem should store
  the Linux path.
- Resolve the harness to an absolute path through the runtime (`FindTool`), so
  PATH differences between interactive and non-interactive shells do not matter.
- Do not depend on a login shell (probe: systemd user session failure). If a
  harness needs PATH from the user's profile, supply an explicit `PATH`
  environment entry resolved once at launch.

## 3. Paths

- YAML intent keeps `working_dir`. For WSL sessions it is a Linux path with
  `wsl.distro`, or a `\\wsl.localhost\<distro>\...` path that TauShell normalizes
  to a Linux path plus distro. Windows drive paths are allowed and mapped to
  `/mnt/<drive>/...` (slower I/O; warn in the create dialog).
- All path hand-offs go through `ToNativePath` and `ToWindowsPath`: file-drawer
  "insert path", pasted-image paths, skill directories, reveal in Explorer.
- Never pass backslash paths through `wsl.exe` arguments (probe: they are
  eaten); use forward slashes or `--exec` argv.

## 4. Harness state readers

`HomeDir` returns `\\wsl.localhost\<distro>\home\<user>` for WSL. The readers
(`harness.*`, `usage.*`, `quota/litellm`) take a root from the runtime; their
existing `...In(root, ...)` test seams make this mostly mechanical.

- Do not start a stopped distro just to read state. If it is not running,
  readers report "no data" and the UI shows "distro stopped". A running session
  implies a running distro.
- UNC reads are slower: keep tail reads bounded, reuse the incremental offsets
  (`usage_files`), and never walk whole trees on the UI thread.
- Resolve the Linux username once (`id -un` through `--exec`) and cache it.

## 5. Detection, install and versions

- Lookup runs inside the distro: `command -v <name>` through `--exec sh -c` with
  the name as a positional argument (no interpolation). **Reject any result under
  `/mnt/`.** The probe showed WSL PATH interop returns Windows npm shims and a
  VS Code `copilot` shim that are not Linux installs.
- Versions and capability probes (`--version`, `--help`, skill-dir option) run
  through `Runtime.Run` with the existing timeouts. Per-adapter tested ranges and
  capability probes are unchanged.
- Installs and updates: guidance only. Show the distro, the probable command
  and a copy button; never run `sudo`, never install silently, never elevate.
  Spike findings (`05-spike-results.md`): use the distro's own `npm` with an
  explicit prefix (a bare `npm` can be the Windows npm), put the harness's Node
  first on PATH at launch, include `--allow-scripts=<pkg>` where npm blocks
  install scripts, and warn about corporate TLS inspection
  (`NODE_EXTRA_CA_CERTS`). Re-detect the requested version before reporting
  success (existing rule).

## 6. Environment and secrets

- Build the child environment on Windows as today (session-scoped, secrets from
  the vault), add each forwarded name to `WSLENV` (probe verified), and start
  `wsl.exe` with that environment. Values never appear in argv, YAML, logs or
  diagnostics.
- Use `NAME/u` flags only where needed; do not forward Windows path variables;
  strip Windows-only variables.
- Document that values are visible to processes inside the distro (same trust
  boundary as the user's own session).

## 7. Process lifetime and termination

- The ConPTY child is `wsl.exe`, owned by the existing job object. Probe:
  killing `wsl.exe` ended its Linux child.
- Graceful stop is unchanged (interrupt keys, `/exit`, short wait, then force).
- Forced stop adds a bounded defense: after terminating `wsl.exe`, look for
  survivors (for example by a per-launch marker environment variable or a
  recorded process group) and terminate them inside the distro. The exact
  handshake is decided in the Phase 0 spike. Make interruption and forced
  termination visible (existing rule).
- Closing TauShell keeps today's confirmation, interrupt, wait and force flow.

## 8. Git

Run the existing `presets` Git functions through `Runtime.Run` in the session
directory. Parsers and the Go-side ownership of branch and status data stay
unchanged ("Git branch and status data come from the Go host"). Credentials and
`safe.directory` come from the distro's git config. Fetch and sync timeouts
stay; errors surface unmodified.

## 9. Config, UI, setup

YAML (additive, validated):

```yaml
runtime: wsl            # omitted = powershell
wsl:
  distro: Ubuntu
```

- Create-session dialog: runtime picker; distro dropdown from `wsl -l -q`
  (decode UTF-16LE); warning for `/mnt/c` working directories.
- Top bar: show the runtime and distro next to the tool.
- Setup and Home: PowerShell is a hard gate only for PowerShell sessions. A new
  "WSL" prerequisite row reports: `wsl.exe` present, version, distros, distro
  running, native harness found, `/mnt` shim warnings.
- Phone server, queue, attention cues and usage are unchanged.

## 10. Security notes

- Phone and terminal guards (explicit pairing, pinned TLS, authentication,
  revocation, send and answer guards) are unchanged; the runtime sits behind
  the session.
- Never log prompt text, terminal contents or environment values.
- `wsl.exe` arguments for distro, user and path come from validated config;
  reject control characters and option-like values (leading `-`).
- The files drawer stays read-only; resolve and contain paths after UNC conversion.

## 11. Open design questions

1. Store `working_dir` as Linux path plus distro, or as UNC? (Recommend Linux path plus distro.)
2. Should TauShell ever run `wsl --install`? (Recommend never; link to Microsoft docs.)
3. Copy skill snapshots into the distro or read them from `/mnt/c`? (Recommend `/mnt/c` first; copy if I/O or permission problems appear.)
4. Support WSL1? (Recommend no: require WSL2, detect and warn.)
5. Multiple users per distro? (Recommend the default user only.)
