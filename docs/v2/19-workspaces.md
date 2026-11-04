# Workspaces

Status: **future planning only; not a Phase 1 requirement**. Phase 1 keeps one
flat, ordered session list. See the
[V3 workspace spec](../v3/features/09-lightweight-workspaces.md).

A workspace groups sessions.

## Implemented behavior (code state, not an approved requirement)

The desktop already groups sessions automatically; the status line above still
says future planning only, so the scope question is open (see the sync report).
What the code does, with frontend unit tests (`frontend/test/workspaces.test.ts`)
and no live-app verification:

- The host maps each session to a group (`App.GetWorkspaces`, `internal/workspaces`).
  Sessions without a group fall into "Sessions".
- Groups keep the shared session order: a group sits where its first session
  appears, and grouping is only a view over the flat order.
- Home has a "Group by workspace" switch, off by default and saved in WebView
  local storage (`taushell.home-grouped.v1`). Off lists every session in one flat
  grid. The sidebar always groups, and shows only sessions that have an open tab.
- Dragging within a group moves one session; dropping onto a session of another
  group moves the whole group (`moveGroup`).

Example:
- Claude Architect
- Claude Implementer
- Copilot Reviewer
- Gemini Researcher
