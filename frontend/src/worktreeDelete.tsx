import { useEffect, useState } from "react";
import { app, WorktreeDeletePlan } from "./bridge";

export type WorktreeDeleteChoice = { removeFolder: boolean; deleteBranch: boolean; force: boolean; blocked: boolean };

export const noWorktreeChoice: WorktreeDeleteChoice = { removeFolder: false, deleteBranch: false, force: false, blocked: false };

// Delete-dialog section for a session that owns a TauShell-created git worktree.
// Dirty or unpushed work blocks removal until the user forces it.
export function WorktreeDeleteOptions({ sessionId, onChange }: { sessionId: string; onChange: (choice: WorktreeDeleteChoice) => void }) {
  const [plan, setPlan] = useState<WorktreeDeletePlan | null>(null);
  const [error, setError] = useState("");
  const [removeFolder, setRemoveFolder] = useState(false);
  const [deleteBranch, setDeleteBranch] = useState(false);
  const [force, setForce] = useState(false);
  useEffect(() => {
    let cancelled = false;
    app().GetWorktreeDeletePlan(sessionId).then((next) => {
      if (cancelled) return;
      setPlan(next);
      // Default to removing the folder only when nothing would be lost.
      setRemoveFolder(next.managed && next.exists && !next.dirty && !next.sharedBy);
    }, (err) => { if (!cancelled) setError(err instanceof Error ? err.message : String(err)); });
    return () => { cancelled = true; };
  }, [sessionId]);
  const risky = !!plan && (plan.dirty || (deleteBranch && plan.unpushed > 0));
  const blocked = removeFolder && risky && !force;
  useEffect(() => {
    onChange({ removeFolder, deleteBranch: removeFolder && deleteBranch, force: removeFolder && force, blocked });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [removeFolder, deleteBranch, force, blocked]);
  if (error) return <p className="tool-warning">{error}</p>;
  if (!plan || !plan.managed || !plan.exists) return null;
  if (plan.sharedBy) return <p className="muted">The worktree folder is also used by session {plan.sharedBy}, so it will be kept.</p>;
  return <div className="muted">
    <p><strong>Git worktree {plan.name}</strong> ({plan.branch})</p>
    <label className="check-option"><input type="checkbox" checked={removeFolder} onChange={(event) => setRemoveFolder(event.target.checked)} />Also remove the worktree folder</label>
    <label className="check-option"><input type="checkbox" checked={deleteBranch} disabled={!removeFolder} onChange={(event) => setDeleteBranch(event.target.checked)} />Also delete the branch {plan.branch}</label>
    {removeFolder && plan.dirty && <p className="tool-warning">The worktree has uncommitted changes that would be lost.</p>}
    {removeFolder && deleteBranch && plan.unpushed > 0 && <p className="tool-warning">The branch has {plan.unpushed} commit{plan.unpushed === 1 ? "" : "s"} that no remote has.</p>}
    {removeFolder && risky && <label className="check-option"><input type="checkbox" checked={force} onChange={(event) => setForce(event.target.checked)} />Discard that work and remove anyway</label>}
  </div>;
}
