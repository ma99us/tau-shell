# YAML Schema

```yaml
id: backend
name: Backend Agent
tool: claude
auto_restore: true
working_dir: D:/Projects/App

environment:
  OPENAI_API_KEY:
    secret: openai-main

powershell:
  utf8: true

actions:
  startup:
    - claude --resume
```
