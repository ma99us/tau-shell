# 11 - Harness Conformance and Common Harness Improvements

Status: **future planning only**. Not ranked in the Top 10. Source: the [harness capability research](../../v2/28-harness-capability-research.md) (only Claude Code and Copilot CLI cells were verified; Codex, OpenCode and GSD cells are unverified) and the live results in [`21-engineering-handoff.md`](../../v2/21-engineering-handoff.md).

## Problem

Every harness must behave the same way in TauShell: graceful shutdown and resume, launch options, popular commands, model selection, accurate working / idle / input detection, token and cost tracking, and current-model tracking. Today this is proven by one large live test (`TestLiveHarness`) with per-tool branches, plus manual probes. There is no single view of how well each harness, or each harness version, plays with TauShell.

## User stories

- As a contributor, I want a repeatable evaluation for a new harness or a new version, so that I can see what works before shipping.
- As a user, I want to see which TauShell features each harness supports and how far that was verified.

## In scope

### A. Conformance suite

- A capability matrix as data: one row per capability (start, resume confirmed, graceful stop, popular commands, model switch, state cues, usage, cost, current model, skills), one column per harness and tested version.
- Shared live probes split out of `TestLiveHarness`, so each capability is a named check.
- A runner script (for example `scripts\harness-eval.ps1`) that writes JSON results per harness and version, using disposable directories and never printing credentials.
- A generated scorecard with three states per cell: verified live, implemented but unverified, unsupported. Never show unverified as passing.
- Playbooks: add a new harness, evaluate a new harness version.

### B. Common improvements most harnesses could support

Ordered by value; see the research document for evidence and the (U) markers.

1. **Structured lifecycle events** (idle, permission, stop) received on a loopback listener, with the screen cue kept as fallback. Injected per launch through flags or environment only; never edit a user's harness settings. Builds on [02](02-reliable-state-and-inbox.md).
2. **Server or protocol interfaces** (`opencode serve`/`acp`, `codex app-server`, the Claude Code SDK; ACP if two or more harnesses confirm it) as an alternative to terminal scraping.
3. **OpenTelemetry usage import**: one local receiver instead of per-harness usage readers, where a harness exports it.
4. **Headless runs** for background jobs such as session titles and summaries.
5. **One auto-approve toggle** mapped to each harness's flag, with the existing visible-interruption rules.
6. **Shared instructions and MCP list**: one source written to each harness's expected file, only on explicit user action.
7. **Fork a conversation** where the harness supports it.
8. **Picker-style model switch.** Harnesses without `/model <id>` (OpenCode) use the manifest `model_picker` command plus a search entry. A harness-provided way to select a model by ID would be more reliable.

## Out of scope

- Claiming support for a harness or version that was not checked live.
- Silent edits to user harness configuration; silent installs or upgrades.

## Acceptance criteria

- The scorecard is generated from the recorded results, not hand-edited, and states what was not verified.
- Evaluating a new version needs one command and disposable state only.
- Any adopted event or protocol source keeps screen reading as a fallback and stays local and authenticated.

## Dependencies

- [02](02-reliable-state-and-inbox.md) (cue schema), [10](10-declarative-adapters.md) (manifests), [`23-adding-a-harness.md`](../../v2/23-adding-a-harness.md).

## Risks

- Documentation drifts quickly; gate on the detected version and keep tested ranges in the manifest.
- Copilot CLI hook and ACP support is unverified; screen reading may stay necessary.
- Hook commands run in the harness's shell on Windows; prefer HTTP handlers.
- No harness is confirmed to honor `COLORFGBG`.
- Live probes spend real model calls.

## Open questions

- Where results are stored (repository, or local only).
- Which harnesses are required to pass before a release.

## Docs to update when implemented

`23-adding-a-harness.md`, `21-engineering-handoff.md`, `17-contributor-guide.md`, README.
