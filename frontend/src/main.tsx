import { Component, DragEvent, ErrorInfo, KeyboardEvent, MouseEvent, ReactNode, useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import "@xterm/xterm/css/xterm.css";
import { app, DialogRequest, InstallResult, Session, SessionEvent, ShutdownProgress, SkillsView, SystemStatus, ToolStatus } from "./bridge";
import "./theme/tokens.css";
import "./style.css";
import { Activity, AttentionKind, attentionEdge, nextActivity, readHarnessSignal } from "./attention";
import { moveId } from "./order";
import { CommandPalette, PaletteItem } from "./harnessActions";
import { GearIcon, HomeIcon } from "./harnessIcons";
import { harnessName } from "./harnesses";
import appIcon from "./assets/appicon.png";
import { errorDetails, reportFrontendFailure, isStoppedSessionError } from "./errors";
import { AppDialog, ShutdownPopup, RenameDialog } from "./dialogs";
import { ProjectIcon, sessionColorStyle, SessionMenu, statusLabel, StateIcon } from "./sessionVisuals";
import { Home, SkillsActions, toolUsable, Setup } from "./home";
import { terminalInstances, lastInputAt, terminalTitles, echoWindowMs, fitInstances, waitForTerminal, sendToSession, interruptSession, TerminalPane } from "./terminalPane";
import { CreateSession } from "./createSession";

type Tab = { id: string; name: string };
const savedTabsKey = "taushell.tabs.v1";
const recentFoldersKey = "taushell.recent-folders.v1";

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
  const [selected, setSelected] = useState("");
  const [activity, setActivity] = useState<Record<string, Activity>>({});
  const activityRef = useRef<Record<string, Activity>>({});
  activityRef.current = activity;
  const workingSince = useRef(new Map<string, number>());
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
      const previous = activityRef.current;
      let changed = false;
      const next = { ...previous };
      const alerts: Array<{ id: string; kind: AttentionKind }> = [];
      sessionsRef.current.forEach((session) => {
        if (!session.runningHere) return;
        const id = session.id;
        const at = outputAt.current.get(id);
        const signal = readHarnessSignal(session.tool, terminalInstances.get(id), terminalTitles.get(id) ?? "");
        if (signal === undefined && at === undefined) return;
        const old = previous[id] ?? { working: false, input: false, unseen: false };
        const { working, input, unseen } = nextActivity(old, signal, at, now, selectedRef.current === id && watching);
        if (working && !old.working) workingSince.current.set(id, now);
        const alert = attentionEdge(old, { working, input }, now - (workingSince.current.get(id) ?? now));
        if (alert) alerts.push({ id, kind: alert });
        if (working !== old.working || input !== old.input || unseen !== old.unseen) {
          next[id] = { working, input, unseen };
          changed = true;
        }
      });
      if (changed) {
        activityRef.current = next;
        setActivity(next);
      }
      // Go decides whether to flash/toast (only while TauShell is in the background) and never sees terminal content.
      alerts.forEach(({ id, kind }) => app().NotifyAttention(id, kind).catch((err) => showError("ui.attention", err)));
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
  const [shutdown, setShutdown] = useState<ShutdownProgress | null>(null);
  const [renameRequest, setRenameRequest] = useState<Session | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<{ sessionId: string; message: string; isError: boolean } | null>(null);
  const [palette, setPalette] = useState<PaletteItem[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [installing, setInstalling] = useState("");
  const [toolNotice, setToolNotice] = useState<InstallResult | null>(null);
  const [toolUpdates, setToolUpdates] = useState<Record<string, string>>({});
  const [refreshing, setRefreshing] = useState(false);
  const [skillsView, setSkillsView] = useState<SkillsView | null>(null);
  const [skillsUpdating, setSkillsUpdating] = useState(false);
  const [setupDismissed, setSetupDismissed] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [autoResumeAll, setAutoResumeAll] = useState(false);
  const autoResumeRan = useRef(false);
  const showError = (source: string, failure: unknown) => setError(reportFrontendFailure(source, failure));

  // Once per app launch, resume the sessions flagged for it (or all, when the global flag is on).
  useEffect(() => {
    if (!loaded || autoResumeRan.current) return;
    autoResumeRan.current = true;
    void (async () => {
      let all = false;
      try {
        all = await app().GetAutoResumeAll();
        setAutoResumeAll(all);
      } catch (err) {
        showError("ui.auto-resume", err);
      }
      const targets = sessions.filter((session) => (all || session.autoResume) && !session.runningHere && session.hasResumeId && !session.resumeNote);
      for (const session of targets) await launch(session, "restore");
      if (targets.length > 0) setSelected("");
    })();
  }, [loaded]);

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
    } finally {
      setLoaded(true);
    }
  };

  useEffect(() => {
    void refreshAll();
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
        terminalInstances.get(event.sessionId)?.writeln(`\r\n[session ${statusLabel(event.session.status)}${event.session.lastError ? `: ${event.session.lastError}` : ""}]\r\n`);
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
    runtime.EventsOnMultiple("focus-session", (sessionId: string) => {
      const target = sessionsRef.current.find((session) => session.id === sessionId);
      if (target) openTab(target);
    }, -1);
    runtime.EventsOnMultiple("settings-error", (message: string) => showError("settings", new Error(message + " (using default settings)")), -1);
    runtime.EventsOnMultiple("shutdown-progress", (progress: ShutdownProgress) => setShutdown(progress.phase === "done" ? null : progress), -1);
    app().DialogHostReady().then((pending) => pending.forEach(add)).catch((err) => showError("ui.dialog-host", err));
    return () => { runtime.EventsOff("dialog-request"); runtime.EventsOff("focus-session"); runtime.EventsOff("settings-error"); runtime.EventsOff("shutdown-progress"); };
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
    const remainingTabs = orderedTabs.filter((tab) => tab.id !== id);
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
  const [dragId, setDragId] = useState("");
  const [dropMark, setDropMark] = useState<{ id: string; after: boolean } | null>(null);
  const dragProps = (id: string, horizontal = false) => ({
    draggable: true,
    onDragStart: (event: DragEvent<HTMLElement>) => { event.dataTransfer.effectAllowed = "move"; event.dataTransfer.setData("text/plain", id); setDragId(id); },
    onDragOver: (event: DragEvent<HTMLElement>) => {
      if (!dragId || dragId === id) return;
      event.preventDefault();
      const rect = event.currentTarget.getBoundingClientRect();
      const after = horizontal ? event.clientX > rect.left + rect.width / 2 : event.clientY > rect.top + rect.height / 2;
      setDropMark((current) => current?.id === id && current.after === after ? current : { id, after });
    },
    onDrop: (event: DragEvent<HTMLElement>) => {
      event.preventDefault();
      const mark = dropMark;
      setDragId("");
      setDropMark(null);
      if (!dragId || !mark || mark.id !== id) return;
      const ordered = moveId(sessions.map((session) => session.id), dragId, id, mark.after);
      if (ordered.join() === sessions.map((session) => session.id).join()) return;
      const previous = sessions;
      setSessions(ordered.map((sessionId) => previous.find((session) => session.id === sessionId)!));
      app().SetSessionOrder(ordered).catch((err) => { setSessions(previous); showError("ui.reorder-sessions", err); });
    },
    onDragEnd: () => { setDragId(""); setDropMark(null); },
  });
  const dropClass = (id: string) => dropMark?.id === id ? (dropMark.after ? "drop-after" : "drop-before") : "";
  const duplicateSession = async (session: Session) => {
    setBusy(true);
    try {
      const copy = await app().DuplicateSession(session.id);
      await refresh();
      void launch(copy, "start");
    } catch (err) {
      showError("ui.duplicate-session", err);
    } finally {
      setBusy(false);
    }
  };
  const colorOf = (id: string) => sessions.find((current) => current.id === id)?.color;
  // Tabs follow the same order as the sidebar and Home tiles, not the order they were opened.
  const orderedTabs = [...tabs].sort((a, b) => sessions.findIndex((s) => s.id === a.id) - sessions.findIndex((s) => s.id === b.id));

  const toolFor = (session: Session) => status?.tools.find((tool) => tool.id === session.tool);
  const harnessFor = (session: Session) => {
    const tool = toolFor(session);
    return session.runningHere && tool?.widgets ? { widgets: tool.widgets, modelSupported: Boolean(tool.capabilities?.some((capability) => capability.option === "--model" && capability.available)) } : undefined;
  };
  const sendFor = (session: Session) => (text: string, submit: boolean, label: string) =>
    sendToSession(session, text, submit, label,
      (message) => setNotice({ sessionId: session.id, message, isError: false }),
      (message) => setNotice({ sessionId: session.id, message, isError: true }));

  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(null), 6000);
    return () => window.clearTimeout(timer);
  }, [notice]);

  const openPalette = async () => {
    const items: PaletteItem[] = [
      { id: "app:new", group: "App", label: "New session", run: () => setDialogOpen(true) },
      { id: "app:home", group: "App", label: "Go to Home", run: () => setSelected("") },
      { id: "app:refresh", group: "App", label: "Refresh sessions and tools", run: () => void refreshAll() },
      { id: "app:settings", group: "App", label: "Edit settings...", hint: "notifications", run: () => void app().OpenSettingsFile().catch((err) => showError("ui.open-settings", err)) },
    ];
    sessions.forEach((session) => {
      items.push({ id: `open:${session.id}`, group: "Session", label: `Open ${session.name}`, hint: session.runningHere ? "running" : statusLabel(session.status), run: () => openTab(session) });
      if (!session.runningHere) {
        const mode = session.hasResumeId ? "restore" : "start";
        items.push({ id: `launch:${session.id}`, group: "Session", label: `${mode === "restore" ? "Resume" : "Start"} ${session.name}`, run: () => void launch(session, mode) });
      }
    });
    const active = sessions.find((session) => session.id === selected);
    const harness = active && harnessFor(active);
    if (active && harness) {
      const send = sendFor(active);
      try {
        (await app().GetPresets(active.id) ?? []).forEach((preset) =>
          items.push({ id: `preset:${active.id}:${preset.name}`, group: "Prompt", label: preset.name, hint: preset.text.slice(0, 60), run: () => send(preset.text, preset.submit, `"${preset.name}"`) }));
      } catch (err) {
        showError("ui.presets", err);
      }
      harness.widgets.commands.forEach((command) =>
        items.push({ id: `command:${active.tool}:${command.text}`, group: "Command", label: command.label, hint: command.text, run: () => send(command.text, true, command.text) }));
      if (harness.modelSupported) {
        items.push({ id: `model:${active.tool}:picker`, group: "Model", label: "Open the harness model picker", hint: "/model", run: () => send("/model", true, "/model") });
        harness.widgets.models.forEach((model) =>
          items.push({ id: `model:${active.tool}:${model}`, group: "Model", label: `Switch model to ${model}`, hint: `/model ${model}`, run: () => send(`/model ${model}`, true, `/model ${model}`) }));
      }
      items.push({ id: `interrupt:${active.tool}`, group: "Command", label: "Interrupt the harness", hint: "Esc", run: () => interruptSession(active, harness.widgets.interruptKeys) });
    }
    setPalette(items);
  };

  // Ctrl+1..9 selects the Nth open session tab, Ctrl+(Shift+)Tab cycles Home and tabs, Ctrl+Shift+P opens the palette.
  const shortcutState = useRef({ ids: [] as string[], selected: "", paletteOpen: false, openPalette });
  shortcutState.current = { ids: orderedTabs.map((tab) => tab.id), selected, paletteOpen: palette !== null, openPalette };
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (!event.ctrlKey || event.altKey || event.metaKey) return;
      const state = shortcutState.current;
      const otherModalOpen = Boolean(document.querySelector(".modal, [role='dialog']:not(.palette)"));
      if (event.shiftKey && event.key.toLowerCase() === "p") {
        event.preventDefault();
        if (state.paletteOpen) setPalette(null);
        else if (!otherModalOpen) void state.openPalette();
        return;
      }
      if (otherModalOpen || state.paletteOpen) return;
      if (!event.shiftKey && /^[1-9]$/.test(event.key)) {
        event.preventDefault();
        const id = state.ids[Number(event.key) - 1];
        if (id) setSelected(id);
      } else if (event.key === "Tab") {
        event.preventDefault();
        const cycle = ["", ...state.ids];
        const next = (cycle.indexOf(state.selected) + (event.shiftKey ? -1 : 1) + cycle.length) % cycle.length;
        setSelected(cycle[next]);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  const installTool = async (tool: ToolStatus) => {
    setInstalling(tool.id);
    setToolNotice(null);
    try {
      let result = await app().InstallTool(tool.id);
      if (result.outcome === "prerequisite-required" && result.prerequisiteId === "node") {
        const node = status?.tools.find((candidate) => candidate.id === "node");
        if (!node) throw new Error("Node.js / npm status was not returned by TauShell.");
        setInstalling(node.id);
        const nodeResult = await app().InstallTool(node.id);
        setToolNotice(nodeResult);
        if (nodeResult.outcome !== "installed") return;
        await refresh(true);
        setInstalling(tool.id);
        result = await app().InstallTool(tool.id);
      }
      setToolNotice(result);
      if (result.outcome === "installed" || result.outcome === "failed") await refresh(true);
    } catch (err) {
      showError("ui.install-tool", err);
    } finally {
      setInstalling("");
    }
  };
  const updateTool = async (tool: ToolStatus) => {
    setInstalling(tool.id);
    setToolNotice(null);
    try {
      const result = await app().UpdateTool(tool.id);
      setToolNotice(result);
      if (result.outcome === "installed" || result.outcome === "failed") {
        await refresh(true);
        setToolUpdates(await app().GetToolUpdates());
      }
    } catch (err) {
      showError("ui.update-tool", err);
    } finally {
      setInstalling("");
    }
  };
  useEffect(() => {
    let cancelled = false;
    const load = () => { void app().GetToolUpdates().then((value) => { if (!cancelled) setToolUpdates(value ?? {}); }).catch(() => undefined); };
    load();
    const timer = window.setInterval(load, 10 * 60 * 1000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, []);
  useEffect(() => {
    let cancelled = false;
    const load = () => { void app().GetSkills().then((value) => { if (!cancelled) setSkillsView(value); }).catch(() => undefined); };
    load();
    const timer = window.setInterval(load, 10 * 60 * 1000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, []);
  const updateSkills = async () => {
    setSkillsUpdating(true);
    try {
      setSkillsView(await app().UpdateSkills());
    } catch (err) {
      showError("ui.update-skills", err);
    } finally {
      setSkillsUpdating(false);
    }
  };
  const toggleSkill = (name: string, enabled: boolean) => {
    setSkillsView((view) => view && { ...view, skills: view.skills.map((skill) => skill.name === name ? { ...skill, enabled } : skill) });
    app().SetSkillEnabled(name, enabled).catch((err) => {
      setSkillsView((view) => view && { ...view, skills: view.skills.map((skill) => skill.name === name ? { ...skill, enabled: !enabled } : skill) });
      showError("ui.set-skill-enabled", err);
    });
  };
  const skillActions: SkillsActions = { view: skillsView, refreshing, updating: skillsUpdating, onUpdate: () => void updateSkills(), onToggle: toggleSkill };
  const refreshAll = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      await Promise.all([
        refresh(true),
        app().CheckToolUpdates().then((value) => setToolUpdates(value ?? {})).catch((err) => showError("ui.check-tool-updates", err)),
        app().CheckSkills().then(setSkillsView).catch((err) => showError("ui.check-skills", err)),
      ]);
    } catch (err) {
      showError("ui.refresh", err);
    } finally {
      setRefreshing(false);
    }
  };
  const openSetupPage = (tool: ToolStatus) => {
    app().OpenToolSetupPage(tool.id).catch((err) => showError("ui.open-setup-page", err));
  };
  const powershell = status?.tools.find((tool) => tool.id === "powershell");
  const showSetup = Boolean(powershell && !toolUsable(powershell) && !setupDismissed);
  const toolActions = { status, installing, notice: toolNotice, updates: toolUpdates, refreshing, onUpdate: (tool: ToolStatus) => void updateTool(tool), onInstall: (tool: ToolStatus) => void installTool(tool), onOpenSetup: openSetupPage, onDismissNotice: () => setToolNotice(null), onError: (err: unknown) => showError("ui.open-tool-config", err) };

  return (
    <div className="app">
      <div className="workspace">
        <aside className={`sidebar ${sidebarCollapsed ? "collapsed" : ""}`}>
          <div className="sidebar-brand"><img className="brand-mark" src={appIcon} alt="" /><span className="brand-text"><strong>TauShell</strong><small>Your AI sessions</small></span></div>
          <div className="sidebar-heading"><span className="sidebar-title">SESSIONS</span> <button className="add-button" title="New session" onClick={() => setDialogOpen(true)}>+</button></div>
          <button className={`nav-row ${selected ? "" : "active"}`} title="Home" aria-label="Home" onClick={() => setSelected("")}><HomeIcon /></button>
          {sessions.map((session) => (
            <div className={`session-item ${selected === session.id ? "active" : ""} ${dragId === session.id ? "dragging" : ""} ${dropClass(session.id)}`} key={session.id} {...dragProps(session.id)} style={sessionColorStyle(session.color)} data-colored={session.color ? "" : undefined} onContextMenu={(event) => openMenu(event, session)}>
              <button className={`session-row ${selected === session.id ? "active" : ""}`} title={session.name} onClick={() => openTab(session)}>
                <StateIcon session={session} activity={activity[session.id]} />
                <span className="collapsed-only"><ProjectIcon session={session} /></span>
                <span className="session-row-text"><strong>{session.name}</strong><small>{harnessName(session.tool)}</small></span>
              </button>
              <button className="delete-button" aria-label={`Delete ${session.name}`} title="Delete session" onClick={() => setDeleteRequest(session)}>x</button>
            </div>
          ))}
          <div className="sidebar-foot"><button className="sidebar-toggle" aria-label="TauShell settings" title="TauShell settings (opens config.yaml)" onClick={() => void app().OpenSettingsFile().catch((err) => showError("ui.open-settings", err))}><GearIcon /></button><button className="sidebar-toggle" aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"} title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"} onClick={() => setSidebarCollapsed((value) => !value)}>{sidebarCollapsed ? ">>" : "<<"}</button></div>
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
                aria-label="Home"
                title="Home"
                onClick={() => setSelected("")}
              ><HomeIcon /></button>
              {orderedTabs.map((tab) => (
                <div className={`tab-item ${selected === tab.id ? "active" : ""} ${dragId === tab.id ? "dragging" : ""} ${dropClass(tab.id)}`} key={tab.id} role="presentation" {...dragProps(tab.id, true)} style={sessionColorStyle(colorOf(tab.id))} data-colored={colorOf(tab.id) ? "" : undefined} onContextMenu={(event) => { const session = sessions.find((current) => current.id === tab.id); if (session) openMenu(event, session); }}>
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
          {notice && notice.sessionId === selected && <div className={notice.isError ? "error-banner" : "notice-banner"} role="status">{notice.message}<button onClick={() => setNotice(null)}>Dismiss</button></div>}
          <section className="main-content">
            <div id="panel-home" className={`home-container ${selected ? "hidden" : ""}`} role="tabpanel" aria-labelledby="tab-home" tabIndex={0}>
              {showSetup && powershell ? <Setup powershell={powershell} actions={toolActions} onRecheck={() => void refresh(true)} onContinue={() => setSetupDismissed(true)} /> :
                <Home sessions={sessions} loaded={loaded} autoResumeAll={autoResumeAll} onAutoResumeAll={(enabled) => { setAutoResumeAll(enabled); void app().SetAutoResumeAll(enabled).catch((err) => { setAutoResumeAll(!enabled); showError("ui.auto-resume-all", err); }); }} onAutoResume={(session, enabled) => void updateSession("ui.auto-resume", () => app().SetSessionAutoResume(session.id, enabled))} status={status} busy={busy} tools={toolActions} skills={skillActions} onNew={() => setDialogOpen(true)} onRefresh={() => void refreshAll()} onHome={() => setSelected("")} onLaunch={launch} onDelete={setDeleteRequest} onDuplicate={duplicateSession} dragProps={dragProps} dropClass={dropClass} dragId={dragId} />}
            </div>
            {terminalIds.map((id) => {
              const tab = tabs.find((current) => current.id === id);
              const session = sessions.find((current) => current.id === id);
              if (!tab && !session?.runningHere) return null;
              return (
                <div id={`panel-${id}`} className={`terminal-tab-content ${tab && selected === id ? "" : "hidden"}`} role="tabpanel" aria-labelledby={`tab-${id}`} tabIndex={0} key={id}>
                  <TerminalPane session={session} visible={Boolean(tab && selected === id)} busy={busy} harness={session ? harnessFor(session) : undefined} onSend={session ? sendFor(session) : undefined} onInterrupt={() => { const harness = session && harnessFor(session); if (session && harness) interruptSession(session, harness.widgets.interruptKeys); }} onLaunch={launch} onError={(failure) => showError("ui.terminal", failure)} />
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
        onDuplicate={() => { const target = menu.session; setMenu(null); void duplicateSession(target); }}
        onRename={() => { setRenameRequest(menu.session); setMenu(null); }}
        onColor={(color) => { const id = menu.session.id; setMenu(null); void updateSession("ui.set-session-color", () => app().SetSessionColor(id, color)); }} />}
      {renameRequest && <RenameDialog session={renameRequest} onClose={() => setRenameRequest(null)}
        onRename={(name) => { const id = renameRequest.id; setRenameRequest(null); void updateSession("ui.rename-session", () => app().RenameSession(id, name)); }} />}
      {deleteRequest && <div className="modal-backdrop"><div className="modal confirm-modal">
        <div className="modal-heading"><div><p className="eyebrow">DELETE SESSION</p><h2>Delete {deleteRequest.name}?</h2></div></div>
        {deleteRequest.runningHere ?
          <p className="muted">This session is running. Close its tab and stop it before deleting.</p> :
          <div className="muted">
            <p><strong>Warning: this cannot be undone.</strong></p>
            <p>Will be removed from TauShell:</p>
            <ul>
              <li>the session definition file (YAML), including its name, color, arguments and presets</li>
              <li>the saved runtime state{deleteRequest.hasResumeId && ", so its harness conversation can no longer be resumed from TauShell"}</li>
              <li>this session's own token and cost total (past usage still counts in the monthly totals)</li>
            </ul>
            <p>Not touched: your project folder, and the harness's own conversation history files.</p>
          </div>}
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
      {shutdown && <ShutdownPopup progress={shutdown} />}
      {palette && <CommandPalette items={palette} onClose={() => setPalette(null)} />}
    </div>
  );
}

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

createRoot(document.getElementById("root")!).render(<AppErrorBoundary><App /></AppErrorBoundary>);
