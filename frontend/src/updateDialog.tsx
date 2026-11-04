import { useEffect, useState } from "react";
import { app, AppUpdate } from "./bridge";
import { parseMarkdown } from "./markdown";
import { renderBlock } from "./fileDrawer";
import { Progress, progressText } from "./updateProgress";


// Confirm-install dialog for a newer release: shows the notes, then downloads,
// verifies, stops sessions (asking first) and restarts into the new version.
export function UpdateDialog({ update, onClose, onError }: {
  update: AppUpdate;
  onClose: () => void;
  onError: (source: string, error: unknown) => void;
}) {
  const [progress, setProgress] = useState<Progress | null>(null);
  const [failure, setFailure] = useState("");
  const working = progress !== null && progress.phase !== "failed";

  useEffect(() => {
    const runtime = window.runtime;
    runtime?.EventsOnMultiple("app-update-progress", (event: Progress) => setProgress(event), -1);
    return () => runtime?.EventsOff("app-update-progress");
  }, []);

  const install = () => {
    setFailure("");
    setProgress({ phase: "downloading", done: 0, total: -1 });
    app().InstallAppUpdate().catch((err) => {
      setProgress(null);
      setFailure(String(err instanceof Error ? err.message : err));
      onError("ui.install-update", err);
    });
  };

  return (
    <div className="modal-backdrop"><div className="modal update-modal">
      <div className="modal-heading"><div><p className="eyebrow">UPDATE</p><h2>TauShell {update.tag} is available</h2></div></div>
      {update.notes && <div className="md update-notes">{parseMarkdown(update.notes).map(renderBlock)}</div>}
      {!update.canInstall && <p className="muted">{update.reason} You can still download the release manually.</p>}
      {update.canInstall && <p className="muted">TauShell downloads the release from GitHub, checks its SHA-256 checksum, stops running sessions (you are asked first) and restarts. Stopped sessions are resumed afterwards where possible. The previous version is kept for rollback.</p>}
      {working && <p role="status">{progressText(progress)}</p>}
      {failure && <p className="error-text" role="alert">{failure}</p>}
      <div className="modal-actions">
        <button className="secondary-button" disabled={working} onClick={onClose}>Later</button>
        <button className="secondary-button" disabled={working} onClick={() => void app().OpenReleasePage()}>Open release page</button>
        <button className="primary-button" disabled={working || !update.canInstall} onClick={install}>Update and restart</button>
      </div>
    </div></div>
  );
}
