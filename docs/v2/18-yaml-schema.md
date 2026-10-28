# YAML Schema

Phase 1 uses one human-edited YAML file per TauShell session, stored under `~/.tau_shell/sessions/`. YAML describes the intended session; mutable process and harness runtime state belongs in SQLite.

```yaml
id: backend-agent
name: Backend Agent
color: "#4f8cff"   # optional #rrggbb tab/sidebar tint
auto_resume: true   # optional; resume this session when TauShell starts (Home tile checkbox)
tool: claude-code
working_dir: D:/Projects/App

environment:
  NODE_NO_WARNINGS: "1"

powershell:
  utf8: true

# Ordered, optional arguments for the selected harness:
launch_args:
  - --model
  - sonnet
```

`id` is TauShell's stable session identifier. The desktop app names new sessions after the project (manifest name, otherwise folder name) and renames or recolors them from the session context menu; those edits rewrite only `name` and `color` in place, preserving other keys and comments, and never change `id` or the file name. The harness session UUID is generated and persisted separately before first launch so TauShell can resume the intended conversation by ID. Do not add a `--continue`/“most recent” restore command to YAML.

The current implementation rejects unknown fields and validates the required
values, supported tool identifiers, working-directory existence, environment
variable names, UTF-8 setting, and filename/ID match before launch.
`launch_args` is implemented as an ordered list of discrete harness arguments,
not a shell command string. The list applies to both start and restore;
harness adapters supply the mode-specific managed session flags. TauShell
rejects arguments that would override its managed session/resume identity.
The desktop form accepts additional arguments one per line, but does not yet
show a complete editable YAML preview or generate harness-specific argument
defaults. TauShell treats saved YAML as the user's explicit configuration and
does not add a separate launch confirmation for configured arguments.

Do not store secret values in YAML; secret-reference support requires a
separately designed secret provider and is outside the current Phase 1 scope.
This schema may evolve with explicit compatibility handling.

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
harness is idle; see
[`08-UI-Design.md`](../v1/08-UI-Design.md#10-harness-widgets).

Skill library presets (from `tau-shell.yaml`) accept an extra `skill: <name>`
field and appear only while that skill is available and enabled. Layering is
library < global < session, by name.

The isolated [`spikes/session-config/`](../../spikes/session-config/) prototype records the earlier validation of strict YAML parsing and the separate runtime-state boundary.

## Cloning a session

"Clone" copies every definition field (harness, working directory, launch
arguments, environment, PowerShell options, presets, color) into a new file
named "<name> (n)" with a unique id. The copy is written by TauShell, so YAML
comments in the source are not carried over. Runtime state is never copied.

## App settings (`~/.tau_shell/config.yaml`)

Optional and hand-edited; a missing file means defaults. Unknown fields and
unsupported versions are rejected (reported once in the UI, defaults used).
"Edit settings..." in the command palette creates a commented example only when
the file is missing and never overwrites it. The file is re-read on every alert.

```yaml
version: 1
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
