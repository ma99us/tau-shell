# Configuration Specification

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
