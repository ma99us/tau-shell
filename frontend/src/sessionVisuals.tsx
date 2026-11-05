import { CSSProperties, useEffect, useRef } from "react";
import { Session } from "./bridge";
import { theme } from "./theme/theme";
import { Activity } from "./attention";
import { inkOn, parseHex, sessionPalette, tintVars } from "./sessionColor";

const sessionColors = () => theme.sessionSwatches();

function initialsOf(text: string): string {
  return text.split(/[-_. ]+/).map((word) => word.match(/[\p{L}\p{N}]/u)?.[0] ?? "").join("").toUpperCase();
}

// Initials follow the session name (minus a clone " (N)" suffix) so renaming regenerates the icon.
export function projectInitials(name: string, workingDir: string): string {
  const folder = workingDir.split(/[\\/]+/).filter(Boolean).pop() ?? "";
  return initialsOf(name.replace(/\s\(\d+\)$/, "")) || initialsOf(folder) || "?";
}

function readableTextColor(background: string): string {
  const rgb = parseHex(background);
  if (!rgb) return theme.fallbackSessionColor();
  return inkOn(rgb, theme.sessionTextOn(false), theme.sessionTextOn(true));
}

export function ProjectIcon({ session }: { session: Session }) {
  const text = projectInitials(session.name, session.workingDir);
  const count = Array.from(text).length;
  const style: CSSProperties = {
    fontSize: `${Math.max(5, Math.min(16, 26 / count * 1.15))}px`,
    ...(session.color ? { background: session.color, borderColor: session.color, color: readableTextColor(session.color) } : {}),
  };
  const icon = <div className="tool-icon project-icon" style={style} aria-hidden="true">{text}</div>;
  // A default clone name ends in " (N)"; show N as a tag on the icon.
  const clone = session.name.match(/\s\((\d+)\)$/)?.[1];
  if (!clone) return icon;
  return <span className="project-icon-wrap">{icon}<span className="clone-tag" aria-hidden="true">{clone}</span></span>;
}

export function sessionColorStyle(color?: string): CSSProperties | undefined {
  if (!color) return undefined;
  const palette = sessionPalette(color, theme.panelBackground(), theme.sessionTextOn(false), theme.sessionTextOn(true), theme.warningColor());
  if (!palette) return { "--session-color": color, ...tintVars } as CSSProperties;
  return {
    ...tintVars,
    "--session-color": palette.color,
    "--session-ink": palette.ink,
    "--session-ink-hover": palette.inkHover,
    "--session-ink-active": palette.inkActive,
    "--session-bar": palette.bar,
    "--session-bar-ink": palette.barInk,
    "--session-bar-warn": palette.barWarn,
  } as CSSProperties;
}

export function SessionMenu({ x, y, session, onClose, onRename, onDuplicate, onColor }: {
  x: number;
  y: number;
  session: Session;
  onClose: () => void;
  onRename: () => void;
  onDuplicate: () => void;
  onColor: (color: string) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const closeOutside = (event: Event) => {
      if (!(event.target instanceof Node && ref.current?.contains(event.target))) onClose();
    };
    const closeOnEscape = (event: globalThis.KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    window.addEventListener("mousedown", closeOutside, true);
    window.addEventListener("keydown", closeOnEscape);
    window.addEventListener("blur", onClose);
    window.addEventListener("resize", onClose);
    ref.current?.querySelector<HTMLButtonElement>("button")?.focus();
    return () => {
      window.removeEventListener("mousedown", closeOutside, true);
      window.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("blur", onClose);
      window.removeEventListener("resize", onClose);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return <div className="context-menu" role="menu" ref={ref} style={{ left: Math.min(x, window.innerWidth - 216), top: Math.min(y, window.innerHeight - 150) }}>
    <button role="menuitem" className="context-menu-item" onClick={onRename}>Rename...</button>
    <button role="menuitem" className="context-menu-item" onClick={onDuplicate}>Clone</button>
    <div className="context-menu-label">Color</div>
    <div className="color-swatches">
      {sessionColors().map((color) => <button key={color.value} role="menuitemradio" aria-checked={session.color === color.value} className={`color-swatch ${session.color === color.value ? "selected" : ""}`} style={{ background: color.value }} title={color.name} aria-label={color.name} onClick={() => onColor(color.value)} />)}
    </div>
    <button role="menuitem" className="context-menu-item" disabled={!session.color} onClick={() => onColor("")}>Default color</button>
  </div>;
}

export function Sep() {
  return <span className="toolbar-sep" aria-hidden="true">|</span>;
}

export function statusLabel(status: string): string {
  return status === "failed" ? "interrupted" : status;
}

type ActivityKind = "working" | "input" | "quiet" | "stopped" | "failed";

function stateIconKind(session: Session, activity?: Activity): { kind: ActivityKind; unseen: boolean } {
  if (!session.runningHere) return { kind: session.status === "failed" ? "failed" : "stopped", unseen: false };
  const kind = activity?.input ? "input" : activity?.working ? "working" : "quiet";
  return { kind, unseen: !!activity?.unseen };
}

const stateLabels: Record<ActivityKind, string> = { working: "Working (output active)", input: "Waiting for your answer", quiet: "Quiet (output stopped)", stopped: "Not running", failed: "Failed" };

export function StateIcon({ session, activity }: { session: Session; activity?: Activity }) {
  const { kind, unseen } = stateIconKind(session, activity);
  const label = kind === "input" ? stateLabels.input : unseen ? "Quiet since you last looked (may need input)" : stateLabels[kind];
  return <span className={`state-icon ${kind} ${unseen ? "unseen" : ""}`} title={label} aria-label={label} role="img" />;
}
