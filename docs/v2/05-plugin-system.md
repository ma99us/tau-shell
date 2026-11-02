# Plugin System

## Phase 1

Use built-in adapters for the supported CLI harnesses: Claude Code, GitHub
Copilot CLI, GSD, OpenAI Codex CLI, and OpenCode. Keep the interface internal;
a general plugin manager, third-party plugin loading, UI actions, and plugin
SDK are later work.

Each adapter should provide:

- Harness identity, executable detection, version information, and declared
  minimum/tested version ranges.
- An initial launch command that uses a caller-supplied session UUID.
- A resume command that targets that same explicit UUID.
- Capability probes for the launch/resume options and adapter operations TauShell
  intends to use.
- A clear error when the executable is absent or the CLI does not support the required option.

The session manager owns persistence, process/ConPTY lifecycle, configuration,
and runtime state. Adapters provide harness-specific argv, resume checks,
discovery and optional capability metadata. Shared discovery policy filters
pre-launch IDs and atomically claims only an unambiguous candidate; adapters
own the harness-specific store enumeration and parsing. A harness with a
TauShell-chosen ID uses `StartArgs`/`ResumeArgs`; Codex/OpenCode instead expose
pre-launch `ExistingSessionIDs` and post-launch `DiscoverSessionID`. Discovery
still correlates by working directory and launch time; an external conversation
created after the baseline can remain indistinguishable and must not be claimed
when multiple candidates match.

Session YAML supports one ordered `launch_args` list of user-configured
harness options, applied to both fresh start and restore.
These are discrete process arguments, not a shell command string. The adapter
remains responsible for the mode-specific TauShell-managed session/resume
identifier and must prevent a configured argument from overriding that
identity. Generated session templates provide editable defaults for supported
harnesses.

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

## Historical CLI option snapshot

The versions below record earlier CLI-contract discovery; they are not the
current support or tested-version matrix. Use the adapter declarations in
[`internal/tools/tools.go`](../../internal/tools/tools.go) for current ranges,
and the [engineering handoff](21-engineering-handoff.md) for live-verification
status.

| Harness | New managed conversation | Resume |
| --- | --- | --- |
| Claude Code | `claude --session-id <uuid>` | `claude --resume <uuid>` |
| GitHub Copilot CLI | `copilot --session-id <uuid>` | `copilot --resume <uuid>` |
| GSD (>= 1.12.0) | `gsd --session <path>` (TauShell-owned file under `~/.gsd/sessions/tau/`) | same `--session <path>` |
| OpenAI Codex CLI | `codex` (ID discovered from the rollout log) | `codex resume <id>` |
| OpenCode | `opencode` (ID discovered from `opencode.db`) | `opencode --session <id>` |

Codex and OpenCode choose their own conversation IDs. TauShell records the ID after launch by matching the new conversation to the session's working directory, skipping IDs owned by other sessions; an ambiguous match records nothing and the session shows "Resume ID unknown". Tested: GSD 1.22.0, OpenCode 1.19.5 (CLI help and database schema), Codex 0.165.0 (CLI help only; no signed-in Codex session was available, so the rollout format, discovery and usage parsing are unverified against a live Codex).

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

### Skill library

`internal/skills` provides one harness-neutral skill library. Sources, first
match wins: local folders (`skills.local_sources`), the applied GitHub snapshot
(`skills.repo`, default the ai-cookbook repo; its `tau-shell.yaml` lists the
exposed skills and prompt presets, each preset gated by `skill:`), then the
bundled snapshot (`internal/skills/bundled`, refreshed with
`scripts/sync-cookbook-skills.ps1`). Skills the user already has personally
(Claude `~/.claude/skills`, Copilot `~/.copilot/skills`, `~/.agents/skills`) are
left out so the personal copy wins.

At launch and resume the session manager asks the library for a content-hashed
plugin folder (`.claude-plugin/plugin.json`, `plugin.json`, `skills/<name>`) and
appends `--plugin-dir <dir>` after the adapter's own arguments. It is skipped
when the session sets `skills: false`, when the adapter has no verified skill
directory route, when `--help` does not list
`--plugin-dir`, or when preparing fails; each case is logged and never blocks
launch. Tested: Copilot CLI 1.0.105 lists the skills; Claude Code 2.1.312
validates the manifest only. `--resume` combined with `--plugin-dir` and
same-name duplicates across sources are untested.

Future adapters may add supported actions and state hints, but tool-specific behavior must remain out of the core session manager.
