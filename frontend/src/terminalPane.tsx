import { useEffect, useRef, useState } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import { app, HarnessWidgets, Session } from "./bridge";
import { theme } from "./theme/theme";
import { composeInjection, readDraft, readHarnessSignal } from "./attention";
import { SessionUsage, formatTokens, formatUSD, hasUsage, usageTooltip } from "./usage";
import { HarnessToolbar, planSend, sendNotice } from "./harnessActions";
import { reportFrontendFailure, isStoppedSessionError } from "./errors";
import { Sep, statusLabel } from "./sessionVisuals";

export const terminalInstances = new Map<string, Terminal>();
export const lastInputAt = new Map<string, number>();
export const terminalTitles = new Map<string, string>();
// Output within this window after a keystroke is treated as echo, not harness work.
export const echoWindowMs = 400;

export const fitInstances = new Map<string, FitAddon>();
const terminalReadyWaiters = new Map<string, Set<(terminal: Terminal) => void>>();

export function waitForTerminal(id: string): Promise<Terminal> {
  const terminal = terminalInstances.get(id);
  if (terminal) return Promise.resolve(terminal);

  return new Promise((resolve, reject) => {
    let settled = false;
    const waiters = terminalReadyWaiters.get(id) ?? new Set<(terminal: Terminal) => void>();
    terminalReadyWaiters.set(id, waiters);

    const removeWaiter = () => {
      waiters.delete(onReady);
      if (waiters.size === 0 && terminalReadyWaiters.get(id) === waiters) terminalReadyWaiters.delete(id);
    };
    const onReady = (ready: Terminal) => {
      if (settled) return;
      settled = true;
      window.clearTimeout(timeout);
      removeWaiter();
      resolve(ready);
    };
    const timeout = window.setTimeout(() => {
      if (settled) return;
      settled = true;
      removeWaiter();
      reject(new Error(`Terminal for session "${id}" did not initialize.`));
    }, 5000);
    waiters.add(onReady);
  });
}

function notifyTerminalReady(id: string, terminal: Terminal) {
  const waiters = terminalReadyWaiters.get(id);
  if (!waiters) return;
  terminalReadyWaiters.delete(id);
  waiters.forEach((resolve) => resolve(terminal));
}

// Windows Terminal convention: Ctrl+C copies only when text is selected (otherwise it is ^C
// for the harness); Ctrl+V and Ctrl+Shift+C/V always paste/copy. Uses the native clipboard
// because WebView2's navigator.clipboard needs a permission prompt.
function handleClipboardKey(event: globalThis.KeyboardEvent, terminal: Terminal, onError: (error: unknown) => void): boolean {
  // App shortcuts (tab switching, command palette) are handled at window level; the terminal must not consume them.
  if (event.ctrlKey && !event.altKey && !event.metaKey &&
    (event.key === "Tab" || (event.shiftKey && event.key.toLowerCase() === "p") || (!event.shiftKey && /^[1-9]$/.test(event.key)))) return false;
  if (!event.ctrlKey || event.altKey || event.metaKey) return true;
  const key = event.key.toLowerCase();
  const copy = key === "c" && (event.shiftKey || terminal.hasSelection());
  const paste = key === "v";
  if (!copy && !paste) return true;
  // preventDefault stops the browser's own paste event from inserting the text a second time.
  event.preventDefault();
  if (event.type !== "keydown") return false;
  if (copy) {
    const text = terminal.getSelection();
    if (text) {
      void window.runtime.ClipboardSetText(text).then(() => terminal.clearSelection()).catch((error) => onError(error));
    }
  } else {
    void window.runtime.ClipboardGetText().then((text) => { if (text) terminal.paste(text); }).catch((error) => onError(error));
  }
  return false;
}

// Harness widgets type into the terminal through xterm so input accounting matches real typing.
// What the harness shows decides whether text is submitted, only inserted, or refused.
export function sendToSession(session: Session, text: string, submit: boolean, label: string, notify: (message: string) => void, fail: (message: string) => void) {
  const terminal = terminalInstances.get(session.id);
  if (!terminal || !session.runningHere) {
    fail(`${session.name} is not running, so nothing was sent.`);
    return;
  }
  const plan = planSend(readHarnessSignal(session.tool, terminal, terminalTitles.get(session.id) ?? ""), submit);
  if (plan === "blocked") {
    fail(sendNotice(plan, label));
    return;
  }
  const injected = composeInjection(readDraft(terminal), text, plan === "submit");
  if (injected.includes("\n")) terminal.paste(injected);
  else terminal.input(injected, true);
  // Enter goes separately so the harness has finished handling the pasted text.
  if (plan === "submit") window.setTimeout(() => terminal.input("\r", true), 150);
  notify(sendNotice(plan, label));
  window.setTimeout(() => terminal.focus(), 160);
}

export function interruptSession(session: Session, keys: string[]) {
  const terminal = terminalInstances.get(session.id);
  if (!terminal || !session.runningHere) return;
  // Separate presses with a gap so the harness sees them as distinct keys, not one escape sequence.
  keys.forEach((key, index) => window.setTimeout(() => terminal.input(key, true), index * 250));
  terminal.focus();
}

export function TerminalPane({ session, visible, busy, harness, onSend, onInterrupt, onLaunch, onError }: {
  session?: Session;
  visible: boolean;
  busy: boolean;
  harness?: { widgets: HarnessWidgets; modelSupported: boolean };
  onSend?: (text: string, submit: boolean, label: string) => void;
  onInterrupt: () => void;
  onLaunch: (session: Session, mode: "start" | "restore" | "fresh") => void;
  onError: (error: unknown) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const runningRef = useRef(session?.runningHere ?? false);
  runningRef.current = session?.runningHere ?? false;
  const [branch, setBranch] = useState("");
  const [model, setModel] = useState("");
  const [usage, setUsage] = useState<SessionUsage | null>(null);
  const sessionId = session?.id;
  useEffect(() => {
    if (!sessionId || !visible) return;
    let cancelled = false;
    const load = () => {
      void app().GetGitBranch(sessionId).then((name) => { if (!cancelled) setBranch(name); }).catch(() => { if (!cancelled) setBranch(""); });
      void app().GetSessionModel(sessionId).then((name) => { if (!cancelled) setModel(name); }).catch(() => { if (!cancelled) setModel(""); });
      void app().GetSessionUsage(sessionId).then((value) => { if (!cancelled) setUsage(value); }).catch(() => { if (!cancelled) setUsage(null); });
    };
    load();
    const timer = window.setInterval(load, 5000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [sessionId, visible]);
  useEffect(() => {
    if (!session || !container.current) return;
    let terminal = terminalInstances.get(session.id);
    const fit = new FitAddon();
    fitInstances.set(session.id, fit);
    const pointerEventsLogged = new Set<string>();
    const logPointerEvent = (event: "pointerdown" | "mousedown") => {
      if (pointerEventsLogged.has(event)) return;
      pointerEventsLogged.add(event);
      void app().LogTerminalPointerEvent(session.id, event).catch((error) => onError(error));
    };
    const onPointerDown = () => logPointerEvent("pointerdown");
    const onMouseDown = () => logPointerEvent("mousedown");
    container.current.addEventListener("pointerdown", onPointerDown, true);
    container.current.addEventListener("mousedown", onMouseDown, true);
    if (!terminal) {
      terminal = new Terminal({ convertEol: false, cursorBlink: true, scrollback: 0, fontFamily: theme.fontMono(), fontSize: theme.terminal().fontSize, theme: { background: theme.terminal().background, foreground: theme.terminal().foreground, cursor: theme.terminal().cursor } });
      terminal.loadAddon(fit);
      terminal.open(container.current);
      terminalInstances.set(session.id, terminal);
      terminal.onTitleChange((title) => terminalTitles.set(session.id, title));
      terminal.onData((data) => { lastInputAt.set(session.id, Date.now()); void app().WriteTerminal(session.id, data).catch((err) => onError(err)); });
      terminal.onBinary((data) => { void app().WriteTerminalBinary(session.id, btoa(data)).catch((err) => onError(err)); });
      terminal.attachCustomKeyEventHandler((event) => handleClipboardKey(event, terminal!, onError));
    } else {
      terminal.loadAddon(fit);
      terminal.open(container.current);
    }
    let lastMouseTrackingMode: string | undefined;
    const mouseModeSubscription = terminal.onWriteParsed(() => {
      const mode = terminal!.modes.mouseTrackingMode;
      if (mode === lastMouseTrackingMode) return;
      lastMouseTrackingMode = mode;
      void app().LogTerminalMouseTrackingMode(session.id, mode).catch((error) => onError(error));
    });
    let pendingPtyResize: number | undefined;
    let sentSize = "";
    const resize = () => {
      try {
        fit.fit();
        // Coalesce drag-resize bursts so the harness redraws once at the final size.
        window.clearTimeout(pendingPtyResize);
        pendingPtyResize = window.setTimeout(() => {
          const size = `${terminal!.cols}x${terminal!.rows}`;
          if (!runningRef.current || size === sentSize) return;
          sentSize = size;
          void app().ResizeTerminal(session.id, terminal!.cols, terminal!.rows).catch((err) => { if (!isStoppedSessionError(err)) onError(err); });
        }, 100);
      } catch (error) {
        reportFrontendFailure("ui.terminal-layout", error);
      }
    };
    const observer = new ResizeObserver(resize);
    observer.observe(container.current);
    resize();
    notifyTerminalReady(session.id, terminal);
    return () => {
      window.clearTimeout(pendingPtyResize);
      observer.disconnect();
      container.current?.removeEventListener("pointerdown", onPointerDown, true);
      container.current?.removeEventListener("mousedown", onMouseDown, true);
      mouseModeSubscription.dispose();
      terminal?.dispose();
      terminalInstances.delete(session.id);
      fitInstances.delete(session.id);
    };
  }, [session?.id]);
  useEffect(() => {
    if (!visible || !session) return;
    const terminal = terminalInstances.get(session.id);
    const fit = fitInstances.get(session.id);
    if (!terminal || !fit) return;
    requestAnimationFrame(() => {
      try {
        fit.fit();
        if (session.runningHere) void app().ResizeTerminal(session.id, terminal.cols, terminal.rows).catch((err) => { if (!isStoppedSessionError(err)) onError(err); });
      } catch (error) {
        reportFrontendFailure("ui.terminal-layout", error);
      }
    });
    if (!session.runningHere) return;
    // Delayed so it wins over a closing dialog or the clicked Start button restoring focus.
    const timer = window.setTimeout(() => {
      if (!document.querySelector(".modal, [role='dialog']")) terminal.focus();
    }, 120);
    return () => window.clearTimeout(timer);
  }, [visible, session?.id, session?.runningHere]);
  if (!session) return <div className="terminal-missing">Session configuration is unavailable.</div>;
  return <div className="terminal-view"><div className="terminal-toolbar"><span className={`status-dot ${session.runningHere ? "online" : ""}`} />{session.tool}<Sep /><span className="toolbar-path">{session.workingDir}</span>{branch && <><Sep /><span className="toolbar-branch">{branch}</span></>}{model && <><Sep /><span className="toolbar-model" title="Model last recorded by the harness">{model}</span></>}{hasUsage(usage?.total) && <><Sep /><span className="toolbar-usage" title={usageTooltip(usage!)}>{formatTokens(usage!.total.tokens)}</span>{(usage!.total.usd > 0 || usage!.total.unpriced) && <><Sep /><span className="toolbar-usage" title={usageTooltip(usage!)}>{formatUSD(usage!.total)}</span></>}</>}<span className={`terminal-status ${!session.runningHere && session.status === "failed" ? "status-warn-text" : ""}`}>{session.runningHere ? "" : statusLabel(session.status) || "Not started"}</span>{!session.runningHere && (session.hasResumeId && session.resumeNote ? <button className="secondary-button terminal-action" disabled={busy} title={session.resumeNote} onClick={() => onLaunch(session, "fresh")}>Start fresh</button> : <button className="primary-button terminal-action" disabled={busy} onClick={() => onLaunch(session, session.hasResumeId ? "restore" : "start")}>{session.hasResumeId ? "Resume" : "Start"}</button>)}{harness && onSend && <HarnessToolbar session={session} widgets={harness.widgets} modelSupported={harness.modelSupported} send={onSend} interrupt={onInterrupt} onError={onError} />}</div><div className="terminal-container" ref={container} /></div>;
}
