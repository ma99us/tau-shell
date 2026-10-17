# Configuration Specification

> **Historical draft:** This is not the current Phase 1 schema. Use
> [docs/v2/18-yaml-schema.md](../v2/18-yaml-schema.md) and
> [docs/v2/10-environment-management.md](../v2/10-environment-management.md).
> In particular, `auto_restore`, arbitrary startup commands, and secret
> interpolation in YAML are not current Phase 1 behavior. The planned
> `launch_args` list for editable harness CLI options is documented in the
> current [YAML schema](../v2/18-yaml-schema.md); it is not the same as a
> free-form startup command.

Root Folder:

```text
~/.tau_shell/
```

## Layout

```text
.tau_shell/
├─ config.yaml
├─ sessions/
├─ projects/
├─ plugins/
├─ snapshots/
└─ logs/
```

## Session Example

```yaml
id: backend-api
name: Backend API

tool: claude
shell: pwsh

working_dir: D:/Projects/Backend

auto_restore: true

environment:
  ANTHROPIC_API_KEY: ${SECRET_ANTHROPIC}
  OPENAI_API_KEY: ${SECRET_OPENAI}
  NODE_NO_WARNINGS: 1

powershell:
  utf8: true
  code_page: 65001

startup:
  - claude --resume
```
