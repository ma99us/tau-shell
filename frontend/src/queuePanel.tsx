import { useState, useSyncExternalStore, type KeyboardEvent } from "react";
import type { ResolvedPreset, Session } from "./bridge";
import { clearQueue, enqueuePrompt, getQueue, queuesVersion, removeQueued, resumeQueue, sendNextNow, subscribeQueues, updateQueued } from "./promptQueue";
import { Icon } from "./icons";

// Enter submits, Shift+Enter keeps the newline; IME composition is left alone.
function submitOnEnter(event: KeyboardEvent<HTMLTextAreaElement>, submit: () => void) {
  if (event.key !== "Enter" || event.shiftKey || event.nativeEvent.isComposing) return;
  event.preventDefault();
  submit();
}

export function useQueueLength(sessionId: string): number {
  useSyncExternalStore(subscribeQueues, queuesVersion);
  return getQueue(sessionId).items.length;
}

export function QueuePanel({ session, presets }: { session: Session; presets: ResolvedPreset[] | null }) {
  useSyncExternalStore(subscribeQueues, queuesVersion);
  const queue = getQueue(session.id);
  const [text, setText] = useState("");
  const [editing, setEditing] = useState<number | null>(null);
  const [sendNote, setSendNote] = useState("");
  const [draft, setDraft] = useState("");
  const copy = (value: string) => { window.runtime.ClipboardSetText(value).catch((error) => console.error("TauShell could not copy the queued prompt.", error)); };
  const saveEdit = (itemId: number) => {
    const prompt = draft.trim();
    if (prompt) updateQueued(session.id, itemId, prompt);
    setEditing(null);
  };
  const add = (body: string) => {
    const prompt = body.trim();
    if (!prompt) return;
    enqueuePrompt(session.id, prompt);
    setText("");
  };
  return <div className="queue-panel">
    <div className="toolbar-popover-note">Queued prompts are typed one at a time when the harness is confirmed idle. They are saved on this computer and kept across restarts, but after a restart the queue stays paused until you press Send.</div>
    {queue.items.map((item, index) => editing === item.id ? <form className="queue-edit" key={item.id} onSubmit={(event) => { event.preventDefault(); saveEdit(item.id); }}>
      <textarea rows={3} autoFocus value={draft} onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => submitOnEnter(event, () => saveEdit(item.id))} aria-label="Edit queued prompt" />
      <div className="queue-edit-actions"><button type="submit" disabled={!draft.trim()}>Save</button><button type="button" onClick={() => setEditing(null)}>Cancel</button></div>
    </form> : <div className="queue-item" key={item.id}>
      <span className="queue-position">{index + 1}</span><span className="queue-text" title={item.text}>{item.text}</span>
      <button className="queue-remove" aria-label="Copy prompt" title="Copy" onClick={() => copy(item.text)}><Icon name="copy" size={13} /></button>
      <button className="queue-remove" aria-label="Edit prompt" title="Edit" onClick={() => { setEditing(item.id); setDraft(item.text); }}><Icon name="edit" size={13} /></button>
      <button className="queue-remove" aria-label="Remove from queue" title="Remove" onClick={() => removeQueued(session.id, item.id)}>x</button>
    </div>)}
    {queue.paused && <div className="toolbar-popover-note error">Paused: {queue.paused}. <button className="queue-link" onClick={() => resumeQueue(session.id)}>Resume queue</button></div>}
    {sendNote && <div className="toolbar-popover-note error">Not sent: {sendNote}.</div>}
    <div className="queue-actions">
      <button className="queue-send" disabled={queue.items.length === 0} title="Send the next prompt now and keep sending the rest as usual" onClick={() => setSendNote(sendNextNow({ id: session.id, tool: session.tool, runningHere: session.runningHere }, Date.now()))}>Send</button>
      {queue.items.length > 0 && <button className="queue-link" onClick={() => clearQueue(session.id)}>Clear queue</button>}
    </div>
    <hr />
    <form className="toolbar-popover-form queue-prompt-form" onSubmit={(event) => { event.preventDefault(); add(text); }}>
      <textarea rows={3} value={text} onChange={(event) => setText(event.target.value)} onKeyDown={(event) => submitOnEnter(event, () => add(text))} placeholder="Prompt to queue (Enter adds, Shift+Enter new line)" aria-label="Prompt to queue" />
      <button type="submit" disabled={!text.trim()}>Add</button>
    </form>
    {presets && presets.length > 0 && <div className="queue-presets">{presets.map((preset) => <button key={preset.name} title={preset.text} onClick={() => add(preset.text)}>{preset.name}</button>)}</div>}
  </div>;
}
