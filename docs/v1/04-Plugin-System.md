# Plugin System

> **Historical sketch:** The per-tool YAML command definitions below are not
> the current Phase 1 integration model. Harness-specific launch and resume
> behavior belongs behind the tool-specific adapter boundary described in
> [docs/v2/05-plugin-system.md](../v2/05-plugin-system.md).

## Purpose
Provide detection, startup, resume logic and automation actions.

## Claude Example

```yaml
tool: claude

detect:
  executable:
    - claude

launch:
  command: claude

resume:
  command: claude --resume

actions:
  opus:
    send: /model opus

  sonnet:
    send: /model sonnet

  compact:
    send: /compact
```
