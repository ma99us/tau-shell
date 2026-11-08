import { useEffect, useState } from "react";
import { app, SecretInfo } from "./bridge";
import { HomePanel, PanelFeedbackState } from "./homePanel";

// Home pane for the secret vault. Values go in and never come back out.
export function SecretsPane({ onError, feedback, onFeedback, onDismissFeedback }: {
  onError: (source: string, error: unknown) => void;
  feedback?: PanelFeedbackState;
  onFeedback?: (feedback?: PanelFeedbackState) => void;
  onDismissFeedback?: () => void;
}) {
  const [secrets, setSecrets] = useState<SecretInfo[] | null>(null);
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [removing, setRemoving] = useState<SecretInfo | null>(null);

  const load = () => app().ListSecrets().then(setSecrets).catch((err) => { setSecrets([]); onError("ui.secrets-list", err); });
  useEffect(() => { void load(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, []);

  const run = (source: string, action: () => Promise<void>, progress: string, success: string, done?: () => void) => {
    setBusy(true);
    onFeedback?.({ tone: "progress", message: progress });
    action().then(() => { done?.(); onFeedback?.({ tone: "success", message: success }); })
      .catch((err) => onError(source, err)).finally(() => { setBusy(false); void load(); });
  };
  const save = (event: React.FormEvent) => {
    event.preventDefault();
    run("ui.secret-save", () => app().SetSecret(name.trim(), value), "Saving secret...", "Secret saved.", () => { setName(""); setValue(""); });
  };

  return <HomePanel className="secrets-pane" title="Secrets" description={<>Stored in Windows Credential Manager. Reference one in a session with <code>secrets: {"{ENV_VAR: secret-name}"}</code>; its value reaches only that session's environment.</>}
    feedback={feedback} onDismissFeedback={onDismissFeedback}>
    {secrets && secrets.length > 0 && <ul className="secret-list">{secrets.map((secret) => <li key={secret.name}>
      <code>{secret.name}</code>
      <span className={`muted ${secret.missing ? "warn" : ""}`}>{secret.missing ? "not stored" : "stored"}{secret.usedBy.length ? `; used by ${secret.usedBy.join(", ")}` : "; not used"}</span>
      {!secret.missing && <button className="secondary-button" disabled={busy} onClick={() => setRemoving(secret)}>Delete</button>}
    </li>)}</ul>}
    <form className="secret-form" onSubmit={save}>
      <input placeholder="Secret name" aria-label="Secret name" value={name} onChange={(event) => setName(event.target.value)} autoComplete="off" />
      <input type="password" placeholder="Value (never shown again)" aria-label="Secret value" value={value} onChange={(event) => setValue(event.target.value)} autoComplete="off" />
      <button className="secondary-button" type="submit" disabled={busy || !name.trim() || !value}>Save</button>
    </form>
    {removing && <div className="modal-backdrop"><div className="modal confirm-modal">
      <h2>Delete secret {removing.name}?</h2>
      <p className="muted">{removing.usedBy.length ? `These sessions use it and will not start until it is added again: ${removing.usedBy.join(", ")}.` : "No session uses it."}</p>
      <div className="modal-actions"><button className="secondary-button" onClick={() => setRemoving(null)}>Cancel</button><button className="primary-button" disabled={busy} onClick={() => { const target = removing; setRemoving(null); run("ui.secret-delete", () => app().DeleteSecret(target.name), "Deleting secret...", "Secret deleted."); }}>Delete</button></div>
    </div></div>}
  </HomePanel>;
}
