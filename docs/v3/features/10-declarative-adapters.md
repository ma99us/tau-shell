# 10 - Declarative Harness Adapters

Status: **future planning**. Rank 10. Sources: V3 ideas (plugin SDK); Orca ("any CLI agent"). User decision: full manifest including resume.

## Problem

TauShell has five adapters written in Go (Claude Code, Copilot CLI, GSD, Codex, OpenCode).
Adding another harness means writing Go, rebuilding and releasing
([`23-adding-a-harness.md`](../../v2/23-adding-a-harness.md)). The AI CLI ecosystem moves
fast, and Orca lists more than 30 agents.

## User stories

- As a user, I want to add a new CLI harness with a file, not a release, so that I can use
  new tools soon after they appear.
- As a user, I want the same state cues, widgets and restore behaviour for manifest
  harnesses as for built-in ones, as far as the harness supports them.
- As a contributor, I want a clear manifest schema and validation messages.

## In scope

- A manifest describing: executable and version/help probes; tested version range;
  capability probes; default and managed launch arguments; session ID creation and
  discovery; **resume** command and how a resume is checked; graceful exit steps; cue
  patterns or signals ([02](02-reliable-state-and-inbox.md)); widgets (commands, models,
  interrupt keys); log locations for usage, model and history.
- Strict validation with errors that name the field.
- Built-in adapters keep working; they may be re-expressed as manifests later.
- Honest tiers: a harness with no resume declaration is launch-only.

## Out of scope

- Arbitrary code or scripts in manifests; a general plugin SDK or marketplace.
- Provider-specific billing prices (kept in the usage ledger).
- Auto-installing manifests from the internet.

## Acceptance criteria

- A user can add a harness without rebuilding TauShell, and it appears in the New Session
  dialog with detection, version support and capability warnings.
- Managed session or resume arguments cannot be overridden by user arguments, as today.
- Resume is reported successful only through the declared check ([finding 2.7](../02-cross-cutting.md#27-restore-truthfulness)); a manifest without a check shows resume as unverified.
- Invalid manifests are rejected with a named-field error and never partly loaded.
- Manifests cannot execute anything beyond the declared harness and probes.
- Arguments remain discrete argv values, not shell strings.

## Dependencies

- [02](02-reliable-state-and-inbox.md) for the cue schema; helps [07](07-conversation-history.md).
- Start gate; [finding 2.5](../02-cross-cutting.md#25-undocumented-harness-formats).

## Risks

- A manifest language that grows into a programming language.
- Users trusting a community manifest that points at an unexpected executable.
- Resume and ID discovery vary too much to express declaratively for some harnesses.

## Spikes

- Express the five existing adapters as manifests and note what does not fit.
- Where manifests live and how they are trusted (for example per-user folder only).

## Open questions

- Ship as YAML consistent with session YAML?
- Should manifests be versioned with a schema number?
- How are cue patterns tested without a live provider?

## Docs to update when implemented

`05-plugin-system.md`, `23-adding-a-harness.md`, `18-yaml-schema.md`, README.
