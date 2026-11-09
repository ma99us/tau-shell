import { useState } from "react";
import { Session } from "./bridge";
import { linkProblem, linkSegments, maxNotesLength, notesProblem, parseTags } from "./sessionNotes";

type OpenLink = (url: string) => void;

// Text with http(s) URLs turned into buttons that open through the host, which
// only opens URLs found in the session's own notes and tags.
export function LinkedText({ text, onOpen }: { text: string; onOpen: OpenLink }) {
  return <>{linkSegments(text).map((segment, index) => segment.url
    ? <a key={index} className="note-link" href={segment.url} title={segment.url} onClick={(event) => { event.preventDefault(); event.stopPropagation(); onOpen(segment.url!); }}>{segment.text}</a>
    : <span key={index}>{segment.text}</span>)}</>;
}

export function SessionNotes({ session, onOpen }: { session: Session; onOpen: OpenLink }) {
  const tags = session.tags ?? [];
  if (!session.notes && !session.link && tags.length === 0) return null;
  return <div className="session-notes">
    {session.link && <p className="session-link"><a className="note-link" href={session.link} title={session.link} onClick={(event) => { event.preventDefault(); onOpen(session.link!); }}>{session.linkLabel || session.link}</a></p>}
    {session.notes && <p className="note-text"><LinkedText text={session.notes} onOpen={onOpen} /></p>}
    {tags.length > 0 && <div className="tag-chips">{tags.map((tag) => <span className="tag-chip" key={tag} title={tag}><LinkedText text={tag} onOpen={onOpen} /></span>)}</div>}
  </div>;
}

export type NotesEdit = { link: string; notes: string; tags: string[] };

export function NotesDialog({ session, onClose, onSave }: { session: Session; onClose: () => void; onSave: (edit: NotesEdit) => void }) {
  const [link, setLink] = useState(session.link ?? "");
  const [notes, setNotes] = useState(session.notes ?? "");
  const [tagText, setTagText] = useState((session.tags ?? []).join(" "));
  const tags = parseTags(tagText);
  const trimmedLink = link.trim();
  const problem = linkProblem(trimmedLink) || notesProblem(notes, tags);
  const changed = trimmedLink !== (session.link ?? "") || notes.trim() !== (session.notes ?? "") || tags.join("\n") !== (session.tags ?? []).join("\n");
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><div className="modal confirm-modal notes-modal">
    <div className="modal-heading"><div><p className="eyebrow">LINK, NOTES AND TAGS</p><h2>{session.name}</h2></div></div>
    <form onSubmit={(event) => { event.preventDefault(); if (!problem && changed) onSave({ link: trimmedLink, notes: notes.trim(), tags }); }}>
      <label>Jira issue or PR link<input autoFocus value={link} placeholder="https://github.com/org/repo/pull/123" onChange={(event) => setLink(event.target.value)} /></label>
      <label>Notes<textarea rows={5} value={notes} placeholder="Why this session exists, what is next, links..." onChange={(event) => setNotes(event.target.value)} /></label>
      <p className="muted notes-hint">{[...notes].length} / {maxNotesLength}. Web links (http or https) become clickable.</p>
      <label>Tags<input value={tagText} placeholder="review VTA-123 https://..." onChange={(event) => setTagText(event.target.value)} /></label>
      <p className="muted notes-hint">Separate tags with spaces or commas. A tag can be a link.</p>
      {tags.length > 0 && <div className="tag-chips">{tags.map((tag) => <span className="tag-chip" key={tag}>{tag}</span>)}</div>}
      {problem && <p className="card-error" role="alert">{problem}</p>}
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={!!problem || !changed}>Save</button></div>
    </form>
  </div></div>;
}
