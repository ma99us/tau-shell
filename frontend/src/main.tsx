import { Component, ErrorInfo, FormEvent, KeyboardEvent, ReactNode, useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { app, Session, SessionEvent, SystemStatus } from "./bridge";
import "./style.css";

type Tab = { id: string; name: string };
const savedTabsKey = "taushell.tabs.v1";
const savedSelectionKey = "taushell.selected-tab.v1";
const recentFoldersKey = "taushell.recent-folders.v1";

function errorDetails(error: unknown): { message: string; stack: string } {
  if (error instanceof Error) return { message: error.message, stack: error.stack ?? "" };
  return { message: String(error), stack: "" };
}

function reportFrontendFailure(source: string, error: unknown, stack = ""): string {
  const details = errorDetails(error);
  const fullStack = stack || details.stack;
  const logger = window.go?.main?.App?.LogFrontendFailure;
  if (logger) {
    void logger(source, details.message, fullStack).catch((logError) => {
      console.error("TauShell could not write a frontend failure to its diagnostic log.", logError);
    });
  } else {
    console.error(`TauShell frontend failure (${source}):`, error);
  }
  return details.message;
}

window.addEventListener("error", (event) => {
  const details = errorDetails(event.error ?? event.message);
  reportFrontendFailure("window.error", event.error ?? event.message, `${details.stack}\n${event.filename}:${event.lineno}:${event.colno}`);
});
window.addEventListener("unhandledrejection", (event) => {
  reportFrontendFailure("window.unhandledrejection", event.reason);
});

class AppErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    reportFrontendFailure("react.render", error, `${error.stack ?? ""}\n${info.componentStack ?? ""}`);
  }

  render() {
    if (this.state.failed) return <main className="main-content"><div className="error-banner">TauShell encountered a display error. Check the diagnostic log for details.</div></main>;
    return this.props.children;
  }
}

function readRecentFolders(): string[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(recentFoldersKey) ?? "[]");
    return Array.isArray(parsed)
      ? parsed.filter((folder): folder is string => typeof folder === "string" && folder.length > 0).slice(0, 5)
      : [];
  } catch (error) {
    reportFrontendFailure("storage.recent-folders", error);
    return [];
  }
}

function readTabs(): Tab[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(savedTabsKey) ?? "[]");
    return Array.isArray(parsed)
      ? parsed.filter((tab): tab is Tab => typeof tab?.id === "string" && typeof tab?.name === "string")
      : [];
  } catch (error) {
    reportFrontendFailure("storage.tabs", error);
    return [];
  }
}

function App() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [tabs, setTabs] = useState<Tab[]>(readTabs);
  const [recentFolders, setRecentFolders] = useState<string[]>(readRecentFolders);
  const [backgroundTerminalIds, setBackgroundTerminalIds] = useState<string[]>([]);
  const [selected, setSelected] = useState(localStorage.getItem(savedSelectionKey) ?? "");
  const [tabScroll, setTabScroll] = useState({ left: false, right: false });
  const tabListRef = useRef<HTMLDivElement>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [closeRequest, setCloseRequest] = useState<Session | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const showError = (source: string, failure: unknown) => setError(reportFrontendFailure(source, failure));

  const refresh = async () => {
    try {
      const [nextSessions, nextStatus] = await Promise.all([
        app().GetSessions(),
        status ? Promise.resolve(status) : app().GetSystemStatus(),
      ]);
      setSessions(nextSessions);
      setBackgroundTerminalIds((current) => current.filter((id) =>
        nextSessions.some((session) => session.id === id && session.runningHere)));
      setTabs((current) => current
        .filter((tab) => nextSessions.some((session) => session.id === tab.id))
        .map((tab) => ({ ...tab, name: nextSessions.find((session) => session.id === tab.id)!.name })));
      setSelected((current) => nextSessions.some((session) => session.id === current) ? current : "");
      setStatus(nextStatus);
      setError("");
    } catch (err) {
      showError("ui.refresh", err);
    }
  };

  useEffect(() => {
    void refresh();
    const runtime = window.runtime;
    if (!runtime) return;
    runtime.EventsOnMultiple("session-event", (event: SessionEvent) => {
      if (event.kind === "state" && event.session) {
        setSessions((current) => current.map((session) => session.id === event.sessionId ? event.session! : session));
        if (!event.session.runningHere) {
          setBackgroundTerminalIds((current) => current.filter((id) => id !== event.sessionId));
        }
        terminalInstances.get(event.sessionId)?.writeln(`\r\n[session ${event.session.status}${event.session.lastError ? `: ${event.session.lastError}` : ""}]\r\n`);
      } else if (event.kind === "output" && event.dataBase64) {
        const encoded = atob(event.dataBase64);
        const bytes = new Uint8Array(encoded.length);
        for (let index = 0; index < encoded.length; index++) bytes[index] = encoded.charCodeAt(index);
        terminalInstances.get(event.sessionId)?.write(bytes);
      }
    }, -1);
    return () => runtime.EventsOff("session-event");
  }, []);

  useEffect(() => {
    localStorage.setItem(savedTabsKey, JSON.stringify(tabs));
    localStorage.setItem(savedSelectionKey, selected);
  }, [tabs, selected]);

  useEffect(() => {
    localStorage.setItem(recentFoldersKey, JSON.stringify(recentFolders));
  }, [recentFolders]);

  useEffect(() => {
    const tabList = tabListRef.current;
    if (!tabList) return;
    const updateScrollState = () => setTabScroll({
      left: tabList.scrollLeft > 0,
      right: tabList.scrollLeft + tabList.clientWidth < tabList.scrollWidth - 1,
    });
    const observer = new ResizeObserver(updateScrollState);
    observer.observe(tabList);
    tabList.addEventListener("scroll", updateScrollState);
    updateScrollState();
    return () => {
      observer.disconnect();
      tabList.removeEventListener("scroll", updateScrollState);
    };
  }, [tabs]);

  useEffect(() => {
    const activeTab = document.getElementById(selected ? `tab-${selected}` : "tab-home");
    activeTab?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [selected, tabs]);

  const handleTabKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const tabList = tabListRef.current;
    if (!tabList) return;
    const tabElements = Array.from(tabList.querySelectorAll<HTMLButtonElement>('[role="tab"]'));
    const currentIndex = tabElements.indexOf(event.currentTarget);
    const nextIndex = event.key === "Home"
      ? 0
      : event.key === "End"
        ? tabElements.length - 1
        : (currentIndex + (event.key === "ArrowRight" ? 1 : -1) + tabElements.length) % tabElements.length;
    const nextTab = tabElements[nextIndex];
    if (!nextTab) return;
    nextTab.focus();
    setSelected(nextTab.dataset.tabId ?? "");
  };

  const scrollTabs = (direction: -1 | 1) => {
    const tabList = tabListRef.current;
    if (tabList) tabList.scrollBy({ left: direction * Math.max(160, tabList.clientWidth * 0.75), behavior: "smooth" });
  };

  const openTab = (session: Session) => {
    setTabs((current) => current.some((tab) => tab.id === session.id) ? current : [...current, { id: session.id, name: session.name }]);
    setBackgroundTerminalIds((current) => current.filter((id) => id !== session.id));
    setSelected(session.id);
  };

  const launch = async (session: Session, mode: "start" | "restore" | "fresh") => {
    setBusy(true);
    setError("");
    try {
      openTab(session);
      const terminal = await waitForTerminal(session.id);
      const started = mode === "restore"
        ? await app().RestoreSession(session.id, terminal.cols, terminal.rows)
        : mode === "fresh"
          ? await app().StartFresh(session.id, terminal.cols, terminal.rows)
          : await app().StartSession(session.id, terminal.cols, terminal.rows);
      openTab(started);
      await refresh();
    } catch (err) {
      showError("ui.session-launch", err);
    } finally {
      setBusy(false);
    }
  };

  const removeTab = (id: string, retainRunningTerminal = false) => {
    const remainingTabs = tabs.filter((tab) => tab.id !== id);
    setTabs((current) => current.filter((tab) => tab.id !== id));
    setBackgroundTerminalIds((current) => {
      if (!retainRunningTerminal) return current.filter((terminalId) => terminalId !== id);
      return current.includes(id) ? current : [...current, id];
    });
    if (selected === id) {
      const nextSelected = remainingTabs[0]?.id ?? "";
      setSelected(nextSelected);
      requestAnimationFrame(() => document.getElementById(nextSelected ? `tab-${nextSelected}` : "tab-home")?.focus());
    }
  };
  const keepTerminalInBackground = (id: string) => removeTab(id, true);
  const closeTab = (id: string) => {
    const session = sessions.find((current) => current.id === id);
    if (session?.runningHere) setCloseRequest(session);
    else removeTab(id);
  };
  const stopAndCloseTab = async (session: Session) => {
    setBusy(true);
    try {
      await app().StopSession(session.id);
      removeTab(session.id);
      setCloseRequest(null);
      await refresh();
    } catch (err) {
      showError("ui.stop-session", err);
    } finally {
      setBusy(false);
    }
  };
  const terminalIds = Array.from(new Set([...tabs.map((tab) => tab.id), ...backgroundTerminalIds]));

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand"><span className="brand-mark">T</span><span>TauShell</span></div>
        <div className="window-caption">Local AI sessions</div>
        <button className="icon-button" title="Refresh" onClick={() => void refresh()}>R</button>
      </header>
      <div className="workspace">
        <aside className="sidebar">
          <div className="sidebar-heading">SESSIONS <button className="add-button" onClick={() => setDialogOpen(true)}>+</button></div>
          <button className={`nav-row ${selected ? "" : "active"}`} onClick={() => setSelected("")}>Home</button>
          {sessions.map((session) => (
            <button className={`session-row ${selected === session.id ? "active" : ""}`} key={session.id} onClick={() => openTab(session)}>
              <span className={`status-dot ${session.runningHere ? "online" : ""}`} />
              <span className="session-row-text"><strong>{session.name}</strong><small>{session.tool === "claude-code" ? "Claude Code" : "GitHub Copilot CLI"}</small></span>
            </button>
          ))}
          <div className="sidebar-foot">Local machine only</div>
        </aside>
        <main className="main">
          <div className="tab-strip">
            <button className="tab-scroll-button" aria-label="Scroll tabs left" disabled={!tabScroll.left} onClick={() => scrollTabs(-1)}>&lt;</button>
            <div className="tabs" role="tablist" aria-label="Workspace tabs" ref={tabListRef}>
              <button
                id="tab-home"
                className={`tab home-tab ${selected === "" ? "active" : ""}`}
                role="tab"
                aria-selected={selected === ""}
                aria-controls="panel-home"
                data-tab-id=""
                tabIndex={selected === "" ? 0 : -1}
                onKeyDown={handleTabKeyDown}
                onClick={() => setSelected("")}
              >Home</button>
              {tabs.map((tab) => (
                <div className={`tab-item ${selected === tab.id ? "active" : ""}`} key={tab.id} role="presentation">
                  <button
                    id={`tab-${tab.id}`}
                    className="tab tab-label"
                    role="tab"
                    aria-selected={selected === tab.id}
                    aria-controls={`panel-${tab.id}`}
                    data-tab-id={tab.id}
                    tabIndex={selected === tab.id ? 0 : -1}
                    onKeyDown={handleTabKeyDown}
                    onClick={() => setSelected(tab.id)}
                  >{tab.name}</button>
                  <button className="tab-close" aria-label={`Close ${tab.name}`} onClick={() => closeTab(tab.id)}>x</button>
                </div>
              ))}
            </div>
            <button className="tab-scroll-button" aria-label="Scroll tabs right" disabled={!tabScroll.right} onClick={() => scrollTabs(1)}>&gt;</button>
          </div>
          {error && <div className="error-banner">{error}<button onClick={() => setError("")}>Dismiss</button></div>}
          <section className="main-content">
            <div id="panel-home" className={`home-container ${selected ? "hidden" : ""}`} role="tabpanel" aria-labelledby="tab-home" tabIndex={0}>
              <Home sessions={sessions} status={status} busy={busy} onNew={() => setDialogOpen(true)} onLaunch={launch} />
            </div>
            {terminalIds.map((id) => {
              const tab = tabs.find((current) => current.id === id);
              const session = sessions.find((current) => current.id === id);
              if (!tab && !session?.runningHere) return null;
              return (
                <div id={`panel-${id}`} className={`terminal-tab-content ${tab && selected === id ? "" : "hidden"}`} role="tabpanel" aria-labelledby={`tab-${id}`} tabIndex={0} key={id}>
                  <TerminalPane session={session} visible={Boolean(tab && selected === id)} busy={busy} onLaunch={launch} onError={(failure) => showError("ui.terminal", failure)} />
                </div>
              );
            })}
          </section>
        </main>
      </div>
      {dialogOpen && <CreateSession recentFolders={recentFolders} onClose={() => setDialogOpen(false)} onCreated={(session) => {
        setSessions((current) => [...current, session]);
        setRecentFolders((current) => [session.workingDir, ...current.filter((folder) => folder.toLowerCase() !== session.workingDir.toLowerCase())].slice(0, 5));
        setDialogOpen(false);
      }} onError={(failure) => showError("ui.create-session", failure)} />}
      {closeRequest && <div className="modal-backdrop"><div className="modal confirm-modal">
        <div className="modal-heading"><div><p className="eyebrow">CLOSE SESSION TAB</p><h2>{closeRequest.name} is running</h2></div></div>
        <p className="muted">Stop its PowerShell and harness process, or keep it running in the background?</p>
        <div className="modal-actions">
          <button className="secondary-button" onClick={() => setCloseRequest(null)}>Cancel</button>
          <button className="secondary-button" onClick={() => { keepTerminalInBackground(closeRequest.id); setCloseRequest(null); }}>Keep running</button>
          <button className="primary-button" disabled={busy} onClick={() => void stopAndCloseTab(closeRequest)}>{busy ? "Stopping..." : "Stop and close"}</button>
        </div>
      </div></div>}
    </div>
  );
}

function Home({ sessions, status, busy, onNew, onLaunch }: {
  sessions: Session[];
  status: SystemStatus | null;
  busy: boolean;
  onNew: () => void;
  onLaunch: (session: Session, mode: "start" | "restore" | "fresh") => void;
}) {
  const [freshSession, setFreshSession] = useState<Session | null>(null);
  return (
    <>
    <div className="home">
      <div className="hero">
        <div><p className="eyebrow">LOCAL SESSION WORKSPACE</p><h1>Pick up where you left off.</h1><p className="muted">Keep your PowerShell AI harness sessions organized, resumable, and close at hand.</p></div>
        <button className="primary-button" onClick={onNew}>Create session</button>
      </div>
      <div className="section-title"><div><h2>Your sessions</h2><p className="muted">Sessions are never started automatically.</p></div></div>
      {sessions.length === 0 ? <div className="empty-card"><div className="empty-icon">+</div><h3>No sessions yet</h3><p>Create a session to connect a project folder to Claude Code or GitHub Copilot CLI.</p><button className="primary-button" onClick={onNew}>Create your first session</button></div> :
        <div className="session-grid">{sessions.map((session) => <article className="session-card" key={session.id}>
          <div className="card-top"><div className="tool-icon">{session.tool === "claude-code" ? "C" : "G"}</div><span className={`state-pill ${session.runningHere ? "running" : ""}`}>{session.runningHere ? "Running" : session.status}</span></div>
          <h3>{session.name}</h3><p className="path">{session.workingDir}</p><p className="session-meta">{session.id} | {session.tool}</p>
          {session.lastError && <p className="card-error">{session.lastError}</p>}
          <div className="card-actions">
            {session.runningHere ? <button className="primary-button" onClick={() => onLaunch(session, "start")}>Open terminal</button> :
              session.hasResumeId ? <><button className="primary-button" disabled={busy} onClick={() => onLaunch(session, "restore")}>Restore</button><button className="text-button" disabled={busy} onClick={() => setFreshSession(session)}>Start fresh</button></> :
                <button className="primary-button" disabled={busy} onClick={() => onLaunch(session, "start")}>Start</button>}
          </div>
        </article>)}</div>}
      <div className="tools-panel"><div><h2>Installed tools</h2><p className="muted">Detected on this machine</p></div><div className="tool-list">
        <ToolRow name="PowerShell 7" tool={status?.powershell} /><ToolRow name="Claude Code" tool={status?.claudeCode} /><ToolRow name="GitHub Copilot CLI" tool={status?.copilotCLI} />
      </div></div>
      {status?.diagnosticsLogPath && <p className="muted diagnostic-path">Diagnostic log: <code>{status.diagnosticsLogPath}</code></p>}
    </div>
    {freshSession && <div className="modal-backdrop"><div className="modal confirm-modal">
      <div className="modal-heading"><div><p className="eyebrow">START FRESH</p><h2>Begin a new harness conversation?</h2></div></div>
      <p className="muted">This replaces the saved resume ID for <strong>{freshSession.name}</strong>. Its previous harness conversation will no longer be restored by TauShell.</p>
      <div className="modal-actions"><button className="secondary-button" onClick={() => setFreshSession(null)}>Cancel</button><button className="primary-button" disabled={busy} onClick={() => { onLaunch(freshSession, "fresh"); setFreshSession(null); }}>Start fresh</button></div>
    </div></div>}
    </>
  );
}

function ToolRow({ name, tool }: { name: string; tool?: SystemStatus["powershell"] }) {
  const label = tool?.installed ? tool.version || "Installed" : tool ? "Not found" : "Checking...";
  return <div className="tool-row" title={tool?.error}><span className={`status-dot ${tool?.installed ? "online" : ""}`} /><strong>{name}</strong><span>{label}</span>{tool?.executablePath && <code className="tool-path">{tool.executablePath}</code>}</div>;
}

const terminalInstances = new Map<string, Terminal>();
const fitInstances = new Map<string, FitAddon>();
const terminalReadyWaiters = new Map<string, Set<(terminal: Terminal) => void>>();

function waitForTerminal(id: string): Promise<Terminal> {
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

function TerminalPane({ session, visible, busy, onLaunch, onError }: {
  session?: Session;
  visible: boolean;
  busy: boolean;
  onLaunch: (session: Session, mode: "start" | "restore" | "fresh") => void;
  onError: (error: unknown) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const runningRef = useRef(session?.runningHere ?? false);
  runningRef.current = session?.runningHere ?? false;
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
      terminal = new Terminal({ convertEol: false, cursorBlink: true, fontFamily: "Cascadia Code, Consolas, monospace", fontSize: 14, theme: { background: "#0c0e12", foreground: "#d8dce5", cursor: "#a4e1c0" } });
      terminal.loadAddon(fit);
      terminal.open(container.current);
      terminalInstances.set(session.id, terminal);
      terminal.onData((data) => { void app().WriteTerminal(session.id, data).catch((err) => onError(err)); });
      terminal.onBinary((data) => { void app().WriteTerminalBinary(session.id, btoa(data)).catch((err) => onError(err)); });
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
    const resize = () => {
      try {
        fit.fit();
        if (runningRef.current) void app().ResizeTerminal(session.id, terminal!.cols, terminal!.rows).catch((err) => onError(err));
      } catch (error) {
        reportFrontendFailure("ui.terminal-layout", error);
      }
    };
    const observer = new ResizeObserver(resize);
    observer.observe(container.current);
    resize();
    notifyTerminalReady(session.id, terminal);
    return () => {
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
        if (session.runningHere) void app().ResizeTerminal(session.id, terminal.cols, terminal.rows).catch((err) => onError(err));
      } catch (error) {
        reportFrontendFailure("ui.terminal-layout", error);
      }
    });
  }, [visible, session?.id, session?.runningHere]);
  if (!session) return <div className="terminal-missing">Session configuration is unavailable.</div>;
  return <div className="terminal-view"><div className="terminal-toolbar"><span className={`status-dot ${session.runningHere ? "online" : ""}`} />{session.id} | {session.tool} | {session.workingDir}<span className="terminal-status">{session.runningHere ? "Connected" : session.status || "Not started"}</span>{!session.runningHere && <button className="primary-button terminal-action" disabled={busy} onClick={() => onLaunch(session, session.hasResumeId ? "restore" : "start")}>{session.hasResumeId ? "Restore" : "Start"}</button>}</div><div className="terminal-container" ref={container} /></div>;
}

function CreateSession({ recentFolders, onClose, onCreated, onError }: {
  recentFolders: string[];
  onClose: () => void;
  onCreated: (session: Session) => void;
  onError: (error: unknown) => void;
}) {
  const [name, setName] = useState("");
  const [tool, setTool] = useState<Session["tool"]>("claude-code");
  const [workingDir, setWorkingDir] = useState("");
  const [launchArgs, setLaunchArgs] = useState("");
  const [saving, setSaving] = useState(false);
  const chooseDirectory = async () => {
    try { const path = await app().SelectWorkingDirectory(); if (path) setWorkingDir(path); } catch (err) { onError(err); }
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      const session = await app().CreateSession(name, tool, workingDir, launchArgs.split(/\r?\n/).map((arg) => arg.trim()).filter(Boolean));
      onCreated(session);
    } catch (err) { onError(err); } finally { setSaving(false); }
  };
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><div className="modal">
    <div className="modal-heading"><div><p className="eyebrow">NEW WORKSPACE</p><h2>Create a session</h2></div><button className="icon-button" onClick={onClose}>x</button></div>
    <form onSubmit={(event) => void submit(event)}>
      <label>Session name<input required autoFocus value={name} onChange={(event) => setName(event.target.value)} placeholder="e.g. API refactor" /></label>
      <label>AI harness<select value={tool} onChange={(event) => setTool(event.target.value as Session["tool"])}><option value="claude-code">Claude Code</option><option value="copilot-cli">GitHub Copilot CLI</option></select></label>
      <label>Project folder<div className="folder-row"><input required list="recent-project-folders" value={workingDir} onChange={(event) => setWorkingDir(event.target.value)} placeholder="Choose a local project folder" /><datalist id="recent-project-folders">{recentFolders.map((folder) => <option key={folder} value={folder} />)}</datalist><button type="button" className="secondary-button" onClick={() => void chooseDirectory()}>Browse</button></div></label>
      <label>Additional launch arguments <span className="optional">(optional, one per line)</span><textarea rows={3} value={launchArgs} onChange={(event) => setLaunchArgs(event.target.value)} placeholder="--model&#10;sonnet" /></label>
      <p className="form-note">A YAML session definition is saved locally. No session starts until you choose Start.</p>
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={saving}>{saving ? "Creating..." : "Create session"}</button></div>
    </form>
  </div></div>;
}

createRoot(document.getElementById("root")!).render(<AppErrorBoundary><App /></AppErrorBoundary>);
