import { Component, CSSProperties, ErrorInfo, FormEvent, KeyboardEvent, MouseEvent, ReactNode, useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { app, DialogRequest, InstallResult, LaunchPreview, Session, SessionEvent, SystemStatus, ToolStatus } from "./bridge";
import "./style.css";
import { readHarnessSignal } from "./attention";

type Tab = { id: string; name: string };
const savedTabsKey = "taushell.tabs.v1";
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

// A resize can race with session stop; that is not a user-facing failure.
function isStoppedSessionError(error: unknown): boolean {
  return errorDetails(error).message.includes("is not running");
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
  const [selected, setSelected] = useState("");
  const [activity, setActivity] = useState<Record<string, Activity>>({});
  const selectedRef = useRef("");
  selectedRef.current = selected;
  const outputAt = useRef(new Map<string, number>());
  const sessionsRef = useRef<Session[]>([]);
  sessionsRef.current = sessions;
  useEffect(() => {
    // Prefers the harness's own cue (title / status row); falls back to output timing.
    const tick = () => {
      const now = Date.now();
      const watching = document.hasFocus();
      setActivity((previous) => {
        let changed = false;
        const next = { ...previous };
        sessionsRef.current.forEach((session) => {
          if (!session.runningHere) return;
          const id = session.id;
          const at = outputAt.current.get(id);
          const signal = readHarnessSignal(session.tool, terminalInstances.get(id), terminalTitles.get(id) ?? "");
          if (signal === undefined && at === undefined) return;
          const old = previous[id] ?? { working: false, input: false, unseen: false };
          const working = signal ? signal === "working" : now - at! < workingWindowMs;
          const input = signal === "input";
          const seen = selectedRef.current === id && watching;
          let unseen = old.unseen;
          if (((old.working && !working) || (input && !old.input)) && !seen) unseen = true;
          if (seen) unseen = false;
          if (working !== old.working || input !== old.input || unseen !== old.unseen) {
            next[id] = { working, input, unseen };
            changed = true;
          }
        });
        return changed ? next : previous;
      });
    };
    const timer = window.setInterval(tick, 500);
    window.addEventListener("focus", tick);
    return () => { window.clearInterval(timer); window.removeEventListener("focus", tick); };
  }, []);
  useEffect(() => {
    setActivity((previous) => selected && previous[selected]?.unseen ? { ...previous, [selected]: { ...previous[selected], unseen: false } } : previous);
  }, [selected]);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => localStorage.getItem("taushell.sidebar-collapsed.v1") === "1");
  useEffect(() => {
    localStorage.setItem("taushell.sidebar-collapsed.v1", sidebarCollapsed ? "1" : "0");
  }, [sidebarCollapsed]);
  const [tabScroll, setTabScroll] = useState({ left: false, right: false });
  const tabListRef = useRef<HTMLDivElement>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [closeRequest, setCloseRequest] = useState<Session | null>(null);
  const [deleteRequest, setDeleteRequest] = useState<Session | null>(null);
  const [menu, setMenu] = useState<{ x: number; y: number; session: Session } | null>(null);
  const [dialogs, setDialogs] = useState<DialogRequest[]>([]);
  const [renameRequest, setRenameRequest] = useState<Session | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [installing, setInstalling] = useState("");
  const [toolNotice, setToolNotice] = useState<InstallResult | null>(null);
  const [setupDismissed, setSetupDismissed] = useState(false);
  const showError = (source: string, failure: unknown) => setError(reportFrontendFailure(source, failure));

  const refresh = async (redetectTools = false) => {
    try {
      const [nextSessions, nextStatus] = await Promise.all([
        app().GetSessions(),
        status && !redetectTools ? Promise.resolve(status) : app().GetSystemStatus(),
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
          outputAt.current.delete(event.sessionId);
          terminalTitles.delete(event.sessionId);
          setActivity((current) => { const { [event.sessionId]: _removed, ...rest } = current; return rest; });
          setBackgroundTerminalIds((current) => current.filter((id) => id !== event.sessionId));
        }
        terminalInstances.get(event.sessionId)?.writeln(`\r\n[session ${event.session.status}${event.session.lastError ? `: ${event.session.lastError}` : ""}]\r\n`);
      } else if (event.kind === "output" && event.dataBase64) {
        if (Date.now() - (lastInputAt.get(event.sessionId) ?? 0) > echoWindowMs) outputAt.current.set(event.sessionId, Date.now());
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
  }, [tabs]);

  // Go-side confirmations (shutdown, installs) render here instead of as native message boxes.
  useEffect(() => {
    const runtime = window.runtime;
    if (!runtime) return;
    const add = (request: DialogRequest) => setDialogs((current) => current.some((item) => item.id === request.id) ? current : [...current, request]);
    runtime.EventsOnMultiple("dialog-request", add, -1);
    app().DialogHostReady().then((pending) => pending.forEach(add)).catch((err) => showError("ui.dialog-host", err));
    return () => runtime.EventsOff("dialog-request");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const answerDialog = (request: DialogRequest, choice: string) => {
    setDialogs((current) => current.filter((item) => item.id !== request.id));
    app().AnswerDialog(request.id, choice).catch((err) => showError("ui.dialog-answer", err));
  };

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
      await fitTerminalNow(session.id);
      const started = mode === "restore"
        ? await app().RestoreSession(session.id, terminal.cols, terminal.rows)
        : mode === "fresh"
          ? await app().StartFresh(session.id, terminal.cols, terminal.rows)
          : await app().StartSession(session.id, terminal.cols, terminal.rows);
      openTab(started);
      // A resize sent while the harness is still initializing can be lost, so repeat it once it is up.
      for (const delay of [500, 2000]) {
        window.setTimeout(() => {
          const current = terminalInstances.get(started.id);
          if (current) void app().ResizeTerminal(started.id, current.cols, current.rows).catch((err) => { if (!isStoppedSessionError(err)) showError("ui.terminal-layout", err); });
        }, delay);
      }
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
  const deleteSession = async (session: Session) => {
    setBusy(true);
    try {
      await app().DeleteSession(session.id);
      removeTab(session.id);
      setDeleteRequest(null);
      await refresh();
    } catch (err) {
      setDeleteRequest(null);
      showError("ui.delete-session", err);
    } finally {
      setBusy(false);
    }
  };
  const terminalIds = Array.from(new Set([...tabs.map((tab) => tab.id), ...backgroundTerminalIds]));
  const openMenu = (event: MouseEvent, session: Session) => {
    event.preventDefault();
    setMenu({ x: event.clientX, y: event.clientY, session });
  };
  const updateSession = async (source: string, update: () => Promise<void>) => {
    try {
      await update();
      await refresh();
    } catch (err) {
      showError(source, err);
    }
  };
  const colorOf = (id: string) => sessions.find((current) => current.id === id)?.color;
  const installTool = async (tool: ToolStatus) => {
    setInstalling(tool.id);
    setToolNotice(null);
    try {
      const result = await app().InstallTool(tool.id);
      setToolNotice(result);
      if (result.outcome === "installed" || result.outcome === "failed") await refresh(true);
    } catch (err) {
      showError("ui.install-tool", err);
    } finally {
      setInstalling("");
    }
  };
  const openSetupPage = (tool: ToolStatus) => {
    app().OpenToolSetupPage(tool.id).catch((err) => showError("ui.open-setup-page", err));
  };
  const powershell = status?.tools.find((tool) => tool.id === "powershell");
  const showSetup = Boolean(powershell && !toolUsable(powershell) && !setupDismissed);
  const toolActions = { status, installing, notice: toolNotice, onInstall: (tool: ToolStatus) => void installTool(tool), onOpenSetup: openSetupPage, onDismissNotice: () => setToolNotice(null) };

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand"><span className="brand-mark">T</span><span>TauShell</span></div>
        <div className="window-caption">Local AI sessions</div>
        <button className="icon-button" title="Refresh sessions and re-detect tools" onClick={() => void refresh(true)}>R</button>
      </header>
      <div className="workspace">
        <aside className={`sidebar ${sidebarCollapsed ? "collapsed" : ""}`}>
          <div className="sidebar-heading"><span className="sidebar-title">SESSIONS</span> <button className="add-button" title="Create session" onClick={() => setDialogOpen(true)}>+</button></div>
          <button className={`nav-row ${selected ? "" : "active"}`} title="Home" onClick={() => setSelected("")}>Home</button>
          {sessions.map((session) => (
            <div className={`session-item ${selected === session.id ? "active" : ""}`} key={session.id} style={sessionColorStyle(session.color)} data-colored={session.color ? "" : undefined} onContextMenu={(event) => openMenu(event, session)}>
              <button className={`session-row ${selected === session.id ? "active" : ""}`} title={session.name} onClick={() => openTab(session)}>
                <StateIcon session={session} activity={activity[session.id]} />
                <span className="session-row-text"><strong>{session.name}</strong><small>{session.tool === "claude-code" ? "Claude Code" : "GitHub Copilot CLI"}</small></span>
              </button>
              <button className="delete-button" aria-label={`Delete ${session.name}`} title="Delete session" onClick={() => setDeleteRequest(session)}>x</button>
            </div>
          ))}
          <div className="sidebar-foot"><span className="sidebar-foot-text">Local machine only</span><button className="sidebar-toggle" aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"} title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"} onClick={() => setSidebarCollapsed((value) => !value)}>{sidebarCollapsed ? ">>" : "<<"}</button></div>
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
                <div className={`tab-item ${selected === tab.id ? "active" : ""}`} key={tab.id} role="presentation" style={sessionColorStyle(colorOf(tab.id))} data-colored={colorOf(tab.id) ? "" : undefined} onContextMenu={(event) => { const session = sessions.find((current) => current.id === tab.id); if (session) openMenu(event, session); }}>
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
                  >{sessions.find((current) => current.id === tab.id) && <StateIcon session={sessions.find((current) => current.id === tab.id)!} activity={activity[tab.id]} />}{tab.name}</button>
                  <button className="tab-close" aria-label={`Close ${tab.name}`} onClick={() => closeTab(tab.id)}>x</button>
                </div>
              ))}
            </div>
            <button className="tab-scroll-button" aria-label="Scroll tabs right" disabled={!tabScroll.right} onClick={() => scrollTabs(1)}>&gt;</button>
          </div>
          {error && <div className="error-banner">{error}<button onClick={() => setError("")}>Dismiss</button></div>}
          <section className="main-content">
            <div id="panel-home" className={`home-container ${selected ? "hidden" : ""}`} role="tabpanel" aria-labelledby="tab-home" tabIndex={0}>
              {showSetup && powershell ? <Setup powershell={powershell} actions={toolActions} onRecheck={() => void refresh(true)} onContinue={() => setSetupDismissed(true)} /> :
                <Home sessions={sessions} status={status} busy={busy} tools={toolActions} onNew={() => setDialogOpen(true)} onLaunch={launch} onDelete={setDeleteRequest} />}
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
        void launch(session, "start");
      }} onError={(failure) => showError("ui.create-session", failure)} />}
      {menu && <SessionMenu x={menu.x} y={menu.y} session={menu.session} onClose={() => setMenu(null)}
        onRename={() => { setRenameRequest(menu.session); setMenu(null); }}
        onColor={(color) => { const id = menu.session.id; setMenu(null); void updateSession("ui.set-session-color", () => app().SetSessionColor(id, color)); }} />}
      {renameRequest && <RenameDialog session={renameRequest} onClose={() => setRenameRequest(null)}
        onRename={(name) => { const id = renameRequest.id; setRenameRequest(null); void updateSession("ui.rename-session", () => app().RenameSession(id, name)); }} />}
      {deleteRequest && <div className="modal-backdrop"><div className="modal confirm-modal">
        <div className="modal-heading"><div><p className="eyebrow">DELETE SESSION</p><h2>Delete {deleteRequest.name}?</h2></div></div>
        {deleteRequest.runningHere ?
          <p className="muted">This session is running. Close its tab and stop it before deleting.</p> :
          <p className="muted">This permanently removes the session definition and its saved resume ID from TauShell. {deleteRequest.hasResumeId && "Its harness conversation can no longer be restored by TauShell. "}Your project folder and the harness's own data are not touched. This cannot be undone.</p>}
        <div className="modal-actions">
          <button className="secondary-button" onClick={() => setDeleteRequest(null)}>Cancel</button>
          <button className="danger-button" disabled={busy || deleteRequest.runningHere} onClick={() => void deleteSession(deleteRequest)}>{busy ? "Deleting..." : "Delete session"}</button>
        </div>
      </div></div>}
      {closeRequest && <div className="modal-backdrop"><div className="modal confirm-modal">
        <div className="modal-heading"><div><p className="eyebrow">CLOSE SESSION TAB</p><h2>{closeRequest.name} is running</h2></div></div>
        <p className="muted">Stop its PowerShell and harness process, or keep it running in the background?</p>
        <div className="modal-actions">
          <button className="secondary-button" onClick={() => setCloseRequest(null)}>Cancel</button>
          <button className="secondary-button" onClick={() => { keepTerminalInBackground(closeRequest.id); setCloseRequest(null); }}>Keep running</button>
          <button className="primary-button" disabled={busy} onClick={() => void stopAndCloseTab(closeRequest)}>{busy ? "Stopping..." : "Stop and close"}</button>
        </div>
      </div></div>}
      {dialogs[0] && <AppDialog request={dialogs[0]} onAnswer={(choice) => answerDialog(dialogs[0], choice)} />}
    </div>
  );
}

const dialogLabels: Record<DialogRequest["kind"], string> = { question: "CONFIRM", warning: "WARNING", error: "PROBLEM", info: "NOTICE" };

function AppDialog({ request, onAnswer }: { request: DialogRequest; onAnswer: (choice: string) => void }) {
  // Escape is the dialog's cancel button; a single-button dialog just closes with it.
  const cancel = request.cancelButton || (request.buttons.length === 1 ? request.buttons[0] : "");
  useEffect(() => {
    if (!cancel) return;
    const onKey = (event: globalThis.KeyboardEvent) => { if (event.key === "Escape") onAnswer(cancel); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [request.id, cancel]);
  const initial = request.buttons.includes(request.defaultButton) ? request.defaultButton : request.buttons[0];
  return <div className="modal-backdrop" role="alertdialog" aria-modal="true" aria-labelledby="app-dialog-title"><div className="modal confirm-modal">
    <div className="modal-heading"><div><p className="eyebrow">{dialogLabels[request.kind] ?? "NOTICE"}</p><h2 id="app-dialog-title">{request.title}</h2></div></div>
    <p className="muted dialog-message">{request.message}</p>
    <div className="modal-actions">{request.buttons.map((button) =>
      <button key={button} autoFocus={button === initial} className={button === initial ? "primary-button" : "secondary-button"} onClick={() => onAnswer(button)}>{button}</button>)}</div>
  </div></div>;
}

const sessionColors = [
  { name: "Red", value: "#e5534b" },
  { name: "Orange", value: "#d9822b" },
  { name: "Yellow", value: "#c9a227" },
  { name: "Green", value: "#4c9f58" },
  { name: "Teal", value: "#2fa4a4" },
  { name: "Blue", value: "#4f8cff" },
  { name: "Purple", value: "#9b73e6" },
  { name: "Pink", value: "#d65a9b" },
];

function sessionColorStyle(color?: string): CSSProperties | undefined {
  return color ? { "--session-color": color } as CSSProperties : undefined;
}

function SessionMenu({ x, y, session, onClose, onRename, onColor }: {
  x: number;
  y: number;
  session: Session;
  onClose: () => void;
  onRename: () => void;
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
    <div className="context-menu-label">Color</div>
    <div className="color-swatches">
      {sessionColors.map((color) => <button key={color.value} role="menuitemradio" aria-checked={session.color === color.value} className={`color-swatch ${session.color === color.value ? "selected" : ""}`} style={{ background: color.value }} title={color.name} aria-label={color.name} onClick={() => onColor(color.value)} />)}
    </div>
    <button role="menuitem" className="context-menu-item" disabled={!session.color} onClick={() => onColor("")}>Default color</button>
  </div>;
}

function RenameDialog({ session, onClose, onRename }: { session: Session; onClose: () => void; onRename: (name: string) => void }) {
  const [name, setName] = useState(session.name);
  const trimmed = name.trim();
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><div className="modal confirm-modal">
    <div className="modal-heading"><div><p className="eyebrow">RENAME SESSION</p><h2>{session.name}</h2></div></div>
    <form onSubmit={(event) => { event.preventDefault(); if (trimmed) onRename(trimmed); }}>
      <label>Session name<input autoFocus maxLength={80} value={name} onFocus={(event) => event.target.select()} onChange={(event) => setName(event.target.value)} /></label>
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={!trimmed || trimmed === session.name}>Rename</button></div>
    </form>
  </div></div>;
}

function Home({ sessions, status, busy, tools, onNew, onLaunch, onDelete }: {
  sessions: Session[];
  status: SystemStatus | null;
  busy: boolean;
  tools: ToolActions;
  onNew: () => void;
  onLaunch: (session: Session, mode: "start" | "restore" | "fresh") => void;
  onDelete: (session: Session) => void;
}) {
  const [freshSession, setFreshSession] = useState<Session | null>(null);
  return (
    <>
    <div className="home">
      <div className="hero">
        <div><p className="eyebrow">LOCAL SESSION WORKSPACE</p><h1>Pick up where you left off.</h1><p className="muted">Keep your PowerShell AI harness sessions organized, resumable, and close at hand.</p></div>
        <button className="primary-button" onClick={onNew}>Create session</button>
      </div>
      <div className="section-title"><div><h2>Your sessions</h2><p className="muted">Existing sessions are never started automatically.</p></div></div>
      {sessions.length === 0 ? <div className="empty-card"><div className="empty-icon">+</div><h3>No sessions yet</h3><p>Create a session to connect a project folder to Claude Code or GitHub Copilot CLI.</p><button className="primary-button" onClick={onNew}>Create your first session</button></div> :
        <div className="session-grid">{sessions.map((session) => <article className="session-card" key={session.id}>
          <div className="card-top"><div className="tool-icon">{session.tool === "claude-code" ? "C" : "G"}</div><span className={`state-pill ${session.runningHere ? "running" : ""}`}>{session.runningHere ? "Running" : session.status}</span><button className="delete-button" aria-label={`Delete ${session.name}`} title="Delete session" onClick={() => onDelete(session)}>x</button></div>
          <h3>{session.name}</h3><p className="path">{session.workingDir}</p><p className="session-meta">{session.id} | {session.tool}</p>
          {session.lastError && <p className="card-error">{session.lastError}</p>}
          <div className="card-actions">
            {session.runningHere ? <button className="primary-button" onClick={() => onLaunch(session, "start")}>Open terminal</button> :
              session.hasResumeId ? <>{!session.resumeNote && <button className="primary-button" disabled={busy} onClick={() => onLaunch(session, "restore")}>Restore</button>}<button className="text-button" disabled={busy} title={session.resumeNote} onClick={() => setFreshSession(session)}>Start fresh</button></> :
                <button className="primary-button" disabled={busy} onClick={() => onLaunch(session, "start")}>Start</button>}
          </div>
        </article>)}</div>}
      <div className="tools-panel"><div><h2>Installed tools</h2><p className="muted">Detected on this machine{status && !status.wingetAvailable && " \u2014 winget is unavailable; Install uses vendor setup links instead"}</p></div><div className="tool-list">
        {status?.tools.map((tool) => <ToolRow key={tool.id} tool={tool} actions={tools} />) ?? <p className="muted">Checking...</p>}
      </div></div>
      {toolNoticeBanner(tools)}
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

type ToolActions = {
  status: SystemStatus | null;
  installing: string;
  notice: InstallResult | null;
  onInstall: (tool: ToolStatus) => void;
  onOpenSetup: (tool: ToolStatus) => void;
  onDismissNotice: () => void;
};

function toolUsable(tool: ToolStatus): boolean {
  return tool.installed && !tool.error && tool.support !== "below-minimum" &&
    (tool.capabilities ?? []).every((capability) => capability.available || capability.optional);
}

function toolNoticeBanner(tools: ToolActions) {
  if (!tools.notice) return null;
  const tone = tools.notice.outcome === "installed" ? "notice-success" : tools.notice.outcome === "failed" ? "notice-error" : "notice-info";
  return <div className={`tool-notice ${tone}`}>
    <span>{tools.notice.message}</span>
    {tools.notice.vendorUrl && <a href={tools.notice.vendorUrl} target="_blank" rel="noreferrer">Open vendor setup page</a>}
    <button className="icon-button" onClick={tools.onDismissNotice}>x</button>
  </div>;
}

function Setup({ powershell, actions, onRecheck, onContinue }: {
  powershell: ToolStatus;
  actions: ToolActions;
  onRecheck: () => void;
  onContinue: () => void;
}) {
  return <div className="home setup-screen">
    <div className="hero"><div><p className="eyebrow">FIRST-RUN SETUP</p><h1>PowerShell 7 is required.</h1>
      <p className="muted">TauShell hosts every session through PowerShell 7. Install it, then re-check to continue.</p></div></div>
    <div className="tools-panel"><div className="tool-list"><ToolRow tool={powershell} actions={actions} /></div></div>
    {toolNoticeBanner(actions)}
    <div className="modal-actions"><button className="secondary-button" onClick={onRecheck}>Re-check</button>
      <button className="text-button" onClick={onContinue}>Continue without PowerShell 7</button></div>
  </div>;
}

function ToolRow({ tool, actions }: { tool: ToolStatus; actions: ToolActions }) {
  const usable = toolUsable(tool);
  const label = tool.installed ? tool.version || "Installed" : "Not found";
  const supportNote = tool.installed && tool.support === "below-minimum" ? `below minimum ${tool.minimum}` :
    tool.installed && tool.support === "untested" ? `untested (tested: ${tool.testedRange})` : "";
  const unavailableCapabilities = (tool.capabilities ?? []).filter((capability) => !capability.available && !capability.optional);
  const installing = actions.installing === tool.id;
  const title = [tool.error, ...(tool.warnings ?? [])].filter(Boolean).join("\n");
  return <div className="tool-row" title={title || undefined}>
    <span className={`status-dot ${usable ? "online" : tool.installed ? "warn" : ""}`} />
    <strong>{tool.name}</strong>
    <span>{label}{supportNote && <span className="tool-support-note"> ({supportNote})</span>}</span>
    {tool.executablePath && <code className="tool-path">{tool.executablePath}</code>}
    {unavailableCapabilities.length > 0 && <span className="tool-warning">missing: {unavailableCapabilities.map((capability) => capability.option).join(", ")}</span>}
    {!tool.installed && <button className="secondary-button tool-action" disabled={installing} onClick={() => actions.onInstall(tool)}>
      {installing ? "Installing..." : tool.install.wingetId && actions.status?.wingetAvailable ? "Install" : "Setup instructions"}
    </button>}
    {!tool.installed && <button className="text-button" onClick={() => actions.onOpenSetup(tool)}>Vendor page</button>}
  </div>;
}

const terminalInstances = new Map<string, Terminal>();
const lastInputAt = new Map<string, number>();
const terminalTitles = new Map<string, string>();
// Output within this window after a keystroke is treated as echo, not harness work.
const echoWindowMs = 400;
// Output newer than this means the session is "working"; older means "quiet".
const workingWindowMs = 2500;

type ActivityKind = "working" | "input" | "quiet" | "stopped" | "failed";
type Activity = { working: boolean; input: boolean; unseen: boolean };

function stateIconKind(session: Session, activity?: Activity): { kind: ActivityKind; unseen: boolean } {
  if (!session.runningHere) return { kind: session.status === "failed" ? "failed" : "stopped", unseen: false };
  const kind = activity?.input ? "input" : activity?.working ? "working" : "quiet";
  return { kind, unseen: !!activity?.unseen };
}

const stateLabels: Record<ActivityKind, string> = { working: "Working (output active)", input: "Waiting for your answer", quiet: "Quiet (output stopped)", stopped: "Not running", failed: "Failed" };

function StateIcon({ session, activity }: { session: Session; activity?: Activity }) {
  const { kind, unseen } = stateIconKind(session, activity);
  const label = kind === "input" ? stateLabels.input : unseen ? "Quiet since you last looked (may need input)" : stateLabels[kind];
  return <span className={`state-icon ${kind} ${unseen ? "unseen" : ""}`} title={label} aria-label={label} role="img" />;
}
const fitInstances = new Map<string, FitAddon>();
const terminalReadyWaiters = new Map<string, Set<(terminal: Terminal) => void>>();

// xterm reports a 0x0 cell until it first renders, so fit() right after open() is a silent no-op and cols/rows stay 80x24.
async function fitTerminalNow(id: string) {
  await document.fonts.ready;
  for (let attempt = 0; attempt < 20; attempt++) {
    await new Promise((resolve) => requestAnimationFrame(resolve));
    const fit = fitInstances.get(id);
    if (!fit) return;
    if (fit.proposeDimensions()) {
      fit.fit();
      return;
    }
  }
}

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

// Windows Terminal convention: Ctrl+C copies only when text is selected (otherwise it is ^C
// for the harness); Ctrl+V and Ctrl+Shift+C/V always paste/copy. Uses the native clipboard
// because WebView2's navigator.clipboard needs a permission prompt.
function handleClipboardKey(event: globalThis.KeyboardEvent, terminal: Terminal, onError: (error: unknown) => void): boolean {
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
      terminal = new Terminal({ convertEol: false, cursorBlink: true, scrollback: 0, fontFamily: "Cascadia Code, Consolas, monospace", fontSize: 14, theme: { background: "#0c0e12", foreground: "#d8dce5", cursor: "#a4e1c0" } });
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
  return <div className="terminal-view"><div className="terminal-toolbar"><span className={`status-dot ${session.runningHere ? "online" : ""}`} />{session.id} | {session.tool} | {session.workingDir}<span className="terminal-status">{session.runningHere ? "Connected" : session.status || "Not started"}</span>{!session.runningHere && (session.hasResumeId && session.resumeNote ? <button className="text-button terminal-action" disabled={busy} title={session.resumeNote} onClick={() => onLaunch(session, "fresh")}>Start fresh</button> : <button className="primary-button terminal-action" disabled={busy} onClick={() => onLaunch(session, session.hasResumeId ? "restore" : "start")}>{session.hasResumeId ? "Restore" : "Start"}</button>)}</div><div className="terminal-container" ref={container} /></div>;
}

function parseLaunchArgs(text: string): string[] {
  return text.split(/\r?\n/).map((arg) => arg.trim()).filter(Boolean);
}

function CreateSession({ recentFolders, onClose, onCreated, onError }: {
  recentFolders: string[];
  onClose: () => void;
  onCreated: (session: Session) => void;
  onError: (error: unknown) => void;
}) {
  const [tool, setTool] = useState<Session["tool"]>("claude-code");
  const [workingDir, setWorkingDir] = useState("");
  const [launchArgs, setLaunchArgs] = useState("");
  const [templateText, setTemplateText] = useState<string | null>(null);
  const [preview, setPreview] = useState<LaunchPreview | null>(null);
  const [previewError, setPreviewError] = useState("");
  const [saving, setSaving] = useState(false);
  const [yamlOpen, setYamlOpen] = useState(false);
  const [yamlText, setYamlText] = useState("");
  const [yamlDirty, setYamlDirty] = useState(false);
  const [yamlError, setYamlError] = useState("");
  const chooseDirectory = async () => {
    try { const path = await app().SelectWorkingDirectory(); if (path) setWorkingDir(path); } catch (err) { onError(err); }
  };
  // Prefill with the harness-specific template on first load and whenever
  // the user switches tool while the field still matches the prior
  // template verbatim (i.e. they have not customized it yet).
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const template = await app().GetSessionTemplate(tool);
        if (cancelled) return;
        const text = template.launchArgs.join("\n");
        setLaunchArgs((current) => (templateText === null || current === templateText ? text : current));
        setTemplateText(text);
      } catch (err) { if (!cancelled) onError(err); }
    })();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tool]);
  // Preview the effective executable and discrete argument list before
  // saving, including any launch argument TauShell cannot verify against
  // the installed harness's own --help output.
  useEffect(() => {
    let cancelled = false;
    const timer = setTimeout(() => {
      (async () => {
        try {
          const result = await app().PreviewSessionLaunch(tool, parseLaunchArgs(launchArgs));
          if (!cancelled) { setPreview(result); setPreviewError(""); }
        } catch (err) {
          if (!cancelled) { setPreview(null); setPreviewError(err instanceof Error ? err.message : String(err)); }
        }
      })();
    }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [tool, launchArgs]);
  // The YAML view follows the form until the user edits it by hand; after
  // that their text is kept until they choose "Reset from form".
  useEffect(() => {
    if (!yamlOpen || yamlDirty) return;
    let cancelled = false;
    const timer = setTimeout(() => {
      (async () => {
        try {
          const text = await app().PreviewSessionYAML("", tool, workingDir, parseLaunchArgs(launchArgs));
          if (!cancelled) { setYamlText(text); setYamlError(""); }
        } catch (err) {
          if (!cancelled) setYamlError(err instanceof Error ? err.message : String(err));
        }
      })();
    }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [yamlOpen, yamlDirty, tool, workingDir, launchArgs]);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      const session = yamlOpen
        ? await app().CreateSessionFromYAML(yamlText)
        : await app().CreateSession("", tool, workingDir, parseLaunchArgs(launchArgs));
      onCreated(session);
    } catch (err) { onError(err); } finally { setSaving(false); }
  };
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><div className="modal">
    <div className="modal-heading"><div><p className="eyebrow">NEW WORKSPACE</p><h2>Create a session</h2></div><button className="icon-button" onClick={onClose}>x</button></div>
    <form onSubmit={(event) => void submit(event)}>
      <label>Project folder<div className="folder-row"><input autoFocus required list="recent-project-folders" value={workingDir} onChange={(event) => setWorkingDir(event.target.value)} placeholder="Choose a local project folder" /><datalist id="recent-project-folders">{recentFolders.map((folder) => <option key={folder} value={folder} />)}</datalist><button type="button" className="secondary-button" onClick={() => void chooseDirectory()}>Browse</button></div></label>
      <label>AI harness<select value={tool} onChange={(event) => setTool(event.target.value as Session["tool"])}><option value="claude-code">Claude Code</option><option value="copilot-cli">GitHub Copilot CLI</option></select></label>
      <label>Launch arguments <span className="optional">(editable template, one per line)</span><textarea rows={3} value={launchArgs} onChange={(event) => setLaunchArgs(event.target.value)} placeholder="--yolo" /></label>
      <div className="launch-preview">
        {previewError ? <p className="tool-warning">{previewError}</p> : preview && <>
          <p className="muted">Will run:</p>
          <code className="launch-command">{[preview.executable, ...preview.arguments].join(" ")}</code>
          {preview.warnings.map((warning) => <p key={warning} className="tool-warning">{warning}</p>)}
        </>}
      </div>
      <div className="yaml-section">
        <button type="button" className="text-button" aria-expanded={yamlOpen} onClick={() => setYamlOpen((open) => !open)}>{yamlOpen ? "Hide session YAML" : "Edit session YAML"}</button>
        {yamlOpen && <>
          <textarea className="yaml-editor" rows={9} spellCheck={false} aria-label="Session YAML" value={yamlText} onChange={(event) => { setYamlText(event.target.value); setYamlDirty(true); }} />
          {yamlError && <p className="tool-warning">{yamlError}</p>}
          <p className="form-note">{yamlDirty ? "Edited by hand: the form fields above no longer change this text. " : "Follows the form until you edit it. "}The definition is validated and saved in normal form.{yamlDirty && <> <button type="button" className="text-button" onClick={() => setYamlDirty(false)}>Reset from form</button></>}</p>
        </>}
      </div>
      <p className="form-note">A YAML session definition is saved locally and the session starts immediately. The session is named after the project; right-click it to rename or recolor.</p>
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={saving}>{saving ? "Creating..." : "Create session"}</button></div>
    </form>
  </div></div>;
}


createRoot(document.getElementById("root")!).render(<AppErrorBoundary><App /></AppErrorBoundary>);
