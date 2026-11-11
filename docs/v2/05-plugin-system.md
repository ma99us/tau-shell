# Plugin System

## Phase 1

Harnesses are described by declarative YAML manifests: the five built-in ones
(Claude Code, GitHub Copilot CLI, GSD, OpenAI Codex CLI, OpenCode) are
embedded in the binary, and users can add their own in
`~/.tau_shell/harnesses/` (confirmed once per manifest content on Home). A
manifest declares data; log parsing stays in named Go readers that a manifest
selects by name. A general plugin manager and third-party code loading are
still later work. See [`23-adding-a-harness.md`](23-adding-a-harness.md) for
the schema, trust model and readers.

Each harness (manifest plus the generic adapter) provides:

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
current support or tested-version matrix. Use the built-in manifests in
[`internal/harness/manifest/builtin/`](../../internal/harness/manifest/builtin/) for current ranges,
and the [engineering handoff](21-engineering-handoff.md) for live-verification
status.

| Harness | New managed conversation | Resume |
| --- | --- | --- |
| Claude Code | `claude --session-id <uuid>` | `claude --resume <uuid>` |
| GitHub Copilot CLI | `copilot --session-id <uuid>` | `copilot --resume <uuid>` |
| GSD (>= 1.12.0) | `gsd --session <path>` (TauShell-owned file under `~/.gsd/sessions/tau/`) | same `--session <path>` |
| OpenAI Codex CLI | `codex` (ID discovered from the rollout log) | `codex resume <id>` |
| OpenCode | `opencode` (ID discovered from `opencode.db`) | `opencode --session <id>` |

Codex and OpenCode choose their own conversation IDs. TauShell records the ID after launch by matching the new conversation to the session's working directory, skipping IDs owned by other sessions; an ambiguous match records nothing and the session shows "Resume ID unknown". Codex 0.166.0 was verified through TauShell and ConPTY: it created a resumable ID, answered prompts, stopped gracefully, restored the same conversation and recalled prior context. Its `task_started`/`task_complete` rollout events were observed as working/idle activity.

The locally installed Claude Code 2.1.263 and Copilot CLI 1.0.96-2 help output documents these options. This is CLI-contract discovery only; successful conversation restoration still requires a controlled integration test. Avoid `--continue` for managed restore because it selects the most recent session rather than TauShell's recorded session.

To add another harness, follow [`23-adding-a-harness.md`](23-adding-a-harness.md).

### Widget metadata

An adapter's `Spec.Widgets` (`internal/tools/tools.go`) declares the
quality-of-life actions TauShell may offer for a running session: slash
`Commands`, suggested `Models` for `/model <id>`, the `InterruptKeys` pressed in
order (Copilot CLI needs Esc twice; Claude Code one Esc), and the `CompactText`
command. The UI shows a widget only for adapters that declare it, and the
Model menu additionally requires the `--model` capability probe. These are
keystrokes typed into the harness, not a harness-confirmed action. If the
composer is empty, TauShell submits the action (unless the harness is waiting
for input); if it already contains a draft, TauShell appends the action and
leaves it unsent for the user to review. Prompt presets always request
submission, so the per-preset `submit` flag no longer applies. A harness without
`/model <id>` (OpenCode) declares `model_picker` (the picker command, here
`/models`); each `models` entry is then typed into the picker search box and
confirmed with Enter (and a second Enter that accepts the default reasoning variant when the model has variants), and only with an empty composer. The
lists are tied to the tested version range. Codex 0.166.0's declared command
widgets are limited to `/compact` and `/status`; its live test also verified
single-Esc interruption, opening the model picker, switching to another
available model, and successful compaction. `/new` and `/help` are not exposed
because their behavior was not verified. Adding a harness means declaring
verified actions or leaving `Widgets` nil, in which case the terminal has no
widgets.

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
appends the adapter's declared skill-directory option. Copilot CLI and Claude
Code use `--plugin-dir <dir>`. Codex uses `--add-dir <dir>` and receives a
per-launch `.agents/skills` copy so it cannot modify a shared skill snapshot.
The route is skipped when the session sets `skills: false`, when the adapter
has no verified route, when the relevant CLI help does not list the option, or
when preparing fails; each case is logged and never blocks launch. Codex
0.166.0's native project and additional-directory discovery were first checked
with disposable `codex exec --ephemeral` probes; the TauShell per-launch skill
route then returned its expected response in the live ConPTY test. That test
uses and removes a temporary Codex profile trusting only its disposable
project. Copilot CLI 1.0.105 lists skills; Claude Code 2.1.312 validates the
manifest only. Resume with skill directories and same-name duplicates across
sources remain untested.

Future adapters may add supported actions and state hints, but tool-specific behavior must remain out of the core session manager.
