# Workspaces

Status: automatic project grouping is implemented as a view over the shared
ordered session list. Broader workspace management remains future planning,
not a Phase 1 requirement; see the
[V3 workspace spec](../v3/features/09-lightweight-workspaces.md).

A workspace groups sessions.

## Implemented behavior (code state, not an approved requirement)

What the code does, with frontend unit tests (`frontend/test/workspaces.test.ts`):

- The host maps each session to its Git repository root, or its working folder
  outside a repository (`App.GetWorkspaces`, `internal/workspaces`). Missing
  folders have their own group; sessions without a mapping fall into "Sessions".
- Groups keep the shared session order: a group sits where its first session
  appears, and grouping is only a view over the flat order.
- Home has a "Group by workspace" switch, off by default and saved in WebView
  local storage (`taushell.home-grouped.v1`). Off lists every session in one flat
  grid. The sidebar always groups, and shows only sessions that have an open tab.
- Dragging within a group moves one session; dropping onto a session of another
  group moves the whole group (`moveGroup`).
- Group Resume restores stopped sessions with a saved conversation and no
  resume warning. Group Stop asks for confirmation before stopping the
  running sessions in the group.

An isolated-profile desktop check on 2026-11-03 exercised group Resume and
Stop, including confirmation; see the
[engineering handoff](21-engineering-handoff.md#declarative-harness-manifests-implemented).
That check does not establish live coverage of every grouping or drag path.

Example:
- Claude Architect
- Claude Implementer
- Copilot Reviewer
- Gemini Researcher
