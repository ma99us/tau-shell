# 09 - Lightweight Workspaces (Automatic Grouping)

Status: **future planning**. Rank 9. Sources: V3 ideas #1 and #9. User decision: lightweight, automatic by repo or folder.

## Problem

Home, the sidebar and the tabs show one flat ordered list. With many sessions across several
projects, it is hard to see "everything for project A" or to start, stop or resume just
that group. Today the only group action is Resume All (every stopped session).

## User stories

- As a user, I want sessions of the same project grouped, so that I find them at a glance.
- As a user, I want to resume or stop one project's sessions together, so that I work on one
  project at a time.
- As a user, I want to collapse groups I am not using.

## In scope

- Automatic grouping by repository root (or working folder when not a repository). No new
  YAML required.
- Group headers in Home and the sidebar: name, count, state summary (for example running
  and needing attention).
- Group actions: resume group, stop group (with confirmation), collapse and expand.
- Keeping the single shared order stored in `session_order`; grouping is a view on top.

## Out of scope

- Manually defined workspaces, nested groups, group-level environment or settings.
- Cross-project templates or "workspace YAML" (V3 ideas example; possible later).
- Moving sessions between groups by hand.

## Acceptance criteria

- Sessions in the same repository root appear under one header with a correct count.
- Reordering still works and Home, sidebar and tabs stay consistent.
- "Resume group" follows the same rules as Resume All: skip sessions without a resume ID or
  with a warning, report each failure.
- "Stop group" asks for confirmation and reports forced stops visibly.
- A session with a missing or deleted folder lands in a clear fallback group.
- Collapsed state persists across restarts.

## Dependencies

- None blocking. Attention summaries come from [02](02-reliable-state-and-inbox.md).

## Risks

- Several worktrees or clones of one repo may group together or apart unexpectedly.
- Grouping by path may surprise users who think in "tasks" rather than repos.
- Interaction with drag reorder across group boundaries.

## Spikes

- How to detect the repository root cheaply and reliably (including worktrees and submodules).

## Open questions

- Should a user be able to override the automatic group name?
- Do tabs also group, or only Home and the sidebar?

## Docs to update when implemented

`03-session-lifecycle.md`, `08-UI-Design.md`, README, `12-storage-layout.md`.
