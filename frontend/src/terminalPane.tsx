import { useEffect, useRef, useState, type ReactNode } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import { useAppearance } from "./theme/appearance";
import { app, GitStatus, HarnessWidgets, Session } from "./bridge";
import { gitParts, gitTooltip } from "./gitChip";
import { parseOsc52 } from "./osc52";
import { Icon } from "./icons";
import { theme } from "./theme/theme";
import { combineSignal, composeInjection, controlSendPlan, freshLogState, planSend, promptKind, readDraft, readHarnessSignal } from "./attention";
import { SessionUsage, formatTokens, formatUSD, hasUsage, usageTooltip } from "./usage";
import { HarnessIcon } from "./harnessIcons";
import { HarnessToolbar, InterruptButton, sendNotice } from "./harnessActions";
import type { QueueHooks } from "./promptQueue";
import { FileDrawer } from "./fileDrawer";
import { ConversationsDrawer } from "./conversationsDrawer";
import { reportFrontendFailure, isStoppedSessionError } from "./errors";
import { Sep, sessionColorStyle, statusLabel } from "./sessionVisuals";
import { isExpensiveModel } from "./modelTier";
import { clearPollFailure, shouldReportPollFailure } from "./polling";

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

const isEmptyClipboardError = (error: unknown) => /operation completed successfully/i.test(error instanceof Error ? error.message : String(error));

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
    // An image-only clipboard has no text: Wails then reports Windows error 0, which is not a failure.
    void window.runtime.ClipboardGetText().then((text) => { if (text) terminal.paste(text); }).catch((error) => { if (!isEmptyClipboardError(error)) onError(error); });
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
  const draft = readDraft(terminal);
  const plan = planSend(readHarnessSignal(session.tool, terminal, terminalTitles.get(session.id) ?? ""), submit, draft);
  if (plan === "blocked") {
    fail(sendNotice(plan, label));
    return;
  }
  const injected = composeInjection(draft, text, plan === "submit");
  if (injected.includes("\n")) terminal.paste(injected);
  else terminal.input(injected, true);
  // Enter goes separately so the harness has finished handling the pasted text.
  if (plan === "submit") window.setTimeout(() => terminal.input("\r", true), 150);
  notify(sendNotice(plan, label));
  window.setTimeout(() => terminal.focus(), 160);
}

// Types text at the cursor of the session's prompt without submitting it. Returns why it was refused, or "".
export function insertAtPrompt(session: Session, text: string): string {
  const terminal = terminalInstances.get(session.id);
  if (!terminal || !session.runningHere) return "Start the session first: its terminal is not open.";
  terminal.input(text, true);
  window.setTimeout(() => terminal.focus(), 50);
  return "";
}

// Screen cue merged with the harness's own log answer, the same signal attention uses.
function liveSignal(id: string, tool: string, terminal: Parameters<typeof readHarnessSignal>[1]) {
  return combineSignal(readHarnessSignal(tool, terminal, terminalTitles.get(id) ?? ""), freshLogState(id, Date.now()));
}

// Types text that arrived over the control channel. Returns why it was refused, or "" when typed.
export function controlSend(session: Session, text: string, submit: boolean, allowBusySubmit = false): string {
  const terminal = terminalInstances.get(session.id);
  if (!terminal || !session.runningHere) return "the session has no open terminal";
  const signal = liveSignal(session.id, session.tool, terminal);
  const plan = controlSendPlan(signal, submit, readDraft(terminal), signal === "input" && promptKind(session.tool, terminal) === "question", allowBusySubmit);
  if (!plan.allowed) return plan.reason;
  const injected = composeInjection(readDraft(terminal), text, submit);
  if (injected.includes("\n")) terminal.paste(injected);
  else terminal.input(injected, true);
  if (submit) window.setTimeout(() => terminal.input("\r", true), 150);
  return "";
}

// How the prompt queue reads and drives the live terminals. controlSend re-checks the screen
// itself, so a send is refused when the harness changed state since the scheduler looked.
export const queueHooks: QueueHooks = {
  signalOf: (session) => liveSignal(session.id, session.tool, terminalInstances.get(session.id)),
  draftOf: (session) => { const terminal = terminalInstances.get(session.id); return terminal ? readDraft(terminal) : ""; },
  send: (session, text) => controlSend(session as Session, text, true),
  sent: (session) => { app().RecordQueueSend(session.id, "sent").catch((error) => console.error("TauShell could not record a queued send.", error)); },
};

export function interruptSession(session: Session, keys: string[]) {
  const terminal = terminalInstances.get(session.id);
  if (!terminal || !session.runningHere) return;
  // Separate presses with a gap so the harness sees them as distinct keys, not one escape sequence.
  keys.forEach((key, index) => window.setTimeout(() => terminal.input(key, true), index * 250));
  terminal.focus();
}

function GitChip({ session, git, onChange, onError, onNotice }: { session: Session; git: GitStatus; onChange: (status: GitStatus) => void; onError: (error: unknown) => void; onNotice?: (message: string) => void }) {
  const [syncing, setSyncing] = useState(false);
  const parts = gitParts(git);
  // The Go host returns the summary; failures surface through onError.
  const sync = () => {
    setSyncing(true);
    app().SyncGit(session.id)
      .then((summary) => onNotice?.(summary), onError)
      .then(() => app().GetGitStatus(session.id))
      .then((next) => { if (next) onChange(next); }, onError)
      .finally(() => setSyncing(false));
  };
  return <>
    <span className="toolbar-branch" title={gitTooltip(git)}>
      <Icon name={git.worktree ? "worktree" : "branch"} size={13} />{parts.label}{parts.dirty && <b className="git-flag">*</b>}
      {parts.ahead > 0 && <span className="git-count" aria-label={`${parts.ahead} unpushed`}><Icon name="arrowUp" size={12} />{parts.ahead}</span>}
      {parts.behind > 0 && <span className="git-count" aria-label={`${parts.behind} to pull`}><Icon name="arrowDown" size={12} />{parts.behind}</span>}
    </span>
    <button className="text-button icon-button git-sync-button" disabled={syncing} title="Sync changes: commit all pending changes, pull (rebase) and push" aria-label="Sync changes" onClick={sync}><Icon name="sync" size={14} /></button>
    {git.worktree && <button className="text-button icon-button git-sync-button" title="Open a GitHub pull request for this branch" aria-label="Open pull request" onClick={() => { app().OpenPullRequest(session.id).catch(onError); }}><Icon name="pullRequest" size={14} /></button>}
  </>;
}

function UsageChips({ usage }: { usage: SessionUsage }) {
  const title = usageTooltip(usage);
  return <>
    <Sep /><span className="toolbar-usage" title={title}>{formatTokens(usage.total.tokens)}</span>
    {(usage.total.usd > 0 || usage.total.unpriced) && <><Sep /><span className="toolbar-usage" title={title}>{formatUSD(usage.total)}</span></>}
  </>;
}

export function TerminalPane({ session, visible, busy, harness, onSend, onNotice, toasts, onInterrupt, onLaunch, onResumeConversation, onError }: {
  session?: Session;
  visible: boolean;
  busy: boolean;
  harness?: { widgets: HarnessWidgets; modelSupported: boolean };
  onSend?: (text: string, submit: boolean, label: string) => void;
  onNotice?: (message: string) => void;
  // This session's toasts, shown between the toolbar and the terminal.
  toasts?: ReactNode;
  onInterrupt: () => void;
  onLaunch: (session: Session, mode: "start" | "restore" | "fresh") => void;
  onResumeConversation: (session: Session, harnessSessionId: string) => void;
  onError: (error: unknown) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const runningRef = useRef(session?.runningHere ?? false);
  runningRef.current = session?.runningHere ?? false;
  const [git, setGit] = useState<GitStatus | null>(null);
  const [model, setModel] = useState("");
  const [usage, setUsage] = useState<SessionUsage | null>(null);
  const [drawer, setDrawer] = useState<"files" | "history" | null>(null);
  const filesOpen = drawer === "files";
  const pollFailures = useRef(new Set<string>());
  const sessionId = session?.id;
  useEffect(() => {
    if (!sessionId || !visible) return;
    let cancelled = false;
    let loading = false;
    const load = async () => {
      if (loading) return;
      loading = true;
      const requests = await Promise.allSettled([
        app().GetGitStatus(sessionId),
        app().GetSessionModel(sessionId),
        app().GetSessionUsage(sessionId),
      ]);
      if (!cancelled) {
        const [branchResult, modelResult, usageResult] = requests;
        const pollKey = (field: string) => `${sessionId}:${field}`;
        if (branchResult.status === "fulfilled") {
          clearPollFailure(pollFailures.current, pollKey("branch"));
          setGit(branchResult.value);
        } else if (shouldReportPollFailure(pollFailures.current, pollKey("branch"))) {
          onError(new Error(`Branch refresh failed: ${String(branchResult.reason)}`, { cause: branchResult.reason }));
        }
        if (modelResult.status === "fulfilled") {
          clearPollFailure(pollFailures.current, pollKey("model"));
          setModel(modelResult.value);
        } else if (shouldReportPollFailure(pollFailures.current, pollKey("model"))) {
          onError(new Error(`Model refresh failed: ${String(modelResult.reason)}`, { cause: modelResult.reason }));
        }
        if (usageResult.status === "fulfilled") {
          clearPollFailure(pollFailures.current, pollKey("usage"));
          setUsage(usageResult.value);
        } else if (shouldReportPollFailure(pollFailures.current, pollKey("usage"))) {
          onError(new Error(`Usage refresh failed: ${String(usageResult.reason)}`, { cause: usageResult.reason }));
        }
      }
      loading = false;
    };
    void load();
    const timer = window.setInterval(load, 5000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [sessionId, visible]);
  // Fetch from the remote now and every 5 minutes so the incoming-commits arrow can appear.
  useEffect(() => {
    if (!sessionId || !visible) return;
    let cancelled = false;
    const fetchRemote = async () => {
      try {
        if (await app().FetchGit(sessionId) && !cancelled) setGit(await app().GetGitStatus(sessionId));
      } catch { /* offline or no remote: keep the last known counts */ }
    };
    void fetchRemote();
    const timer = window.setInterval(fetchRemote, 5 * 60 * 1000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [sessionId, visible]);
  const { appearance, resolved } = useAppearance();
  useEffect(() => {
    const terminal = session ? terminalInstances.get(session.id) : undefined;
    if (!terminal) return;
    Object.assign(terminal.options, theme.terminal());
    try {
      fitInstances.get(session!.id)?.fit();
      if (session!.runningHere) void app().ResizeTerminal(session!.id, terminal.cols, terminal.rows).catch((err) => { if (!isStoppedSessionError(err)) onError(err); });
    } catch {
      // Hidden panes cannot be measured; the next show re-fits.
    }
  }, [session?.id, resolved, appearance.terminalFontSize]);
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
      terminal = new Terminal({ convertEol: false, cursorBlink: true, scrollback: 0, fontFamily: theme.fontMono(), ...theme.terminal() });
      terminal.loadAddon(fit);
      terminal.open(container.current);
      terminalInstances.set(session.id, terminal);
      terminal.onTitleChange((title) => terminalTitles.set(session.id, title));
      terminal.onData((data) => { lastInputAt.set(session.id, Date.now()); void app().WriteTerminal(session.id, data).catch((err) => onError(err)); });
      terminal.onBinary((data) => { void app().WriteTerminalBinary(session.id, btoa(data)).catch((err) => onError(err)); });
      terminal.attachCustomKeyEventHandler((event) => handleClipboardKey(event, terminal!, onError));
      // Harnesses with their own mouse selection (Copilot CLI) copy through OSC 52.
      terminal.parser.registerOscHandler(52, (payload) => {
        const text = parseOsc52(payload);
        if (text) void window.runtime.ClipboardSetText(text).catch((error) => onError(error));
        return true;
      });
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
    let focusFrame = 0;
    const fitFrame = requestAnimationFrame(() => {
      try {
        fit.fit();
        if (session.runningHere) void app().ResizeTerminal(session.id, terminal.cols, terminal.rows).catch((err) => { if (!isStoppedSessionError(err)) onError(err); });
      } catch (error) {
        reportFrontendFailure("ui.terminal-layout", error);
      }
      focusFrame = requestAnimationFrame(() => {
        if (session.runningHere && !document.querySelector(".modal, [role='dialog']")) terminal.focus();
      });
    });
    return () => {
      cancelAnimationFrame(fitFrame);
      cancelAnimationFrame(focusFrame);
    };
  }, [visible, session?.id, session?.runningHere]);
  if (!session) return <div className="terminal-missing">Session configuration is unavailable.</div>;
  return <div className="terminal-view" style={sessionColorStyle(session.color)} data-colored={session.color ? "" : undefined}>
    <div className="terminal-toolbar">
      <span className={`status-dot ${session.runningHere ? "online" : ""}`} /><span className="harness-tag"><HarnessIcon tool={session.tool} />{session.tool}</span>{harness && onSend && <InterruptButton interrupt={onInterrupt} />}
      <Sep /><span className="toolbar-path-group"><span className="toolbar-path">{session.workingDir}</span><button className="text-button icon-button path-reveal" title="Open the project folder in File Explorer" aria-label="Open the project folder in File Explorer" onClick={() => app().RevealProjectRoot(session.id).catch(onError)}><Icon name="reveal" size={14} /></button></span>
      {git?.branch && <><Sep /><GitChip session={session} git={git} onChange={setGit} onError={onError} onNotice={onNotice} /></>}
      {model && <><Sep /><span className={`toolbar-model ${isExpensiveModel(model) ? "expensive" : ""}`} title={isExpensiveModel(model) ? "Expensive model (last recorded by the harness)" : "Model last recorded by the harness"}><Icon name="model" size={13} />{model}</span></>}
      {hasUsage(usage?.total) && <UsageChips usage={usage!} />}
      <span className={`terminal-status ${!session.runningHere && session.status === "failed" ? "status-warn-text" : ""}`}>{session.runningHere ? "" : statusLabel(session.status) || "Not started"}</span>
      {!session.runningHere && (session.hasResumeId && session.resumeNote ? <button className="secondary-button terminal-action" disabled={busy} title={session.resumeNote} onClick={() => onLaunch(session, "fresh")}>Start fresh</button> : <button className="primary-button terminal-action" disabled={busy} onClick={() => onLaunch(session, session.hasResumeId ? "restore" : "start")}>{session.hasResumeId ? "Resume" : "Start"}</button>)}
    </div>
    {toasts}
    <div className="terminal-body">
      <div className="terminal-container" ref={container} />
      <div className="side-rail">
        <button className="text-button terminal-action icon-button" aria-pressed={filesOpen} title="Files: browse the project files (read-only)" aria-label="Files: browse the project files (read-only)" onClick={() => setDrawer(filesOpen ? null : "files")}><Icon name="files" /></button>
        <button className="text-button terminal-action icon-button" aria-pressed={drawer === "history"} title="History: past conversations of this session" aria-label="History: past conversations of this session" onClick={() => setDrawer(drawer === "history" ? null : "history")}><Icon name="history" /></button>
        {harness && onSend && <HarnessToolbar session={session} widgets={harness.widgets} modelSupported={harness.modelSupported} send={onSend} onError={onError} />}
      </div>
      {filesOpen && <FileDrawer session={session} onError={onError} onInsert={(path) => { const why = insertAtPrompt(session, /\s/.test(path) ? `"${path}" ` : `${path} `); if (why) onError(new Error(why)); }} />}
      {drawer === "history" && <ConversationsDrawer session={session} busy={busy} onResume={onResumeConversation} onClose={() => setDrawer(null)} onError={onError} />}
    </div>
  </div>;
}
