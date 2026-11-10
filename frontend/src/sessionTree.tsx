import { MouseEvent, useEffect, useState } from "react";
import { app, Session } from "./bridge";
import { Activity } from "./attention";
import { harnessName } from "./harnesses";
import { Icon } from "./icons";
import { ProjectIcon, sessionColorStyle, StateIcon } from "./sessionVisuals";
import { Workspace } from "./workspaces";
import { GroupControls, GroupHeader } from "./workspaceView";

type Place = { label: string; worktree: boolean };

// Branch or worktree name per session, refreshed in the background; the Go host owns all git data.
function usePlaces(ids: string[], enabled: boolean): Record<string, Place> {
  const [places, setPlaces] = useState<Record<string, Place>>({});
  const key = ids.join("|");
  useEffect(() => {
    if (!enabled || !key) return;
    let cancelled = false;
    const load = () => {
      for (const id of key.split("|")) {
        app().GetGitStatus(id).then((git) => {
          if (cancelled) return;
          const label = git?.worktree?.name || git?.branch || "";
          setPlaces((current) => current[id]?.label === label && current[id]?.worktree === Boolean(git?.worktree) ? current : { ...current, [id]: { label, worktree: Boolean(git?.worktree) } });
        }, () => undefined);
      }
    };
    load();
    const timer = window.setInterval(load, 20000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [key, enabled]);
  return places;
}

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
  const places = usePlaces(visible.flatMap((workspace) => workspace.sessions.map((session) => session.id)), !collapsed);
  return <>{visible.map((workspace) => {
    const open = collapsed || !groupControls.collapsed.has(workspace.group.key);
    return <div className="workspace-group" key={workspace.group.key}>
      {!collapsed && <GroupHeader workspace={workspace} controls={groupControls} variant="tree" />}
      {open && <div className={collapsed ? "" : "group-children"}>{workspace.sessions.map((session) => (
        <div className={`session-item ${selected === session.id ? "active" : ""} ${dragId === session.id ? "dragging" : ""} ${dropClass(session.id)}`} key={session.id} {...dragProps(session.id)} style={sessionColorStyle(session.color)} data-colored={session.color ? "" : undefined} onContextMenu={(event) => onMenu(event, session)}>
          <button className={`session-row ${selected === session.id ? "active" : ""}`} title={session.name} onClick={() => onOpen(session)}>
            <StateIcon session={session} activity={activity[session.id]} />
            <span className="collapsed-only"><ProjectIcon session={session} /></span>
            <span className="session-row-text"><strong>{session.name}</strong>{places[session.id]?.label ? <small className="session-place"><Icon name={places[session.id].worktree ? "worktree" : "branch"} size={11} />{places[session.id].label}</small> : <small>{harnessName(session.tool)}</small>}</span>
          </button>
          <button className="delete-button" aria-label={`Delete ${session.name}`} title="Delete session" onClick={() => onDelete(session)}>x</button>
        </div>
      ))}</div>}
    </div>;
  })}</>;
}
