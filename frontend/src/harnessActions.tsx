import { ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { app, HarnessWidgets, ResolvedPreset, Session } from "./bridge";
import type { HarnessSignal } from "./attention";

// What to do with text bound for the harness, given what its own UI is showing.
//  - "blocked": a permission/selection prompt is open; typed text could answer it.
//  - "submit": the harness is idle, so type the text and press Enter.
//  - "insert": type the text only; the user presses Enter when ready.
export type SendPlan = "blocked" | "submit" | "insert";

export function planSend(signal: HarnessSignal, submit: boolean): SendPlan {
  if (signal === "input") return "blocked";
  return submit && signal === "idle" ? "submit" : "insert";
}

export function sendNotice(plan: SendPlan, label: string): string {
  if (plan === "blocked") return "Not sent: the harness is waiting for an answer. Answer it in the terminal first.";
  if (plan === "submit") return `Sent ${label}. TauShell cannot confirm the harness applied it.`;
  return `Inserted ${label}; press Enter in the terminal when ready.`;
}

export type SendFn = (text: string, submit: boolean, label: string) => void;

export type PaletteItem = {
  id: string;
  group: string;
  label: string;
  hint?: string;
  run: () => void;
};

const mruKey = "taushell.palette-mru.v1";

export function readMru(): string[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(mruKey) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((id): id is string => typeof id === "string").slice(0, 20) : [];
  } catch {
    return [];
  }
}

function writeMru(id: string) {
  try {
    localStorage.setItem(mruKey, JSON.stringify([id, ...readMru().filter((current) => current !== id)].slice(0, 20)));
  } catch (error) {
    console.error("TauShell could not save the command palette history.", error);
  }
}

// Subsequence match; lower score is better, -1 means no match.
export function fuzzyScore(query: string, text: string): number {
  if (!query) return 0;
  const haystack = text.toLowerCase();
  const direct = haystack.indexOf(query);
  if (direct >= 0) return direct;
  let position = 0;
  let gaps = 0;
  for (const character of query) {
    const found = haystack.indexOf(character, position);
    if (found < 0) return -1;
    gaps += found - position;
    position = found + 1;
  }
  return 100 + gaps;
}

function ToolbarMenu({ label, title, disabled, onOpen, children }: {
  label: ReactNode;
  title: string;
  disabled?: boolean;
  onOpen?: () => void;
  children: (close: () => void) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onPointer = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setOpen(false); };
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    window.addEventListener("pointerdown", onPointer, true);
    window.addEventListener("keydown", onKey);
    return () => { window.removeEventListener("pointerdown", onPointer, true); window.removeEventListener("keydown", onKey); };
  }, [open]);
  return <div className="toolbar-menu" ref={root}>
    <button className="text-button terminal-action" aria-haspopup="menu" aria-expanded={open} disabled={disabled} title={title}
      onClick={() => { if (!open) onOpen?.(); setOpen(!open); }}>{label}</button>
    {open && <div className="toolbar-popover" role="menu">{children(() => setOpen(false))}</div>}
  </div>;
}

export function HarnessToolbar({ session, widgets, modelSupported, send, interrupt, onError }: {
  session: Session;
  widgets: HarnessWidgets;
  modelSupported: boolean;
  send: SendFn;
  interrupt: () => void;
  onError: (error: unknown) => void;
}) {
  const [presets, setPresets] = useState<ResolvedPreset[] | null>(null);
  const [presetError, setPresetError] = useState("");
  const [customModel, setCustomModel] = useState("");
  const loadPresets = () => {
    app().GetPresets(session.id)
      .then((list) => { setPresets(list ?? []); setPresetError(""); })
      .catch((error) => { setPresets([]); setPresetError(error instanceof Error ? error.message : String(error)); });
  };
  const sendModel = (model: string, close: () => void) => {
    const id = model.trim();
    if (!id || /\s/.test(id)) { onError(new Error("Model IDs cannot contain spaces.")); return; }
    send(`/model ${id}`, true, `/model ${id}`);
    close();
  };
  return <div className="harness-actions">
    <ToolbarMenu label="Prompts" title="Send a saved prompt" onOpen={loadPresets}>
      {(close) => <>
        {presetError && <div className="toolbar-popover-note error">{presetError}</div>}
        {presets === null && <div className="toolbar-popover-note">Loading...</div>}
        {presets?.length === 0 && !presetError && <div className="toolbar-popover-note">No presets yet.</div>}
        {presets?.map((preset) => <button role="menuitem" key={preset.name} title={preset.text}
          onClick={() => { send(preset.text, preset.submit, `"${preset.name}"`); close(); }}>{preset.name}</button>)}
        <hr />
        <button role="menuitem" onClick={() => { app().OpenPresetsFile().catch(onError); close(); }}>Edit presets...</button>
      </>}
    </ToolbarMenu>
    <ToolbarMenu label="Commands" title="Send a harness slash command">
      {(close) => widgets.commands.map((command) => <button role="menuitem" key={command.text} title={command.text}
        onClick={() => { send(command.text, true, command.text); close(); }}>{command.label}<small>{command.text}</small></button>)}
    </ToolbarMenu>
    {modelSupported && <ToolbarMenu label="Model" title="Ask the harness to switch model (sends /model)">
      {(close) => <>
        <button role="menuitem" onClick={() => { send("/model", true, "/model"); close(); }}>Open the harness model picker<small>/model</small></button>
        <hr />
        {widgets.models.map((model) => <button role="menuitem" key={model} onClick={() => sendModel(model, close)}>{model}</button>)}
        <form className="toolbar-popover-form" onSubmit={(event) => { event.preventDefault(); sendModel(customModel, close); }}>
          <input value={customModel} onChange={(event) => setCustomModel(event.target.value)} placeholder="Custom model ID" aria-label="Custom model ID" />
          <button type="submit" disabled={!customModel.trim()}>Send</button>
        </form>
      </>}
    </ToolbarMenu>}
    <button className="text-button terminal-action" title="Compact the conversation" onClick={() => send(widgets.compactText, true, widgets.compactText)}>Compact</button>
    <button className="text-button terminal-action" title="Interrupt the harness (Esc)" onClick={interrupt}>Interrupt</button>
  </div>;
}

export function CommandPalette({ items, onClose }: { items: PaletteItem[]; onClose: () => void }) {
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const mru = useMemo(readMru, []);
  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return items
      .map((item) => ({ item, score: fuzzyScore(needle, `${item.group} ${item.label}`) }))
      .filter((entry) => entry.score >= 0)
      .sort((a, b) => {
        if (!needle) {
          const left = mru.indexOf(a.item.id);
          const right = mru.indexOf(b.item.id);
          if (left !== right) return (left < 0 ? 1e6 : left) - (right < 0 ? 1e6 : right);
          return 0;
        }
        return a.score - b.score;
      })
      .map((entry) => entry.item)
      .slice(0, 40);
  }, [items, query, mru]);
  useEffect(() => setIndex(0), [query]);
  const list = useRef<HTMLDivElement>(null);
  useEffect(() => { list.current?.querySelector(".active")?.scrollIntoView({ block: "nearest" }); }, [index]);
  const choose = (item: PaletteItem | undefined) => {
    if (!item) return;
    writeMru(item.id);
    onClose();
    item.run();
  };
  return <div className="modal-backdrop palette-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <div className="palette" role="dialog" aria-modal="true" aria-label="Command palette">
      <input autoFocus value={query} placeholder="Type a command, prompt or session..." aria-label="Command palette search"
        onChange={(event) => setQuery(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Escape") { event.preventDefault(); onClose(); }
          else if (event.key === "ArrowDown") { event.preventDefault(); setIndex((current) => Math.min(current + 1, visible.length - 1)); }
          else if (event.key === "ArrowUp") { event.preventDefault(); setIndex((current) => Math.max(current - 1, 0)); }
          else if (event.key === "Enter") { event.preventDefault(); choose(visible[index]); }
        }} />
      <div className="palette-list" ref={list} role="listbox">
        {visible.length === 0 && <div className="palette-empty">No matches.</div>}
        {visible.map((item, position) => <button key={item.id} role="option" aria-selected={position === index} className={position === index ? "active" : ""}
          onMouseEnter={() => setIndex(position)} onClick={() => choose(item)}>
          <span className="palette-group">{item.group}</span><span className="palette-label">{item.label}</span>{item.hint && <small>{item.hint}</small>}
        </button>)}
      </div>
    </div>
  </div>;
}
