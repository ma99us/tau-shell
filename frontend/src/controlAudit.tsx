import { useEffect, useState } from "react";
import { app, ControlAuditEntry } from "./bridge";
import { addAuditEntry, auditCaller, keepEntries } from "./controlAuditLog";
import { HomePanel, PanelFeedbackState } from "./homePanel";

// The Home pane for the local control channel: an on/off switch and every
// request received. The switch acts now and is not saved; config.yaml decides
// the state at startup.
export function ControlAudit({ onError, feedback, onFeedback, onDismissFeedback }: {
  onError: (source: string, error: unknown) => void;
  feedback?: PanelFeedbackState;
  onFeedback?: (feedback?: PanelFeedbackState) => void;
  onDismissFeedback?: () => void;
}) {
  const [enabled, setEnabled] = useState(false);
  const [saving, setSaving] = useState(false);
  const [entries, setEntries] = useState<ControlAuditEntry[]>([]);

  useEffect(() => {
    let active = true;
    app().GetControlStatus().then((status) => {
      if (!active) return;
      setEnabled(status.enabled);
      setEntries([...(status.audit ?? [])].reverse().slice(0, keepEntries));
    }).catch((err) => onError("ui.control-status", err));
    const runtime = window.runtime;
    runtime?.EventsOnMultiple("control-audit", (entry: ControlAuditEntry) => setEntries((current) => addAuditEntry(current, entry)), -1);
    return () => { active = false; runtime?.EventsOff("control-audit"); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const toggle = (next: boolean) => {
    setSaving(true);
    onFeedback?.({ tone: "progress", message: "Changing control channel..." });
    app().SetControlEnabled(next).then(() => {
      setEnabled(next);
      onFeedback?.({ tone: "success", message: next ? "Control channel enabled." : "Control channel disabled." });
    }).catch((err) => {
      onError("ui.control-toggle", err);
      // The channel may have switched even though saving failed; show the truth.
      app().GetControlStatus().then((status) => setEnabled(status.enabled)).catch(() => undefined);
    }).finally(() => setSaving(false));
  };

  return <HomePanel className="control-audit" title="Control channel" description={<>{enabled ? "On: programs running as you can list sessions with " : "Off: nothing can reach TauShell through "}<code>taushell ctl</code>. The choice is saved to settings.</>}
    actions={<div className="refresh-group">
        {saving && <span className="spinner" role="status" aria-label="Switching control channel" />}
        <label className="check-option"><input type="checkbox" role="switch" checked={enabled} disabled={saving} onChange={(event) => toggle(event.target.checked)} />{enabled ? "On" : "Off"}</label>
      </div>}
    feedback={feedback} onDismissFeedback={onDismissFeedback}>
    {enabled && (entries.length === 0
      ? <p className="muted skill-note">No requests yet.</p>
      : <ul className="control-audit-list">{entries.map((entry, index) => <li key={`${entry.time}-${index}`}>
        <span className="muted">{new Date(entry.time).toLocaleTimeString()}</span> {auditCaller(entry)} {entry.op} <span className={entry.outcome === "ok" ? "" : "warn"}>{entry.outcome}</span>
      </li>)}</ul>)}
  </HomePanel>;
}
