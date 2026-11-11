# Harness Capability Research

Desk research of what the supported harnesses document that matters to a host
application, and what TauShell could adopt. It is not a requirement list and
not tested behavior; Phase 1 scope in `AGENTS.md` still wins.

Research date: 2026-11-10. Only Claude Code and Copilot CLI pages were fetched
fully enough to cite. The Codex, OpenCode and GSD pages were blocked or
unreachable (403/404), so those cells are **(U)**: from memory, unverified.
Re-verify a (U) cell against the installed version before relying on it.

## Sources

- Claude Code: <https://code.claude.com/docs/en/cli-reference>, <https://code.claude.com/docs/en/hooks>
- Copilot CLI: <https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference>, <https://docs.github.com/en/copilot/how-tos/copilot-cli/automate-copilot-cli/run-cli-programmatically>
- Codex: <https://developers.openai.com/codex/noninteractive> (blocked), <https://github.com/openai/codex> (`docs/config.md`, a stub)
- OpenCode: <https://opencode.ai/docs/> (blocked)
- GSD: the `gsd` binary is GSD Pi (<https://github.com/open-gsd/gsd-pi>, npm `@opengsd/gsd-pi`). The older `gsd-build/*` repos are archived and are a different product.

## Capability matrix

Verified cells carry a source; everything else is (U).

| Capability | Claude Code | Copilot CLI | Codex | OpenCode | GSD Pi |
|---|---|---|---|---|---|
| Headless | `claude -p`, piped stdin (verified); `--output-format json`/`stream-json` (U) | `-p`, piped stdin, `-s`, `--no-ask-user` (verified) | `codex exec --json` (U) | `opencode run`, `--format json` (U) | (U) |
| Resume | `-c`, `-r <id\|name>` (verified); fork (U) | `--continue`/`--resume` (U) | `codex resume`/`fork` (U) | `-c`, `-s <id>`, `--fork` (U) | (U) |
| Model switch in session | `/model` (U) | `/model` (U) | `/model` (U) | `/models` opens a picker; no `/model <id>` | (U) |
| Model on launch | `--model`, `ANTHROPIC_MODEL` (U) | `--model` (verified) | `-m`, `config.toml` (U) | `-m provider/model`, `opencode.json` (U) | (U) |
| Lifecycle hooks | Rich: `SessionStart`, `Notification`, `PermissionRequest`, `Stop`, `StopFailure`, `Pre/PostCompact`, `SessionEnd`, `Pre/PostModelSwitch`; command and HTTP handlers (verified) | `help monitoring` topic exists; contents (U) | Hooks exist (verified); `notify`, `[otel]` (U) | Plugin events (U) | (U) |
| Permissions | `--permission-mode`, auto mode (verified) | `--allow-all`/`--yolo`, `--allow-tool`, `--allow-url` (verified) | `--ask-for-approval`, `--sandbox` (U) | `permission` config (U) | (U) |
| Host interfaces | SDK, `claude agents --json`, `attach`, `logs`, `daemon` (verified) | `copilot app`; ACP/server (U) | `codex app-server` (JSON-RPC) (U) | `opencode serve`, `opencode acp` (U) | `gsd --web` (verified); RPC (U) |
| Instructions file | `CLAUDE.md` | `copilot init`, `copilot instruction` (verified) | `AGENTS.md` (U) | `AGENTS.md` (U) | `.gsd/` (verified) |
| MCP / skills | `claude mcp`, `plugin`, `agents` (verified) | `copilot mcp`, `skill`, `plugin` (verified) | MCP in `config.toml` (U) | MCP in config (U) | Extensions, skills, commands (verified) |

## Things most harnesses share that TauShell could adopt

Ordered by value to TauShell.

1. **Structured lifecycle events instead of screen reading.** Claude Code documents
   idle, permission and stop events with HTTP handlers, which avoids Windows shell
   quoting. TauShell could receive them on a loopback listener and treat the screen
   cue only as a fallback. Needs a per-launch overlay (flags or env), never edits to
   the user's settings. Copilot, Codex and OpenCode equivalents are (U).
2. **A server or protocol interface.** `opencode serve`/`acp`, `codex app-server`
   and Claude Code's SDK are (mostly U) alternatives to terminal scraping. ACP is the
   most portable target if two or more harnesses confirm it.
3. **OpenTelemetry usage import.** One local OTLP receiver could replace per-harness
   usage readers where a harness exports it (Claude Code, Codex: (U)).
4. **Headless runs.** Background jobs (session titles, "summarize this session")
   without opening a terminal. Verified for Claude Code and Copilot.
5. **One auto-approve toggle** mapped to each harness's flag, shown with the
   existing visible-interruption rules.
6. **Shared instructions and MCP list.** `AGENTS.md` is read by most; one source
   written to each harness's expected file.
7. **Fork a conversation** where the harness supports it.

## Risks

- Documentation drifts quickly (Claude Code ships features almost daily); gate
  features on the detected version and keep the tested ranges in the manifest.
- Copilot CLI's hook and ACP support is unverified; screen reading may stay necessary.
- Hook commands run in the harness's shell on Windows; prefer HTTP handlers.
- No harness was confirmed to honor `COLORFGBG`; do not assume it.
- Editing a user's harness settings to inject hooks is invasive; prefer launch-scoped
  flags and environment variables.
