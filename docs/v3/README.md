# TauShell V3 (future planning)

> **Status: future planning only.** Nothing here is a current requirement or a commitment.
> Phase 1 scope in [`AGENTS.md`](../../AGENTS.md) and [`docs/v2/`](../v2/) stays authoritative.
> These documents describe *what* and *why*, not technical designs.

V3 grows TauShell from a session manager into a calmer way to supervise many local AI coding
agents: control them from scripts and phones, know reliably when they need you, and
dispatch work to them, while staying PowerShell-first, local-first and not an IDE.

Inputs: the original [V3 ideas summary](../V3_Ideas_Summary%20(out%20of%20scope%20future).md),
the [stablyai/orca](https://github.com/stablyai/orca) project, and a check of every idea
against the current code.

## Decisions

- **Ranking criteria:** user value, then fit with TauShell principles, then risk, then
  effort. Dependencies set the delivery order, not the rank.
- **Start gate:** begin V3 only after remaining Phase 1 verification is closed
  ([cross-cutting, section 1](02-cross-cutting.md#1-start-gate)).
- **Git:** light only (branch, dirty, ahead/behind). No worktrees, no diff review.
- **Remote and mobile:** in scope. MVP is push plus quick reply and action buttons over the
  local Wi-Fi/LAN only, through a TauShell-managed ntfy sidecar (Android first). Internet
  relay, VPN and iOS are later.
- **IDE surfaces:** a simple file tree, Markdown viewer and copy-path. Not an editor or browser.
- **Control API:** opt-in, current Windows user only. Agents inside TauShell may drive other
  sessions only when that is enabled.
- **Prompt dispatch:** queue prompts while a harness works and send each only when the
  previous turn is done and the harness is confirmed idle. Never interrupt a session. No
  time-based schedules.
- **Workspaces:** lightweight, grouped automatically by repo or folder.
- **Adapters:** manifest-based, including resume.
- **Platform:** Windows first; keep seams for macOS/Linux ([portability track](02-cross-cutting.md#3-portability-track-not-ranked)).

## Top 10

| # | Feature | Status today | Depends on |
|---|---|---|---|
| 1 | [Control CLI and local API](features/01-control-cli-and-api.md) | Gap | Start gate |
| 2 | [Reliable agent state and notification inbox](features/02-reliable-state-and-inbox.md) | Partial | none |
| 3 | [Prompt dispatch (queue and broadcast)](features/03-prompt-dispatch.md) | Gap | 2 (auto-send); 1 (optional) |
| 4 | [Mobile companion MVP](features/04-mobile-companion.md) | Gap | 1, 2; LAN spikes |
| 5 | [Project file drawer](features/05-project-file-drawer.md) | Gap | none |
| 6 | [Status insights (quota and git chips)](features/06-status-insights.md) | Partial | none |
| 7 | [Conversation history browser](features/07-conversation-history.md) | Partial | none |
| 8 | [Secret vault](features/08-secret-vault.md) | Gap | Phase 1 env-leak fix |
| 9 | [Lightweight workspaces](features/09-lightweight-workspaces.md) | Partial | none |
| 10 | [Declarative harness adapters](features/10-declarative-adapters.md) | Partial | 2 (cue schema) |

Already implemented and therefore not ranked: fleet restore (Resume All and auto resume),
usage and cost ledger, tool actions, terminal compatibility, skills, updates and clone. See
[the gap analysis](01-research-and-gap-analysis.md#2-status-of-candidate-ideas-checked-against-the-code).

## Suggested delivery order

1. Close the start gate, including the environment command-line leak fix.
2. Features 1 and 2 (foundations), then 3, then 4.
3. Features 5, 6, 7, 8 and 9 can run in parallel at any point after the gate.
4. Feature 10 after 2 defines the cue schema.

## Documents

- [01 Research and gap analysis](01-research-and-gap-analysis.md)
- [02 Cross-cutting rules, findings and spec template](02-cross-cutting.md)
- [03 Parked and rejected ideas](03-parked-and-rejected.md)
- [Feature specs](features/)
- [Implementation plans](implementation/) (ready-to-start work packages; so far feature 04 only)
