# YAML Schema

Phase 1 uses one human-edited YAML file per TauShell session, stored under `~/.tau_shell/sessions/`. YAML describes the intended session; mutable process and harness runtime state belongs in SQLite.

```yaml
id: backend-agent
name: Backend Agent
color: "#4f8cff"   # optional #rrggbb tab/sidebar tint
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

The isolated [`spikes/session-config/`](../../spikes/session-config/) prototype records the earlier validation of strict YAML parsing and the separate runtime-state boundary.
