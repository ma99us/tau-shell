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
delete them under **Secrets** on Home; values are write-only in the UI. Each
secret shows its `{ENV_VAR: name}` reference with a copy button, for pasting into
the `secrets:` block. At launch
TauShell reads each referenced secret and places it in the session's process
environment (never the command line, YAML, state database, logs or bridge
responses). Entering a value in the UI sends it to the host in the write-only
`SetSecret` request. A variable cannot be in both blocks. If a referenced
secret is missing the launch fails with a message naming the secret and the
session state is not changed. Duplicating a session copies the secret names,
never the values.

## WSL sessions

Session `environment:` values and resolved vault secrets cross into the distro
through `WSLENV` entries on the `wsl.exe` process environment, never in argv or
logs. Skill folders are passed as Linux-visible paths (for example
`/mnt/c/...`). The default PATH inside `--exec` is minimal, so TauShell passes an
explicit `PATH` resolved at launch. Design, status and backlog: [`docs/wsl/`](../wsl/).
