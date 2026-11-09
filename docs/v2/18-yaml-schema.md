# YAML Schema

Phase 1 uses one human-edited YAML file per TauShell session, stored under `~/.tau_shell/sessions/`. YAML describes the intended session; mutable process and harness runtime state belongs in SQLite.

```yaml
id: backend-agent
name: Backend Agent
color: "#4f8cff"   # optional #rrggbb tab/sidebar tint
link: https://github.com/o/r/pull/12   # optional http(s) Jira issue or PR link (max 500 chars)
notes: "Review before Friday"   # optional free text (max 500 chars); URLs become links
tags: ["review", "VTA-123"]   # optional, up to 8; no whitespace, so a URL is a valid tag
auto_resume: true   # optional; resume this session when TauShell starts (Home tile checkbox)
tool: claude-code
working_dir: D:/Projects/App

environment:
  NODE_NO_WARNINGS: "1"

# Optional: ENV_VAR -> name of a secret in the vault (Home > Secrets).
secrets:
  OPENAI_API_KEY: openai-main

powershell:
  utf8: true

# Optional WSL runtime (default is PowerShell). working_dir is then an absolute
# Linux path in the distro (not checked on disk, so the distro is not started).
# runtime: wsl
# wsl:
#   distro: Ubuntu

# Ordered, optional arguments for the selected harness:
launch_args:
  - --model
  - sonnet
```

`id` is TauShell's stable session identifier. The desktop app names new sessions after the project (manifest name, otherwise folder name) and renames or recolors them from the session context menu; those edits rewrite only `name` and `color` in place, preserving other keys and comments, and never change `id` or the file name. TauShell serializes config writers per path and checks that the file has not changed before replacement; an observed external edit returns a conflict to reload and retry. Non-cooperating external editors cannot be locked out, and a change after the recheck can still race. Supported tool values: claude-code, copilot-cli, gsd, codex, opencode. Default launch arguments: Claude --dangerously-skip-permissions, Copilot --yolo, Codex --dangerously-bypass-approvals-and-sandbox, OpenCode --auto, GSD none. Harness conversation IDs are stored separately from YAML: TauShell chooses the Claude and Copilot IDs and names GSD's session file; Codex and OpenCode choose IDs that TauShell discovers after launch. Do not add a `--continue`/“most recent” restore command to YAML.

The current implementation rejects unknown fields and validates the required
values, supported tool identifiers, working-directory existence, environment
variable names, UTF-8 setting, and filename/ID match before launch.
`launch_args` is implemented as an ordered list of discrete harness arguments,
not a shell command string. The list applies to both start and restore;
harness adapters supply the mode-specific managed session flags. TauShell
rejects arguments that would override its managed session/resume identity.
The desktop form supplies editable harness-specific defaults and accepts
additional arguments one per line. Its full YAML preview follows the form until
hand-edited, and the edited definition is validated before saving. A newly
created session is selected and started immediately. TauShell treats saved
YAML as the user's explicit configuration and does not add a separate launch
confirmation for configured arguments.

`runtime` is `powershell` (default, may be omitted) or `wsl`; `wsl.distro` is required
for `wsl` (1 to 64 letters, digits, dots, dashes or underscores) and rejected
otherwise. The runtime is chosen when a session is created
and frozen for clones. WSL sessions support Claude Code, Copilot CLI and GSD
(GSD launches only; its logs are not read in WSL homes); `codex` and `opencode`
are not offered and are rejected at launch because TauShell cannot discover their
session IDs inside the distro.
See [`docs/wsl/`](../wsl/).

Do not store secret values in YAML. The `secrets:` mapping contains names of
values stored through Home > Secrets in the current user's Windows Credential
Manager; TauShell resolves them into the child environment at launch and
reports a missing value as an error. See the
[security model](11-security-model.md#secret-vault) for the same-user and
process-environment limitations. This schema may evolve with explicit
compatibility handling.

## Prompt presets

Prompt presets are user-authored text TauShell types into a running harness.
They are plain prompts, not validated harness commands.

Global presets live in `~/.tau_shell/prompts.yaml`. A missing file means no
presets; an invalid file is reported in the Prompts menu, not ignored. The file
is rejected on unknown fields, a missing `name`/`text`, or duplicate names
(case-insensitive). "Edit presets..." in the Prompts menu creates a commented
example only when the file is missing, then opens it; TauShell never overwrites
it.

```yaml
version: 1
presets:
  - name: Review uncommitted changes
    text: "Review the uncommitted changes in {project} on branch {branch}."
    tools: [claude-code, copilot-cli]   # optional; default is all harnesses
    submit: true                        # optional; false = insert only
```

A session YAML may add a `presets:` list in the same entry format. Session
entries are appended after the global ones and replace a global preset with the
same name. Variables: `{project}` (working-directory name), `{dir}`,
`{session}` (session name), `{branch}` (current git branch, empty outside a
repository), `{date}` (YYYY-MM-DD). Unknown `{name}` tokens are left as written.
Do not put secrets in presets. `submit: true` presses Enter only when the
harness is idle; see the current
[widget delivery notes](21-engineering-handoff.md#harness-widgets-implemented).
The [V1 UI design](../v1/08-UI-Design.md#10-harness-widgets) is historical
context.

Skill library presets (from `tau-shell.yaml`) accept an extra `skill: <name>`
field and appear only while that skill is available and enabled. Layering is
library < global < session, by name.

The isolated [`spikes/session-config/`](../../spikes/session-config/) prototype records the earlier validation of strict YAML parsing and the separate runtime-state boundary.

## Harness manifests (`~/.tau_shell/harnesses/*.yaml`)

User-added harnesses are declarative YAML manifests; the schema, readers and
trust model are in [`23-adding-a-harness.md`](23-adding-a-harness.md). A
session's `tool:` may name any loaded harness. Confirmation of user manifests
is runtime state in `~/.tau_shell/harness-trust.json` (content hash), not in
session YAML.

## Cloning a session

"Clone" opens the New session dialog with the original's project folder shown
read-only (a clone always inherits it) and its harness and launch arguments
prefilled and editable. It then copies every other definition field
(environment, PowerShell options, presets, color) into a new file named
"name (n)" with a unique id and starts it. The copy is written by TauShell, so
YAML comments in the source are not carried over. Runtime state is never copied.

## App settings (`~/.tau_shell/config.yaml`)

Optional and hand-edited; a missing file means defaults. Unknown fields and
unsupported versions are rejected (reported once in the UI, defaults used).
"Edit settings..." in the command palette creates a commented example only when
the file is missing and never overwrites it. The file is re-read on every alert.

```yaml
version: 1
control:
  enabled: false       # control pipe for "taushell ctl"; the Home switch edits this line in place
  agents: false        # reserved: let programs inside sessions use it (not active yet)
mobile:
  enabled: false       # phone server (experimental); the Home Phone switch edits this line in place
  interface: ""        # the one local IP to listen on, required when enabled; never all interfaces
  port: 47821          # 1024-65535
  screen: true         # let paired phones read a running session's visible terminal text; false keeps it on the PC
  presets:             # optional quick replies added to each phone's own replies
    - Run the tests
notifications:
  toast: true          # Windows toast while TauShell is in the background
  taskbar_flash: true  # flash the taskbar button; both default to true
usage:
  month_start_day: 1   # 1-28; day the monthly totals reset (default 1)
  price_updates: true  # refresh Claude prices daily from Anthropic's pricing page
  claude_prices:       # optional USD per million tokens; cache prices default to
    opus-5.5: { input: 4, output: 20 }  # 1.25x / 2x / 0.1x input when omitted
```

Skills block (all optional):

```yaml
skills:
  enabled: true        # false turns the skill library off
  repo: https://github.com/mgerdov-igt/ai-cookbook  # "" disables fetching
  ref: main            # branch, tag or commit
  auto_update: false   # true applies new commits without a click
  local_sources:       # absolute folders with skills/ (and tau-shell.yaml); win over GitHub
    - C:/Projects/ai-cookbook
```

A session YAML may set `skills: false` to skip loading the library for that
session. See [`05-plugin-system.md`](05-plugin-system.md).

`usage.claude_prices` entries win over fetched prices, which win over the
built-in table. Invalid values reject the file like any other unknown field.
