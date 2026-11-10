import { useEffect, useState } from "react";
import { app, Conversation, Session } from "./bridge";
import { Icon } from "./icons";
import { conversationDays, filterConversations } from "./conversations";
import { formatTokens } from "./usage";

export function ConversationsDrawer({ session, busy, onResume, onClose, onError }: {
  session: Session;
  busy: boolean;
  onResume: (session: Session, harnessSessionId: string) => void;
  onClose: () => void;
  onError: (error: unknown) => void;
}) {
  const [list, setList] = useState<Conversation[] | null>(null);
  const [query, setQuery] = useState("");
  const [pending, setPending] = useState<Conversation | null>(null);
  const [note, setNote] = useState("");
  useEffect(() => {
    let cancelled = false;
    setList(null);
    app().ListConversations(session.id).then((items) => { if (!cancelled) setList(items ?? []); }).catch((error) => { if (!cancelled) { setList([]); onError(error); } });
    return () => { cancelled = true; };
  }, [session.id, session.runningHere, onError]);
  const copyId = (id: string) => window.runtime.ClipboardSetText(id).then(() => setNote("Copied the conversation ID.")).catch(onError);
  const shown = filterConversations(list ?? [], query);
  return <aside className="file-drawer conversations-drawer" aria-label="Conversation history">
    <div className="file-drawer-head">
      <strong>History</strong>
    </div>
    <input className="conversation-search" type="search" placeholder="Filter by title, ID or model" value={query} onChange={(event) => setQuery(event.target.value)} />
    {session.runningHere && <div className="toolbar-popover-note">Stop the session to resume a different conversation.</div>}
    <div className="file-tree">
      {list === null ? <div className="toolbar-popover-note">Loading...</div> : shown.length === 0 ? <div className="toolbar-popover-note">{list.length ? "No match." : "No conversations recorded yet."}</div> : shown.map((item) => <div className="conversation-row" key={item.harnessSessionId}>
        <div className="conversation-title" title={item.title || item.harnessSessionId}>{item.title || "(untitled)"}{item.current && <span className="conversation-badge">latest</span>}</div>
        <div className="conversation-meta">{conversationDays(item)}{item.model ? ` | ${item.model}` : ""}{item.tokens > 0 ? ` | ${formatTokens(item.tokens)}` : ""}</div>
        <div className="conversation-id" title={item.harnessSessionId}>{item.harnessSessionId}</div>
        {item.unavailable && <div className="conversation-warn">Cannot resume: {item.unavailable}</div>}
        <div className="conversation-actions">
          <button className="file-act" title="Copy the conversation ID" aria-label="Copy the conversation ID" onClick={() => void copyId(item.harnessSessionId)}><Icon name="copy" size={14} /></button>
          <button className="secondary-button" disabled={busy || session.runningHere || Boolean(item.unavailable)} title={session.runningHere ? "Stop the session first" : "Resume this conversation"} onClick={() => { if (item.current) { onResume(session, item.harnessSessionId); onClose(); } else setPending(item); }}>Resume</button>
        </div>
      </div>)}
    </div>
    {note && <div className="toolbar-popover-note">{note}</div>}
    {pending && <div className="modal-backdrop"><div className="modal confirm-modal">
      <h2>Resume this conversation?</h2>
      <p className="muted">It becomes the latest conversation for <strong>{session.name}</strong>: TauShell will resume it from now on. The current one stays in the history and can be chosen again.</p>
      <div className="modal-actions"><button className="secondary-button" onClick={() => setPending(null)}>Cancel</button><button className="primary-button" disabled={busy} onClick={() => { const chosen = pending; setPending(null); onResume(session, chosen.harnessSessionId); onClose(); }}>Resume</button></div>
    </div></div>}
  </aside>;
}
