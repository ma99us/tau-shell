import { CSSProperties, useEffect, useRef } from "react";
import { Session } from "./bridge";
import { theme } from "./theme/theme";
import { Activity } from "./attention";

const sessionColors = () => theme.sessionSwatches();

function projectInitials(workingDir: string, fallback: string): string {
  const folder = workingDir.split(/[\\/]+/).filter(Boolean).pop() ?? "";
  const letters = folder.split(/[-_. ]+/).map((word) => word.match(/[\p{L}\p{N}]/u)?.[0] ?? "").join("").toUpperCase();
  return letters || (fallback.match(/[\p{L}\p{N}]/u)?.[0] ?? "?").toUpperCase();
}

function readableTextColor(background: string): string {
  const hex = background.replace("#", "");
  if (!/^[0-9a-f]{6}$/i.test(hex))   return theme.fallbackSessionColor();
  const [r, g, b] = [0, 2, 4].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) => c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  const luminance = 0.2126 * r + 0.7152 * g + 0.0722 * b;
  return theme.sessionTextOn(luminance > 0.3);
}

export function ProjectIcon({ session }: { session: Session }) {
  const text = projectInitials(session.workingDir, session.name);
  const count = Array.from(text).length;
  const style: CSSProperties = {
    fontSize: `${Math.max(5, Math.min(16, 26 / count * 1.15))}px`,
    ...(session.color ? { background: session.color, borderColor: session.color, color: readableTextColor(session.color) } : {}),
  };
  return <div className="tool-icon project-icon" style={style} aria-hidden="true">{text}</div>;
}

export function sessionColorStyle(color?: string): CSSProperties | undefined {
  return color ? { "--session-color": color } as CSSProperties : undefined;
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
