# Plugin System

## Phase 1

Use two small built-in harness adapters for Claude Code and GitHub Copilot CLI. Keep this interface internal; a general plugin manager, third-party plugin loading, UI actions, and plugin SDK are later work.

Each adapter should provide:

- Harness identity, executable detection, version information, and declared
  minimum/tested version ranges.
- An initial launch command that uses a caller-supplied session UUID.
- A resume command that targets that same explicit UUID.
- Capability probes for the launch/resume options and adapter operations TauShell
  intends to use.
- A clear error when the executable is absent or the CLI does not support the required option.

The common session manager owns persistence, process/ConPTY lifecycle, configuration, and runtime state. Adapters only translate TauShell launch/resume intent into harness-specific CLI arguments.

Session YAML is planned to support one ordered `launch_args` list of
user-configured harness options, applied to both fresh start and restore.
These are discrete process arguments, not a shell command string. The adapter
remains responsible for the mode-specific TauShell-managed session/resume
identifier and must prevent a configured argument from overriding that
identity. Generated session templates may provide editable defaults for each
harness. This schema extension is planned and is not part of the current
strict parser until implemented.

### Version support and tool setup

Use the installed version rather than silently pinning, downgrading, or
updating a harness. Record minimum requirements and tested ranges per adapter;
warn when installed versions fall outside those ranges. Probe needed
capabilities and report missing/regressed features explicitly. A version
outside the tested range alone is not a reason to block launch if required
capabilities are present.

At startup, discover supported harness executables and versions. Missing
harnesses do not block the desktop app; the UI offers a user-confirmed install
through a known package source or an official vendor setup link. Verify
installation by detecting the executable and version again. Show the resolved
executable path alongside its version so users can identify which installation
won PATH resolution; do not silently prefer a newer or separately installed
copy.

## Initial CLI options observed

| Harness | New managed conversation | Resume |
| --- | --- | --- |
| Claude Code | `claude --session-id <uuid>` | `claude --resume <uuid>` |
| GitHub Copilot CLI | `copilot --session-id <uuid>` | `copilot --resume <uuid>` |

The locally installed Claude Code 2.1.263 and Copilot CLI 1.0.96-2 help output documents these options. This is CLI-contract discovery only; successful conversation restoration still requires a controlled integration test. Avoid `--continue` for managed restore because it selects the most recent session rather than TauShell's recorded session.

To add another harness, follow [`23-adding-a-harness.md`](23-adding-a-harness.md).

### Widget metadata

An adapter's `Spec.Widgets` (`internal/tools/tools.go`) declares the
quality-of-life actions TauShell may offer for a running session: slash
`Commands`, suggested `Models` for `/model <id>`, the `InterruptKeys` pressed in
order (Copilot CLI needs Esc twice; Claude Code one Esc), and the `CompactText`
command. The UI shows a widget only for adapters that declare it, and the
Model menu additionally requires the `--model` capability probe. These are
keystrokes typed into the harness: they are not confirmed by it, and the lists
are tied to the tested version range. Adding a harness means declaring them or
leaving `Widgets` nil, in which case the terminal simply has no widgets.

Future adapters may add supported actions and state hints, but tool-specific behavior must remain out of the core session manager.
