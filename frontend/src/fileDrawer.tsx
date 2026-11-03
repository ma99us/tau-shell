import { Fragment, ReactNode, useCallback, useEffect, useState } from "react";
import { app, ProjectEntry, ProjectFile, Session } from "./bridge";
import { Block, Inline, parseMarkdown } from "./markdown";
import { Icon } from "./icons";
import { fileIconFor } from "./fileTypes";

function renderInline(nodes: Inline[]): ReactNode {
  return nodes.map((node, index) => {
    switch (node.kind) {
      case "code": return <code key={index}>{node.text}</code>;
      case "strong": return <strong key={index}>{renderInline(node.children)}</strong>;
      case "em": return <em key={index}>{renderInline(node.children)}</em>;
      // Shown as text with its target; never a live link.
      case "link": return <span className="md-link" key={index} title={node.href}>{node.label}</span>;
      default: return <Fragment key={index}>{node.text}</Fragment>;
    }
  });
}

function renderBlock(block: Block, index: number): ReactNode {
  switch (block.kind) {
    case "heading": return <div className={`md-heading md-h${Math.min(block.level, 3)}`} key={index}>{renderInline(block.inline)}</div>;
    case "code": return <pre className="md-code" key={index}>{block.text}</pre>;
    case "rule": return <hr key={index} />;
    case "quote": return <blockquote key={index}>{renderInline(block.inline)}</blockquote>;
    case "list": {
      const items = block.items.map((item, position) => <li key={position}>{renderInline(item)}</li>);
      return block.ordered ? <ol key={index}>{items}</ol> : <ul key={index}>{items}</ul>;
    }
    default: return <p key={index}>{renderInline(block.inline)}</p>;
  }
}

const isMarkdown = (path: string) => /\.(md|markdown)$/i.test(path);

export function FileDrawer({ session, onClose, onError }: { session: Session; onClose: () => void; onError: (error: unknown) => void }) {
  const [tree, setTree] = useState<Record<string, ProjectEntry[]>>({});
  const [open, setOpen] = useState<string[]>([]);
  const [showAll, setShowAll] = useState(false);
  const [file, setFile] = useState<ProjectFile | null>(null);
  const [note, setNote] = useState("");
  const [menu, setMenu] = useState<{ x: number; y: number; entry: ProjectEntry } | null>(null);
  useEffect(() => {
    if (!menu) return;
    const close = () => setMenu(null);
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") close(); };
    window.addEventListener("pointerdown", close);
    window.addEventListener("blur", close);
    window.addEventListener("keydown", onKey);
    return () => { window.removeEventListener("pointerdown", close); window.removeEventListener("blur", close); window.removeEventListener("keydown", onKey); };
  }, [menu]);
  const load = useCallback((dir: string, all: boolean) => {
    app().ListProjectDir(session.id, dir, all)
      .then((listing) => setTree((current) => ({ ...current, [dir]: listing.entries ?? [] })))
      .catch(onError);
  }, [session.id, onError]);
  useEffect(() => { setTree({}); setOpen([]); setFile(null); load("", showAll); }, [session.id, showAll, load]);
  const toggle = (entry: ProjectEntry) => {
    if (open.includes(entry.path)) { setOpen(open.filter((path) => path !== entry.path)); return; }
    setOpen([...open, entry.path]);
    if (!tree[entry.path]) load(entry.path, showAll);
  };
  const view = (entry: ProjectEntry) => app().ReadProjectFile(session.id, entry.path).then(setFile).catch((error) => { setFile(null); onError(error); });
  const copy = (text: string, what: string) => window.runtime.ClipboardSetText(text).then(() => setNote(`Copied ${what}.`)).catch(onError);
  const copyFull = (entry: ProjectEntry) => app().ProjectFullPath(session.id, entry.path).then((full) => copy(full, "the full path")).catch(onError);
  const rows = (dir: string, depth: number): ReactNode => (tree[dir] ?? []).map((entry) => {
    const expanded = open.includes(entry.path);
    const kind = fileIconFor(entry.name);
    return <Fragment key={entry.path}>
      <div className="file-row" style={{ paddingLeft: 8 + depth * 14 }}
        onContextMenu={(event) => { event.preventDefault(); setMenu({ x: Math.min(event.clientX, window.innerWidth - 210), y: Math.min(event.clientY, window.innerHeight - 120), entry }); }}>
        <button className="file-name" title={`${entry.path} (right-click for more)`} onClick={() => entry.dir ? toggle(entry) : void view(entry)}>
          <span className="file-ico" style={{ color: entry.dir ? "#d6a85c" : kind.color }}><Icon name={entry.dir ? (expanded ? "folderOpen" : "folder") : kind.icon} size={15} /></span><span className="file-label">{entry.name}</span>
        </button>
      </div>
      {entry.dir && expanded && rows(entry.path, depth + 1)}
    </Fragment>;
  });
  return <aside className="file-drawer" aria-label="Project files">
    <div className="file-drawer-head">
      <strong>Files</strong>
      <label title="Also show .git, node_modules and similar folders"><input type="checkbox" checked={showAll} onChange={(event) => setShowAll(event.target.checked)} /> all</label>
      <button className="file-act" onClick={onClose} aria-label="Close the file drawer">x</button>
    </div>
    <div className="file-tree">{tree[""] ? (tree[""].length ? rows("", 0) : <div className="toolbar-popover-note">Empty folder.</div>) : <div className="toolbar-popover-note">Loading...</div>}</div>
    {note && <div className="toolbar-popover-note">{note}</div>}
    {menu && <div className="file-menu" role="menu" style={{ left: menu.x, top: menu.y }} onPointerDown={(event) => event.stopPropagation()}>
      <button role="menuitem" onClick={() => { void copy(menu.entry.path, "the relative path"); setMenu(null); }}><Icon name="copy" size={14} />Copy relative name</button>
      <button role="menuitem" onClick={() => { void copyFull(menu.entry); setMenu(null); }}><Icon name="copy" size={14} />Copy full name</button>
      <button role="menuitem" onClick={() => { app().RevealProjectFile(session.id, menu.entry.path).catch(onError); setMenu(null); }}><Icon name="reveal" size={14} />Open in File Explorer</button>
    </div>}
    {file && <div className="file-view">
      <div className="file-view-head"><span title={file.path}>{file.path}</span><button className="file-act" onClick={() => setFile(null)} aria-label="Close the file">x</button></div>
      {file.truncated && <div className="toolbar-popover-note">Only the first part of this file is shown.</div>}
      {isMarkdown(file.path) ? <div className="md">{parseMarkdown(file.text).map(renderBlock)}</div> : <pre className="md-code">{file.text}</pre>}
    </div>}
  </aside>;
}
