import { useEffect, useState } from "react";
import { DialogRequest, Session, ShutdownProgress } from "./bridge";
import appIcon from "./assets/appicon.png";

const dialogLabels: Record<DialogRequest["kind"], string> = { question: "CONFIRM", warning: "WARNING", error: "PROBLEM", info: "NOTICE" };

export function AppDialog({ request, onAnswer }: { request: DialogRequest; onAnswer: (choice: string) => void }) {
  // Escape is the dialog's cancel button; a single-button dialog just closes with it.
  const cancel = request.cancelButton || (request.buttons.length === 1 ? request.buttons[0] : "");
  useEffect(() => {
    if (!cancel) return;
    const onKey = (event: globalThis.KeyboardEvent) => { if (event.key === "Escape") onAnswer(cancel); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [request.id, cancel]);
  const initial = request.buttons.includes(request.defaultButton) ? request.defaultButton : request.buttons[0];
  return <div className="modal-backdrop" role="alertdialog" aria-modal="true" aria-labelledby="app-dialog-title"><div className="modal confirm-modal">
    <div className="modal-heading"><div><p className="eyebrow">{dialogLabels[request.kind] ?? "NOTICE"}</p><h2 id="app-dialog-title">{request.title}</h2></div></div>
    <p className="muted dialog-message">{request.message}</p>
    <div className="modal-actions">{request.buttons.map((button) =>
      <button key={button} autoFocus={button === initial} className={button === initial ? "primary-button" : "secondary-button"} onClick={() => onAnswer(button)}>{button}</button>)}</div>
  </div></div>;
}

const shutdownStateLabel = { stopping: "Stopping...", stopped: "Stopped", "force-stopped": "Force-stopped" } as const;

export function ShutdownPopup({ progress }: { progress: ShutdownProgress }) {
  const total = progress.sessions.length;
  const finished = progress.sessions.filter((session) => session.state !== "stopping").length;
  return <div className="modal-backdrop shutdown-backdrop" role="alertdialog" aria-modal="true" aria-labelledby="shutdown-title"><div className="modal shutdown-modal">
    <div className="shutdown-heading"><img className="shutdown-mark" src={appIcon} alt="" />
      <div><p className="eyebrow">SHUTTING DOWN</p><h2 id="shutdown-title">Closing TauShell</h2></div></div>
    <p className="muted dialog-message">Asking each session to exit cleanly. Anything still running after a short wait is force-stopped.</p>
    <div className="shutdown-bar" role="progressbar" aria-valuemin={0} aria-valuemax={total} aria-valuenow={finished}><div className="shutdown-bar-fill" style={{ width: `${total ? (finished / total) * 100 : 0}%` }} /></div>
    <p className="muted shutdown-count">{finished} of {total} stopped - {progress.elapsedSeconds}s</p>
    <div className="shutdown-list">{progress.sessions.map((session, index) => <div key={`${session.name}-${index}`} className="shutdown-row">
      {session.state === "stopping" ? <span className="spinner" aria-hidden="true" /> : <span className={`status-dot ${session.state === "stopped" ? "online" : "warn"}`} />}
      <strong>{session.name}</strong><span className={`shutdown-state ${session.state === "force-stopped" ? "status-warn-text" : ""}`}>{shutdownStateLabel[session.state]}</span>
    </div>)}</div>
  </div></div>;
}

export function RenameDialog({ session, onClose, onRename }: { session: Session; onClose: () => void; onRename: (name: string) => void }) {
  const [name, setName] = useState(session.name);
  const trimmed = name.trim();
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><div className="modal confirm-modal">
    <div className="modal-heading"><div><p className="eyebrow">RENAME SESSION</p><h2>{session.name}</h2></div></div>
    <form onSubmit={(event) => { event.preventDefault(); if (trimmed) onRename(trimmed); }}>
      <label>Session name<input autoFocus maxLength={80} value={name} onFocus={(event) => event.target.select()} onChange={(event) => setName(event.target.value)} /></label>
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={!trimmed || trimmed === session.name}>Rename</button></div>
    </form>
  </div></div>;
}
