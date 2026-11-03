import { useEffect, useRef, useState } from "react";
import { app, MobileInterface, MobileStatus, PairingOffer } from "./bridge";

const pollMs = 1000;

// The Home pane for the phone companion: an on/off switch, the paired phones,
// and the pairing flow. Pairing needs the phone to scan the code AND the user to
// approve the phone's name here. The offer (code, fingerprint) stays in memory
// only while the window is open and is never logged.
export function PhonePane({ onError }: { onError: (source: string, error: unknown) => void }) {
  const [status, setStatus] = useState<MobileStatus | null>(null);
  const [interfaces, setInterfaces] = useState<MobileInterface[]>([]);
  const [offer, setOffer] = useState<PairingOffer | null>(null);
  const [busy, setBusy] = useState(false);
  const offerRef = useRef(offer);
  offerRef.current = offer;

  const refresh = () => app().GetMobileStatus().then((next) => {
    setStatus(next);
    if (next.pairing === "idle" && offerRef.current) setOffer(null);
  }).catch((err) => onError("ui.phone-status", err));

  useEffect(() => {
    void refresh();
    app().GetMobileInterfaces().then(setInterfaces).catch((err) => onError("ui.phone-interfaces", err));
    const timer = window.setInterval(() => void refresh(), pollMs);
    return () => window.clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const run = (source: string, action: () => Promise<unknown>) => {
    setBusy(true);
    action().catch((err) => onError(source, err)).finally(() => { setBusy(false); void refresh(); });
  };

  const enabled = status?.enabled ?? false;
  const awaiting = status?.pairing === "awaiting-approval";

  return <div className="tools-panel phone-pane">
    <div className="tools-header">
      <div><h2>Phone</h2><p className="muted">{enabled ? `On: paired Android phones can reach TauShell on your network at ${status?.address}.` : "Off: no phone can reach TauShell."}       The choice is saved to settings.</p></div>
      <div className="refresh-group">
              <label className="check-option">Address
                <select value={status?.interface ?? ""} disabled={busy || !status}
                  onChange={(event) => run("ui.phone-interface", () => app().SetMobileInterface(event.target.value))}>
                  {!status?.interface && <option value="">Choose this PC's address</option>}
                  {status?.interface && !interfaces.some((item) => item.address === status.interface) && <option value={status.interface}>{status.interface} (not found)</option>}
                  {interfaces.map((item) => <option key={`${item.name}-${item.address}`} value={item.address}>{item.address} ({item.name})</option>)}
                </select>
              </label>
        {busy && <span className="spinner" role="status" aria-label="Updating phone server" />}
        <label className="check-option"><input type="checkbox" role="switch" checked={enabled} disabled={busy || !status}
          onChange={(event) => run("ui.phone-toggle", () => app().SetMobileEnabled(event.target.checked))} />{enabled ? "On" : "Off"}</label>
      </div>
    </div>
    {enabled && <>
      <ul className="phone-list">
        {status?.devices.length === 0 && <li className="muted">No paired phones.</li>}
        {status?.devices.map((device) => <li key={device.id}>
          <span>{device.name}</span> <span className="muted">paired {new Date(device.created).toLocaleDateString()}{device.lastSeen ? `, last seen ${new Date(device.lastSeen).toLocaleTimeString()}` : ""}, expires {new Date(device.expires).toLocaleDateString()}</span>
          <button className="secondary-button" disabled={busy} onClick={() => run("ui.phone-revoke", () => app().RevokePhone(device.id))}>Revoke</button>
        </li>)}
      </ul>
      {awaiting && <div className="phone-approve" role="alertdialog" aria-label="Approve phone">
        <p>A phone named <strong>{status?.pendingName}</strong> scanned the code. Approve only if that is your phone.</p>
        <button className="primary-button" disabled={busy} onClick={() => run("ui.phone-approve", () => app().DecidePairing(true))}>Approve</button>
        <button className="secondary-button" disabled={busy} onClick={() => run("ui.phone-deny", () => app().DecidePairing(false))}>Deny</button>
      </div>}
      {offer && !awaiting && <div className="phone-offer">
        <img src={offer.qr} alt="Pairing QR code" width={192} height={192} />
        <div>
          <p>Scan this code with the TauShell app. It works once and expires at {new Date(offer.expiresAt).toLocaleTimeString()}.</p>
          <button className="secondary-button" onClick={() => run("ui.phone-cancel", async () => { await app().CancelPairing(); setOffer(null); })}>Cancel</button>
        </div>
      </div>}
      {!offer && !awaiting && <button className="primary-button" disabled={busy}
        onClick={() => run("ui.phone-pair", async () => setOffer(await app().StartPairing()))}>Add phone</button>}
      {status && status.audit.length > 0 && <ul className="control-audit-list">{[...status.audit].reverse().slice(0, 20).map((entry, index) => <li key={`${entry.time}-${index}`}>
        <span className="muted">{new Date(entry.time).toLocaleTimeString()}</span> {entry.image} {entry.op} <span className={entry.outcome === "ok" ? "" : "warn"}>{entry.outcome}</span>
      </li>)}</ul>}
    </>}
  </div>;
}
