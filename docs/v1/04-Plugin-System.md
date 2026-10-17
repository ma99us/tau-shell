# Plugin System

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
