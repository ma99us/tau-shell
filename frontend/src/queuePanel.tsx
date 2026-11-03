import { useState, useSyncExternalStore } from "react";
import type { ResolvedPreset, Session } from "./bridge";
import { hasCueSignal } from "./attention";
import { clearQueue, enqueuePrompt, getQueue, queuesVersion, removeQueued, resumeQueue, subscribeQueues } from "./promptQueue";

export function useQueueLength(sessionId: string): number {
  useSyncExternalStore(subscribeQueues, queuesVersion);
  return getQueue(sessionId).items.length;
}

const sameFolder = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

// Why a session cannot take part in a broadcast, or "".
function targetRefusal(target: Session): string {
  if (!hasCueSignal(target.tool)) return "no reliable idle signal for this harness";
  if (!target.runningHere) return "not running";
  if (target.resumeNote) return "has a resume warning";
  return "";
}

export function QueuePanel({ session, peers, presets }: { session: Session; peers: Session[]; presets: ResolvedPreset[] | null }) {
  useSyncExternalStore(subscribeQueues, queuesVersion);
  const queue = getQueue(session.id);
  const [text, setText] = useState("");
  const [targets, setTargets] = useState<string[]>([]);
  const others = peers.filter((peer) => peer.id !== session.id);
  const chosen = [session, ...others.filter((peer) => targets.includes(peer.id))];
  const shared = chosen.length > 1 && chosen.some((a, index) => chosen.some((b, other) => other > index && sameFolder(a.workingDir, b.workingDir)));
  const add = (body: string) => {
    const prompt = body.trim();
    if (!prompt) return;
    chosen.forEach((target) => enqueuePrompt(target.id, prompt));
    setText("");
  };
  return <div className="queue-panel">
    <div className="toolbar-popover-note">Queued prompts are typed one at a time when the harness is confirmed idle. They are not saved: closing TauShell discards them.</div>
    {queue.items.map((item, index) => <div className="queue-item" key={item.id}>
      <span className="queue-position">{index + 1}</span><span className="queue-text" title={item.text}>{item.text}</span>
      <button className="queue-remove" aria-label="Remove from queue" title="Remove" onClick={() => removeQueued(session.id, item.id)}>x</button>
    </div>)}
    {queue.paused && <div className="toolbar-popover-note error">Paused: {queue.paused}. <button className="queue-link" onClick={() => resumeQueue(session.id)}>Resume queue</button></div>}
    {queue.items.length > 0 && <button className="queue-link" onClick={() => clearQueue(session.id)}>Clear queue</button>}
    <hr />
    <form className="toolbar-popover-form" onSubmit={(event) => { event.preventDefault(); add(text); }}>
      <input value={text} onChange={(event) => setText(event.target.value)} placeholder="Prompt to queue" aria-label="Prompt to queue" />
      <button type="submit" disabled={!text.trim()}>Add</button>
    </form>
    {presets && presets.length > 0 && <div className="queue-presets">{presets.map((preset) => <button key={preset.name} title={preset.text} onClick={() => add(preset.text)}>{preset.name}</button>)}</div>}
    {others.length > 0 && <>
      <hr />
      <div className="toolbar-popover-note">Also queue for:</div>
      {others.map((peer) => {
        const refusal = targetRefusal(peer);
        return <label className={`queue-target ${refusal ? "disabled" : ""}`} key={peer.id} title={refusal ? `Skipped: ${refusal}` : peer.workingDir}>
          <input type="checkbox" disabled={Boolean(refusal)} checked={targets.includes(peer.id)}
            onChange={(event) => setTargets((current) => event.target.checked ? [...current, peer.id] : current.filter((id) => id !== peer.id))} />
          {peer.name}{refusal && <small>{refusal}</small>}
        </label>;
      })}
      {shared && <div className="toolbar-popover-note error">Some selected sessions share a working directory; parallel agents may overwrite each other's edits.</div>}
    </>}
  </div>;
}
