import { Fragment, ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { highlightCode, languageFor, languageForFence } from "./highlight";
import { app, GitChange, GitPathInfo, ProjectEntry, ProjectFile, Session } from "./bridge";
import { buildChangeTree, ChangeNode, changeLabel, countChanges } from "./gitChanges";
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

export function renderBlock(block: Block, index: number): ReactNode {
  switch (block.kind) {
    case "heading": return <div className={`md-heading md-h${Math.min(block.level, 3)}`} key={index}>{renderInline(block.inline)}</div>;
    case "code": return <CodeView key={index} language={block.lang ? languageForFence(block.lang) : null} text={block.text} />;
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

function CodeView({ language, text }: { language: string | null; text: string }) {
  const html = useMemo(() => highlightCode(language, text), [language, text]);
  // The markup comes from highlight.js, which escapes the file's text.
  return html === null ? <pre className="md-code">{text}</pre> : <pre className="md-code"><code className="hljs" dangerouslySetInnerHTML={{ __html: html }} /></pre>;
}

const sizeKey = "taushell.fileDrawer.v1";
function readSizes(): { width: number; view: number } {
  try { const parsed = JSON.parse(localStorage.getItem(sizeKey) ?? "{}"); return { width: Number(parsed.width) || 360, view: Number(parsed.view) || 0 }; } catch { return { width: 360, view: 0 }; }
}

export function FileDrawer({ session, onClose, onError, onInsert }: { session: Session; onClose: () => void; onError: (error: unknown) => void; onInsert: (path: string) => void }) {
  const [tree, setTree] = useState<Record<string, ProjectEntry[]>>({});
  const [open, setOpen] = useState<string[]>([]);
  const [file, setFile] = useState<ProjectFile | null>(null);
  const [tab, setTab] = useState<"files" | "git">("files");
  const [changes, setChanges] = useState<GitChange[] | null | undefined>(undefined);
  const [collapsed, setCollapsed] = useState<string[]>([]);
  const [note, setNote] = useState("");
  const treeRef = useRef<HTMLDivElement>(null);
  const drawerRef = useRef<HTMLElement>(null);
  const [sizes, setSizes] = useState(readSizes);
  useEffect(() => { try { localStorage.setItem(sizeKey, JSON.stringify(sizes)); } catch { /* sizes are a convenience */ } }, [sizes]);
  // Drag a border: the left edge sets the drawer width, the file view top edge sets its height.
  const drag = (axis: "x" | "y") => (event: React.PointerEvent<HTMLDivElement>) => {
    const drawer = drawerRef.current;
    if (!drawer) return;
    event.preventDefault();
    const target = event.currentTarget;
    target.setPointerCapture(event.pointerId);
    const box = drawer.getBoundingClientRect();
    const move = (at: PointerEvent) => {
      if (axis === "x") setSizes((now) => ({ ...now, width: Math.round(Math.min(Math.max(box.right - at.clientX, 240), (drawer.parentElement?.clientWidth ?? 900) * 0.9)) }));
      else setSizes((now) => ({ ...now, view: Math.round(Math.min(Math.max(box.bottom - at.clientY, 80), box.height - 120)) }));
    };
    const stop = () => { target.removeEventListener("pointermove", move); target.removeEventListener("pointerup", stop); target.removeEventListener("pointercancel", stop); };
    target.addEventListener("pointermove", move); target.addEventListener("pointerup", stop); target.addEventListener("pointercancel", stop);
  };
  const [menu, setMenu] = useState<{ x: number; y: number; entry: ProjectEntry; git?: GitPathInfo } | null>(null);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    if (!menu) return;
    const close = () => setMenu(null);
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") close(); };
    window.addEventListener("pointerdown", close);
    window.addEventListener("blur", close);
    window.addEventListener("keydown", onKey);
    return () => { window.removeEventListener("pointerdown", close); window.removeEventListener("blur", close); window.removeEventListener("keydown", onKey); };
  }, [menu]);
  const selectedPath = file?.path;
  // The file view opens over the bottom of the drawer and shrinks the tree; keep the selected row visible.
  useEffect(() => {
    if (!selectedPath) return;
    const frame = requestAnimationFrame(() => treeRef.current?.querySelector(".file-row.selected")?.scrollIntoView({ block: "nearest" }));
    return () => cancelAnimationFrame(frame);
  }, [selectedPath]);
  // The parent passes a fresh onError each render; a ref keeps load stable so a re-render
  // never resets the tree, the open folders or the selected file.
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;
  const load = useCallback((dir: string) => {
    app().ListProjectDir(session.id, dir, true)
      .then((listing) => setTree((current) => ({ ...current, [dir]: listing.entries ?? [] })))
      .catch((error) => onErrorRef.current(error));
  }, [session.id]);
  useEffect(() => { setTree({}); setOpen([]); setFile(null); setChanges(undefined); setCollapsed([]); load(""); }, [session.id, load]);
  // The Git tab lists changed files; git status is cheap, so it refreshes while the tab is open.
  // The count on the tab label needs the list too, so the first load also runs from the Files tab.
  useEffect(() => {
    let cancelled = false;
    const refresh = () => app().ListGitChanges(session.id).then((list) => { if (!cancelled) setChanges(list); }).catch((error) => { if (!cancelled && tab === "git") onErrorRef.current(error); });
    void refresh();
    const timer = window.setInterval(refresh, tab === "git" ? 4000 : 15000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [session.id, tab, refresh]);
  const toggle = (entry: ProjectEntry) => {
    if (open.includes(entry.path)) { setOpen(open.filter((path) => path !== entry.path)); return; }
    setOpen([...open, entry.path]);
    if (!tree[entry.path]) load(entry.path);
  };
  // A file that cannot be previewed stays selected; the pane explains why and keeps its actions.
  const view = (entry: ProjectEntry) => app().ReadProjectFile(session.id, entry.path).then(setFile).catch((error) => {
    setFile({ path: entry.path, text: "", truncated: false, unsupported: error instanceof Error ? error.message : String(error) });
  });
  const copy = (text: string, what: string) => window.runtime.ClipboardSetText(text).then(() => setNote(`Copied ${what}.`)).catch(onError);
  const copyFull = (entry: { path: string }) => app().ProjectFullPath(session.id, entry.path).then((full) => copy(full, "the full path")).catch(onError);
  // Right-click opens the menu at once; the git actions join it when git answers.
  const openMenu = (event: React.MouseEvent, entry: ProjectEntry) => {
    event.preventDefault();
    setMenu({ x: Math.min(event.clientX, window.innerWidth - 210), y: Math.min(event.clientY, window.innerHeight - 260), entry });
    app().GetGitPathInfo(session.id, entry.path).then((git) => setMenu((current) => current && current.entry.path === entry.path ? { ...current, git } : current)).catch(() => undefined);
  };
  const gitAction = (entry: ProjectEntry, action: "stage" | "unstage" | "untrack" | "ignore") => {
    setMenu(null);
    app().GitPathAction(session.id, entry.path, action).then((summary) => { setNote(summary); setRefresh((now) => now + 1); }).catch(onError);
  };
  const rows = (dir: string, depth: number): ReactNode => (tree[dir] ?? []).map((entry) => {
    const expanded = open.includes(entry.path);
    const kind = fileIconFor(entry.name);
    return <Fragment key={entry.path}>
      <div className={`file-row ${!entry.dir && file?.path === entry.path ? "selected" : ""}`} style={{ paddingLeft: 8 + depth * 14 }}
        onContextMenu={(event) => openMenu(event, entry)}>
        <button className="file-name" title={`${entry.path} (right-click for more)`} onClick={() => entry.dir ? toggle(entry) : void view(entry)}>
          <span className="file-ico" style={{ color: entry.dir ? "var(--file-folder)" : kind.color }}><Icon name={entry.dir ? (expanded ? "folderOpen" : "folder") : kind.icon} size={15} /></span><span className="file-label">{entry.name}</span>
        </button>
      </div>
      {entry.dir && expanded && rows(entry.path, depth + 1)}
    </Fragment>;
  });
  const changeRows = (nodes: ChangeNode[], depth: number): ReactNode => nodes.map((node) => {
    const expanded = !collapsed.includes(node.path);
    const entry: ProjectEntry = { name: node.name, path: node.path, dir: node.dir, size: 0 };
    const kind = fileIconFor(node.name);
    const openChange = () => {
      if (node.code !== "D") { void view(entry); return; }
      setFile({ path: node.path, text: "", truncated: false, unsupported: "the file was deleted" });
    };
    return <Fragment key={node.path}>
      <div className={`file-row ${!node.dir && file?.path === node.path ? "selected" : ""}`} style={{ paddingLeft: 8 + depth * 14 }}
        onContextMenu={(event) => openMenu(event, entry)}>
        <button className="file-name" title={node.dir ? node.path : `${node.path} - ${changeLabel(node)}`} onClick={() => node.dir ? setCollapsed(expanded ? [...collapsed, node.path] : collapsed.filter((path) => path !== node.path)) : openChange()}>
          <span className="file-ico" style={{ color: node.dir ? "var(--file-folder)" : kind.color }}><Icon name={node.dir ? (expanded ? "folderOpen" : "folder") : kind.icon} size={15} /></span><span className="file-label">{node.name}</span>
        </button>
        {!node.dir && <span className="git-code" data-code={node.code} title={changeLabel(node)}>{node.code}</span>}
      </div>
      {node.dir && expanded && changeRows(node.children, depth + 1)}
    </Fragment>;
  });
  const changeTree = useMemo(() => buildChangeTree(changes ?? []), [changes]);
  const changeCount = useMemo(() => countChanges(changeTree), [changeTree]);
  const gitBody = changes === undefined ? <div className="toolbar-popover-note">Loading...</div>
    : changes === null ? <div className="toolbar-popover-note">This folder is not a git repository.</div>
    : changes.length === 0 ? <div className="toolbar-popover-note">No changes. The workspace is clean.</div>
    : changeRows(changeTree, 0);
  return <aside className="file-drawer" ref={drawerRef} style={{ width: `min(${sizes.width}px, 90%)` }} aria-label="Project files">
    <div className="file-resize file-resize-x" onPointerDown={drag("x")} role="separator" aria-orientation="vertical" aria-label="Resize the file drawer" />
    <div className="file-drawer-head">
      <div className="file-tabs" role="tablist">
        <button role="tab" aria-selected={tab === "files"} className={`file-tab ${tab === "files" ? "active" : ""}`} onClick={() => setTab("files")}>Files</button>
        <button role="tab" aria-selected={tab === "git"} className={`file-tab ${tab === "git" ? "active" : ""}`} onClick={() => setTab("git")} title="Changed files in this workspace">Git{changeCount > 0 && <span className="file-tab-count">{changeCount}</span>}</button>
      </div>
      <button className="file-act" onClick={onClose} aria-label="Close the file drawer">x</button>
    </div>
    <div className="file-tree" ref={treeRef}>{tab === "git" ? gitBody : tree[""] ? (tree[""].length ? rows("", 0) : <div className="toolbar-popover-note">Empty folder.</div>) : <div className="toolbar-popover-note">Loading...</div>}</div>
    {note && <div className="toolbar-popover-note">{note}</div>}
    {menu && <div className="file-menu" role="menu" style={{ left: menu.x, top: menu.y }} onPointerDown={(event) => event.stopPropagation()}>
      <button role="menuitem" onClick={() => { onInsert(menu.entry.path); setMenu(null); }}><Icon name="commands" size={14} />Insert path in prompt</button>
      <button role="menuitem" onClick={() => { void copy(menu.entry.path, "the relative path"); setMenu(null); }}><Icon name="copy" size={14} />Copy relative name</button>
      <button role="menuitem" onClick={() => { void copyFull(menu.entry); setMenu(null); }}><Icon name="copy" size={14} />Copy full name</button>
      <button role="menuitem" onClick={() => { app().RevealProjectFile(session.id, menu.entry.path).catch(onError); setMenu(null); }}><Icon name="reveal" size={14} />Open in File Explorer</button>
      {menu.git?.repo && <>
        <div className="file-menu-divider" role="separator" />
        <button role="menuitem" disabled={!menu.git.unstaged} onClick={() => gitAction(menu.entry, "stage")}><Icon name="arrowUp" size={14} />Stage (git add)</button>
        <button role="menuitem" disabled={!menu.git.staged} onClick={() => gitAction(menu.entry, "unstage")}><Icon name="arrowDown" size={14} />Unstage</button>
        <button role="menuitem" disabled={menu.git.ignored} onClick={() => gitAction(menu.entry, "ignore")} title="Append to the nearest existing .gitignore"><Icon name="stop" size={14} />Add to .gitignore</button>
        <button role="menuitem" disabled={!menu.git.tracked} onClick={() => gitAction(menu.entry, "untrack")} title="Remove from git, keep on disk; Sync changes then removes it from the remote"><Icon name="clear" size={14} />Stop tracking (keep file)</button>
      </>}
    </div>}
    {file && <div className="file-view" style={sizes.view ? { flex: "none", height: sizes.view } : undefined}>
      <div className="file-resize file-resize-y" onPointerDown={drag("y")} role="separator" aria-orientation="horizontal" aria-label="Resize the file preview" />
      <div className="file-view-head"><span title={file.path}>{file.path}</span>
        <button className="file-act" onClick={() => onInsert(file.path)} title="Insert the relative path into the prompt" aria-label="Insert the relative path into the prompt"><Icon name="commands" size={14} /></button>
        <button className="file-act" onClick={() => void copy(file.path, "the relative path")} title="Copy relative name" aria-label="Copy the relative name"><Icon name="copy" size={14} /></button>
        <button className="file-act" onClick={() => void copyFull(file)} title="Copy full name" aria-label="Copy the full name"><Icon name="copy" size={14} /></button>
        <button className="file-act" onClick={() => app().RevealProjectFile(session.id, file.path).catch(onError)} title="Open location in File Explorer" aria-label="Open the location in File Explorer"><Icon name="reveal" size={14} /></button>
        <button className="file-act" onClick={() => app().OpenProjectFile(session.id, file.path).catch(onError)} title="Open in the default application" aria-label="Open the file in its default application"><Icon name="external" size={14} /></button>
        <button className="file-act" onClick={() => setFile(null)} aria-label="Close the file">x</button></div>
      <div className="file-view-body">
      {file.truncated && <div className="toolbar-popover-note">Only the first part of this file is shown.</div>}
      {file.unsupported ? <div className="file-warning" role="alert">Warning: this file cannot be previewed ({file.unsupported.replace(/\.$/, "")}). You can still use the buttons above.</div> : file.imageUrl ? <div className="file-image"><img src={file.imageUrl} alt={file.path} /></div> : isMarkdown(file.path) ? <div className="md">{parseMarkdown(file.text).map(renderBlock)}</div> : <CodeView language={languageFor(file.path)} text={file.text} />}
      </div>
    </div>}
  </aside>;
}
