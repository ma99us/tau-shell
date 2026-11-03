import { Icon } from "./icons";
import { Workspace, summarize, summaryText } from "./workspaces";
import { Session } from "./bridge";

export type GroupControls = {
  collapsed: Set<string>;
  busy: boolean;
  needsAttention: (session: Session) => boolean;
  onToggle: (key: string) => void;
  onResume: (workspace: Workspace) => void;
  onStop: (workspace: Workspace) => void;
};

export function GroupHeader({ workspace, controls, variant }: { workspace: Workspace; controls: GroupControls; variant: "tree" | "home" }) {
  const { group } = workspace;
  const open = !controls.collapsed.has(group.key);
  const summary = summarize(workspace.sessions, controls.needsAttention);
  const text = summaryText(summary);
  const title = group.path ? `${group.name} - ${group.path}\n${text}` : `${group.name}\n${text}`;
  return <div className={`group-header group-${variant}`}>
    <button className="group-toggle" aria-expanded={open} title={title} onClick={() => controls.onToggle(group.key)}>
      <span className={`group-chevron ${open ? "open" : ""}`}><Icon name="chevron" size={12} /></span>
      <Icon name={open ? "folderOpen" : "folder"} size={14} />
      <span className="group-name">{group.name}</span>
      {summary.running > 0 && <span className="group-dot" aria-label={`${summary.running} running`} />}
      {summary.needsAttention > 0 && <span className="group-dot attention" aria-label={`${summary.needsAttention} need attention`} />}
      <span className="group-count">{variant === "home" ? text : summary.total}</span>
    </button>
    <span className="group-actions">
      <button className="group-action" disabled={controls.busy || summary.resumable.length === 0} aria-label={`Resume group ${group.name}`} title={summary.resumable.length ? `Resume ${summary.resumable.length} stopped session(s) with a saved conversation` : "Nothing to resume in this group"} onClick={() => controls.onResume(workspace)}><Icon name="play" size={12} /></button>
      <button className="group-action stop" disabled={controls.busy || summary.stoppable.length === 0} aria-label={`Stop group ${group.name}`} title={summary.stoppable.length ? `Stop ${summary.stoppable.length} running session(s)` : "No running sessions in this group"} onClick={() => controls.onStop(workspace)}><Icon name="stop" size={12} /></button>
    </span>
  </div>;
}
