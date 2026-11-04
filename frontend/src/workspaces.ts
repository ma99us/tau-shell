import type { Session } from "./bridge";

export type WorkspaceGroup = { key: string; name: string; path: string; repo: boolean };
export type Workspace = { group: WorkspaceGroup; sessions: Session[] };

export const ungrouped: WorkspaceGroup = { key: "ungrouped", name: "Sessions", path: "", repo: false };

// Groups sit in the order their first session appears in the shared session
// order, and sessions keep that order inside a group, so grouping is only a view.
export function groupSessions(sessions: Session[], groups: Record<string, WorkspaceGroup>): Workspace[] {
  const result: Workspace[] = [];
  const byKey = new Map<string, Workspace>();
  for (const session of sessions) {
    const group = groups[session.id] ?? ungrouped;
    let workspace = byKey.get(group.key);
    if (!workspace) {
      workspace = { group, sessions: [] };
      byKey.set(group.key, workspace);
      result.push(workspace);
    }
    workspace.sessions.push(session);
  }
  return result;
}

// Moves the workspace holding `fromId` directly before or after the one holding
// `toId`, keeping each group's session order; returns the flat id order.
export function moveGroup(workspaces: Workspace[], fromId: string, toId: string, after: boolean): string[] {
  const has = (workspace: Workspace, id: string) => workspace.sessions.some((session) => session.id === id);
  const from = workspaces.find((workspace) => has(workspace, fromId));
  const to = workspaces.find((workspace) => has(workspace, toId));
  const flat = workspaces.flatMap((workspace) => workspace.sessions.map((session) => session.id));
  if (!from || !to || from === to) return flat;
  const rest = workspaces.filter((workspace) => workspace !== from);
  const index = rest.indexOf(to) + (after ? 1 : 0);
  return [...rest.slice(0, index), from, ...rest.slice(index)].flatMap((workspace) => workspace.sessions.map((session) => session.id));
}

export const flattenWorkspaces = (workspaces: Workspace[]): Session[] => workspaces.flatMap((workspace) => workspace.sessions);

export type WorkspaceSummary = { total: number; running: number; needsAttention: number; resumable: Session[]; stoppable: Session[] };

export function summarize(sessions: Session[], needsAttention: (session: Session) => boolean): WorkspaceSummary {
  return {
    total: sessions.length,
    running: sessions.filter((session) => session.runningHere).length,
    needsAttention: sessions.filter((session) => session.runningHere && needsAttention(session)).length,
    // Same rule as Resume All: a saved conversation and no warning.
    resumable: sessions.filter((session) => !session.runningHere && session.hasResumeId && !session.resumeNote),
    stoppable: sessions.filter((session) => session.runningHere),
  };
}

export function summaryText(summary: WorkspaceSummary): string {
  const parts = [`${summary.total} session${summary.total === 1 ? "" : "s"}`];
  if (summary.running) parts.push(`${summary.running} running`);
  if (summary.needsAttention) parts.push(`${summary.needsAttention} need${summary.needsAttention === 1 ? "s" : ""} attention`);
  return parts.join(", ");
}

const collapsedKey = "taushell.workspaces-collapsed.v1";

export function readCollapsed(storage: Pick<Storage, "getItem">): Set<string> {
  try {
    const parsed: unknown = JSON.parse(storage.getItem(collapsedKey) ?? "[]");
    return new Set(Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === "string") : []);
  } catch {
    return new Set();
  }
}

export function writeCollapsed(storage: Pick<Storage, "setItem">, collapsed: Set<string>): void {
  storage.setItem(collapsedKey, JSON.stringify([...collapsed]));
}
