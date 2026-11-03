# Environment Management

Environment variables are session scoped.

Plain values go under `environment:`. Secrets go under `secrets:`, which maps an
environment variable to the name of a vault secret:

```yaml
environment:
  NODE_NO_WARNINGS: "1"

secrets:
  OPENAI_API_KEY: openai-main
  ANTHROPIC_API_KEY: anthropic-main
```

Secret values live in Windows Credential Manager (generic credentials named
`TauShell/secret/<name>`, current user, local machine only). Add, replace and
delete them under **Secrets** on Home; values are write-only in the UI. At launch
TauShell reads each referenced secret and places it in the session's process
environment only (never the command line, YAML, state database, logs or bridge
payloads). A variable cannot be in both blocks. If a referenced secret is missing
the launch fails with a message naming the secret and the session state is not
changed. Duplicating a session copies the secret names, never the values.
