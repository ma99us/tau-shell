import { useEffect, useState } from "react";
import { app } from "./bridge";
import { setHarnesses, type HarnessInfo, type HarnessLoadError } from "./harnesses";

// Lists harnesses defined in ~/.tau_shell/harnesses and manifests that failed
// to load. A user manifest cannot launch until the user confirms its exact
// content here; editing the file asks again.
export function UserHarnesses({ onError }: { onError: (source: string, error: unknown) => void }) {
  const [users, setUsers] = useState<HarnessInfo[]>([]);
  const [errors, setErrors] = useState<HarnessLoadError[]>([]);
  const load = () => app().GetHarnesses().then((catalog) => {
    setHarnesses(catalog.harnesses ?? []);
    setUsers((catalog.harnesses ?? []).filter((item) => !item.builtin));
    setErrors(catalog.errors ?? []);
  }).catch((error) => onError("ui.get-harnesses", error));
  useEffect(() => { void load(); }, []);
  if (users.length === 0 && errors.length === 0) return null;
  return <div className="tool-list user-harnesses">
    {users.map((item) => <div className="tool-row" key={item.id}>
      <span className={`status-dot ${item.trusted && item.resolvedPath ? "online" : "warn"}`} />
      <div className="tool-details">
        <div className="tool-heading"><strong>{item.name}</strong><span className="tool-version">user harness</span></div>
        <code className="tool-path" title={item.path}>{item.path}</code>
        {item.resolvedPath ? <code className="tool-path" title={item.resolvedPath}>{item.resolvedPath}</code> : <span className="tool-warning tool-support-note">{item.executable} was not found on PATH</span>}
        {!item.resumeVerified && <span className="tool-warning tool-support-note">resume cannot be verified for this harness</span>}
        {!item.trusted && <div className="tool-actions">
          <span className="tool-warning tool-support-note">Not confirmed: it cannot start sessions until you confirm it.</span>
          <button className="secondary-button tool-action" title={`Allow TauShell to launch ${item.executable} using this manifest`} onClick={() => void app().ConfirmHarness(item.id).then(load).catch((error) => onError("ui.confirm-harness", error))}>Confirm</button>
        </div>}
      </div>
    </div>)}
    {errors.map((failure) => <div className="tool-row" key={failure.path} title={failure.path}>
      <span className="status-dot warn" />
      <div className="tool-details">
        <div className="tool-heading"><strong>Harness file skipped</strong></div>
        <code className="tool-path" title={failure.path}>{failure.path}</code>
        <span className="tool-warning tool-support-note">{failure.error}</span>
      </div>
    </div>)}
  </div>;
}
