import { useState, useSyncExternalStore } from "react";
import type { ResolvedPreset, Session } from "./bridge";
import { clearQueue, enqueuePrompt, getQueue, queuesVersion, removeQueued, resumeQueue, subscribeQueues } from "./promptQueue";

export function useQueueLength(sessionId: string): number {
  useSyncExternalStore(subscribeQueues, queuesVersion);
  return getQueue(sessionId).items.length;
}

export function QueuePanel({ session, presets }: { session: Session; presets: ResolvedPreset[] | null }) {
  useSyncExternalStore(subscribeQueues, queuesVersion);
  const queue = getQueue(session.id);
  const [text, setText] = useState("");
  const add = (body: string) => {
    const prompt = body.trim();
    if (!prompt) return;
    enqueuePrompt(session.id, prompt);
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
    <form className="toolbar-popover-form queue-prompt-form" onSubmit={(event) => { event.preventDefault(); add(text); }}>
      <textarea rows={3} value={text} onChange={(event) => setText(event.target.value)} placeholder="Prompt to queue" aria-label="Prompt to queue" />
      <button type="submit" disabled={!text.trim()}>Add</button>
    </form>
    {presets && presets.length > 0 && <div className="queue-presets">{presets.map((preset) => <button key={preset.name} title={preset.text} onClick={() => add(preset.text)}>{preset.name}</button>)}</div>}
  </div>;
}
