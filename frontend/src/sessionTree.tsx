import { MouseEvent } from "react";
import { Session } from "./bridge";
import { Activity } from "./attention";
import { harnessName } from "./harnesses";
import { ProjectIcon, sessionColorStyle, StateIcon } from "./sessionVisuals";
import { Workspace } from "./workspaces";
import { GroupControls, GroupHeader } from "./workspaceView";

// The sidebar list: workspace groups holding only the sessions that have an open tab.
export function SessionTree({ workspaces, openIds, selected, collapsed, activity, groupControls, dragId, dragProps, dropClass, onOpen, onMenu, onDelete }: {
  workspaces: Workspace[];
  openIds: Set<string>;
  selected: string;
  collapsed: boolean;
  activity: Record<string, Activity>;
  groupControls: GroupControls;
  dragId: string;
  dragProps: (id: string) => object;
  dropClass: (id: string) => string;
  onOpen: (session: Session) => void;
  onMenu: (event: MouseEvent, session: Session) => void;
  onDelete: (session: Session) => void;
}) {
  const visible = workspaces
    .map((workspace) => ({ ...workspace, sessions: workspace.sessions.filter((session) => openIds.has(session.id)) }))
    .filter((workspace) => workspace.sessions.length > 0);
  return <>{visible.map((workspace) => {
    const open = collapsed || !groupControls.collapsed.has(workspace.group.key);
    return <div className="workspace-group" key={workspace.group.key}>
      {!collapsed && <GroupHeader workspace={workspace} controls={groupControls} variant="tree" />}
      {open && <div className={collapsed ? "" : "group-children"}>{workspace.sessions.map((session) => (
        <div className={`session-item ${selected === session.id ? "active" : ""} ${dragId === session.id ? "dragging" : ""} ${dropClass(session.id)}`} key={session.id} {...dragProps(session.id)} style={sessionColorStyle(session.color)} data-colored="" onContextMenu={(event) => onMenu(event, session)}>
          <button className={`session-row ${selected === session.id ? "active" : ""}`} title={session.name} onClick={() => onOpen(session)}>
            <StateIcon session={session} activity={activity[session.id]} />
            <span className="collapsed-only"><ProjectIcon session={session} /></span>
            <span className="session-row-text"><strong>{session.name}</strong><small>{harnessName(session.tool)}</small></span>
          </button>
          <button className="delete-button" aria-label={`Delete ${session.name}`} title="Delete session" onClick={() => onDelete(session)}>x</button>
        </div>
      ))}</div>}
    </div>;
  })}</>;
}
