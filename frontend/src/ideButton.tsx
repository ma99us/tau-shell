import { useEffect, useRef, useState } from "react";
import { app, Session } from "./bridge";
import { Icon } from "./icons";
import { IdeOption, readIdePick, writeIdePick } from "./idePick";

function IdeGlyph({ option }: { option: IdeOption }) {
  return option.icon ? <img className="ide-icon" src={option.icon} alt="" width={16} height={16} /> : <Icon name="code" size={14} />;
}

// Split button: the main part opens the project in the last-used IDE, the caret lists the others.
export function IdeButton({ session, onError, onNotice }: { session: Session; onError: (error: unknown) => void; onNotice?: (message: string) => void }) {
  const [options, setOptions] = useState<IdeOption[]>([]);
  const [pick, setPick] = useState("");
  const [open, setOpen] = useState(false);
  const [menuAt, setMenuAt] = useState({ left: 0, top: 0 });
  const group = useRef<HTMLSpanElement>(null);
  const sessionId = session.id;
  useEffect(() => {
    let cancelled = false;
    app().ListIDEs(sessionId).then((list) => {
      if (cancelled) return;
      const next = list ?? [];
      setOptions(next);
      setPick(readIdePick(localStorage, sessionId, next)?.id ?? "");
    }, onError);
    return () => { cancelled = true; };
  }, [sessionId, session.workingDir]);
  useEffect(() => {
    if (!open) return;
    const closeOutside = (event: Event) => { if (!(event.target instanceof Node && group.current?.contains(event.target))) setOpen(false); };
    const closeOnEscape = (event: globalThis.KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    window.addEventListener("mousedown", closeOutside, true);
    window.addEventListener("keydown", closeOnEscape);
    window.addEventListener("blur", close);
    function close() { setOpen(false); }
    return () => {
      window.removeEventListener("mousedown", closeOutside, true);
      window.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("blur", close);
    };
  }, [open]);
  const current = options.find((option) => option.id === pick) ?? options[0];
  if (!current) return null;
  const launch = (option: IdeOption) => {
    setOpen(false);
    setPick(option.id);
    writeIdePick(localStorage, sessionId, option.id);
    app().OpenInIDE(sessionId, option.id).then((note) => { if (note) onNotice?.(note); }, onError);
  };
  return <span className="ide-button" ref={group}>
    <button className="text-button icon-button ide-main" title={`Open the project in ${current.name}`} aria-label={`Open the project in ${current.name}`} onClick={() => launch(current)}><IdeGlyph option={current} /></button>
    {options.length > 1 && <button className="text-button icon-button ide-caret" title="Choose an IDE" aria-label="Choose an IDE" aria-haspopup="menu" aria-expanded={open} onClick={() => { const rect = group.current?.getBoundingClientRect(); if (rect) setMenuAt({ left: Math.min(rect.left, window.innerWidth - 220), top: rect.bottom + 4 }); setOpen(!open); }}><Icon name="chevron" size={10} /></button>}
    {open && <div className="ide-menu" role="menu" style={menuAt}>
      {options.map((option) => <button key={option.id} role="menuitemradio" aria-checked={option.id === current.id} className="context-menu-item ide-menu-item" onClick={() => launch(option)}><IdeGlyph option={option} />{option.name}</button>)}
    </div>}
  </span>;
}
