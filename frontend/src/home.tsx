import { useEffect, useRef, useState } from "react";
import { app, AppUpdate, CodexQuota, CopilotQuota, InstallResult, Session, SkillsView, SystemStatus, ToolStatus, WSLAction, WSLStatus } from "./bridge";
import { UsageSummary, hasUsage, monthTooltip, usageText } from "./usage";
import { GearIcon, HarnessIcon, RuntimeIcon, ToolIcon } from "./harnessIcons";
import { Icon } from "./icons";
import { usePlaces } from "./sessionTree";
import { getHarnesses, harnessIds, harnessShort, isHarness } from "./harnesses";
import { UserHarnesses } from "./userHarnesses";
import { ProjectIcon, sessionColorStyle, statusLabel } from "./sessionVisuals";
import { clearPollFailure, shouldReportPollFailure } from "./polling";
import { ControlAudit } from "./controlAudit";
import { PhonePane } from "./phonePane";
import { SecretsPane } from "./secretsPane";
import { Workspace, sortRecent, ungrouped } from "./workspaces";
import { runtimeLabel } from "./wslRuntime";
import { claudeBudgetKey, groupedKey, readClaudeBudget, readGrouped, readSortRecent, sortRecentKey, writeStored } from "./storage";
import { ClaudeQuota, claudeQuotaView, formatDollars, parseBudgetInput } from "./claudeQuota";
import { UpdateDialog } from "./updateDialog";
import { SessionNotes } from "./sessionNotesView";
import { GroupControls, GroupHeader } from "./workspaceView";
import { AppearancePane } from "./appearancePane";
import { HomePanel, HomePanelId, PanelFeedback, PanelFeedbackState } from "./homePanel";
export function Home({ sessions, workspaces, groupControls, loaded, autoResumeAll, onAutoResumeAll, onAutoResume, status, busy, tools, skills, onNew, onRefresh, onHome, onLaunch, onDelete, onDuplicate, onError, panelFeedback = {}, onDismissPanelFeedback, onPanelFeedback, dragProps, dropClass, dragId }: {
  workspaces: Workspace[];
  groupControls: GroupControls;
  skills: SkillsActions;
  autoResumeAll: boolean;
  onAutoResumeAll: (enabled: boolean) => void;
  onAutoResume: (session: Session, enabled: boolean) => void;
  onDuplicate: (session: Session) => void;
  onError: (source: string, error: unknown) => void;
  panelFeedback?: Partial<Record<HomePanelId, PanelFeedbackState>>;
  onDismissPanelFeedback?: (panel: HomePanelId) => void;
  onPanelFeedback?: (panel: HomePanelId, feedback?: PanelFeedbackState) => void;
  dragProps: (id: string) => object;
  dropClass: (id: string) => string;
  dragId: string;
  sessions: Session[];
  loaded: boolean;
  status: SystemStatus | null;
  busy: boolean;
  tools: ToolActions;
  onNew: () => void;
  onRefresh: () => void;
  onHome: () => void;
  onLaunch: (session: Session, mode: "start" | "restore" | "fresh") => void | Promise<void>;
  onDelete: (session: Session) => void;
}) {
  const places = usePlaces(sessions.map((session) => session.id), true);
  const [freshSession, setFreshSession] = useState<Session | null>(null);
  const [grouped, setGrouped] = useState(() => readGrouped(localStorage));
  const changeGrouped = (value: boolean) => {
    setGrouped(value);
    writeStored(groupedKey, value, onError);
  };
  const [sortedRecent, setSortedRecent] = useState(() => readSortRecent(localStorage));
  const changeSortedRecent = (value: boolean) => {
    setSortedRecent(value);
    writeStored(sortRecentKey, value, onError);
  };
  const [summary, setSummary] = useState<UsageSummary | null>(null);
  const pollFailures = useRef(new Set<string>());
  useEffect(() => {
    let cancelled = false;
    let loading = false;
    const load = () => {
      if (loading) return;
      loading = true;
      void app().GetUsageSummary().then((value) => {
        if (cancelled) return;
        clearPollFailure(pollFailures.current, "usage-summary");
        setSummary(value);
      }).catch((error) => {
        if (!cancelled && shouldReportPollFailure(pollFailures.current, "usage-summary")) onError("ui.poll-usage-summary", error);
      }).finally(() => { loading = false; });
    };
    load();
    const timer = window.setInterval(load, 15000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, []);
  const [quota, setQuota] = useState<CopilotQuota | null>(null);
  useEffect(() => {
    let cancelled = false;
    let loading = false;
    const load = () => {
      if (loading) return;
      loading = true;
      void app().GetCopilotQuota(false).then((value) => {
        if (cancelled) return;
        clearPollFailure(pollFailures.current, "copilot-quota");
        setQuota(value ?? null);
      }).catch((error) => {
        if (!cancelled && shouldReportPollFailure(pollFailures.current, "copilot-quota")) onError("ui.poll-copilot-quota", error);
      }).finally(() => { loading = false; });
    };
    load();
    const timer = window.setInterval(load, 300000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, []);
  const [codexQuota, setCodexQuota] = useState<CodexQuota | null>(null);
  useEffect(() => {
    let cancelled = false;
    let loading = false;
    const load = () => {
      if (loading) return;
      loading = true;
      void app().GetCodexQuota(false).then((value) => {
        if (cancelled) return;
        clearPollFailure(pollFailures.current, "codex-quota");
        setCodexQuota(value ?? null);
      }).catch((error) => {
        if (!cancelled && shouldReportPollFailure(pollFailures.current, "codex-quota")) onError("ui.poll-codex-quota", error);
      }).finally(() => { loading = false; });
    };
    load();
    const timer = window.setInterval(load, 300000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, []);
  const [claudeQuota, setClaudeQuota] = useState<ClaudeQuota | null>(null);
  const [claudeBudget, setClaudeBudget] = useState<number | null>(() => readClaudeBudget(localStorage));
  const [budgetDraft, setBudgetDraft] = useState<string | null>(null);
  const commitBudget = (text: string) => {
    const value = parseBudgetInput(text);
    setClaudeBudget(value);
    writeStored(claudeBudgetKey, value, onError);
    setBudgetDraft(null);
  };
  useEffect(() => {
    let cancelled = false;
    let loading = false;
    const load = () => {
      if (loading) return;
      loading = true;
      void app().GetClaudeQuota(false).then((value) => {
        if (cancelled) return;
        clearPollFailure(pollFailures.current, "claude-quota");
        setClaudeQuota(value ?? null);
      }).catch((error) => {
        if (!cancelled && shouldReportPollFailure(pollFailures.current, "claude-quota")) onError("ui.poll-claude-quota", error);
      }).finally(() => { loading = false; });
    };
    load();
    const timer = window.setInterval(load, 300000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, []);
  const [resumeProgress, setResumeProgress] = useState("");
  const [appVersion, setAppVersion] = useState("");
  const [appUpdate, setAppUpdate] = useState<AppUpdate | null>(null);
  const [updateOpen, setUpdateOpen] = useState(false);
  const [rollbackVersion, setRollbackVersion] = useState("");
  useEffect(() => { void app().GetRollbackVersion().then(setRollbackVersion).catch(() => setRollbackVersion("")); }, []);
  useEffect(() => {
    let cancelled = false;
    void app().GetAppVersion().then((value) => { if (!cancelled) { clearPollFailure(pollFailures.current, "app-version"); setAppVersion(value); } }).catch((error) => { if (!cancelled && shouldReportPollFailure(pollFailures.current, "app-version")) onError("ui.poll-app-version", error); });
    void app().CheckAppUpdate().then((value) => { if (!cancelled) { clearPollFailure(pollFailures.current, "app-update"); setAppUpdate(value); } }).catch((error) => { if (!cancelled && shouldReportPollFailure(pollFailures.current, "app-update")) onError("ui.poll-app-update", error); });
    return () => { cancelled = true; };
  }, []);
  const resumable = sessions.filter((session) => !session.runningHere && session.hasResumeId && !session.resumeNote);
  const resumeAll = async () => {
    for (let index = 0; index < resumable.length; index++) {
      setResumeProgress(`Resuming ${index + 1}/${resumable.length}: ${resumable[index].name}`);
      await onLaunch(resumable[index], "restore");
    }
    setResumeProgress("");
    onHome();
  };
  return (
    <>
    <div className="home">
      <div className="hero">
        <div><h1>Pick up where you left off.</h1><p className="muted">Keep your PowerShell AI harness sessions organized, resumable, and close at hand.</p></div>
        <div className="hero-actions">
          {resumeProgress && <span className="muted">{resumeProgress}</span>}
          <label className="check-option" title="Resume every stopped session that has a saved harness session when TauShell starts"><input type="checkbox" checked={autoResumeAll} onChange={(event) => onAutoResumeAll(event.target.checked)} />Auto-resume all on startup</label>
          <button className="secondary-button" disabled={busy || resumable.length === 0} title={resumable.length ? `Resume ${resumable.length} stopped session(s) that have a saved harness session` : "No stopped sessions with a saved harness session"} onClick={() => void resumeAll()}>Resume All</button>
          <button className="primary-button"           onClick={onNew}>New session</button>
        </div>
      </div>
      <div className="section-title"><div><h2>Your sessions</h2></div>{!loaded && <span className="spinner" role="status" aria-label="Loading sessions" />}</div>
      {quota && !quota.unlimited && <p className={`home-usage home-quota${quota.canProject && quota.projected > quota.entitlement ? " warn" : ""}`} title={`${quota.account ? `Copilot account: ${quota.account}. ` : ""}Plan: ${quota.plan}. Read through your GitHub CLI sign-in. ${quota.daysLeft} days left in the cycle.`}>Copilot{quota.account ? ` (${quota.account})` : ""}: <b className="kv">{Math.round(quota.used)} / {Math.round(quota.entitlement)} credits ({Math.round(quota.percentUsed)}%)</b>, resets {quota.resetDate}{quota.canProject ? <>, on pace for <b className="kv">~{Math.round(quota.projected)}</b></> : ""}</p>}
      {codexQuota && <p className="home-usage home-quota" title="Codex plan windows reported by the local Codex app-server. Separate from TauShell token and cost estimates.">Codex{codexQuota.plan ? ` (${codexQuota.plan})` : ""}: <b className="kv">{codexQuota.windows.map((window) => `${window.label}: ${Math.round(100 - window.usedPercent)}% remaining${window.resetAt ? `, resets ${new Date(window.resetAt).toLocaleString()}` : ""}`).join(" | ")}</b></p>}
      {claudeQuota && (() => {
        const view = claudeQuotaView(claudeQuota, claudeBudget);
        const source = view.overridden ? "set by you" : claudeQuota.budgetSource === "team" ? "team budget from the proxy" : claudeQuota.budgetSource === "member" ? "your budget from the proxy" : "no limit reported by the proxy";
        return <p className={`home-usage home-quota${view.overBudget ? " warn" : ""}`} title={`Claude Code through the ${claudeQuota.host} LiteLLM proxy. Spend is estimated by the proxy for the current budget cycle${claudeQuota.daysLeft ? `, ${claudeQuota.daysLeft} days left` : ""}. Budget: ${source}.${claudeQuota.teamBudget > 0 ? ` Team${claudeQuota.teamName ? ` ${claudeQuota.teamName}` : ""}: ${formatDollars(claudeQuota.teamSpend)} of ${formatDollars(claudeQuota.teamBudget)}.` : ""}`}>Claude ({claudeQuota.host}): <b className="kv">{formatDollars(claudeQuota.spend)}{view.budget > 0 ? ` / ${formatDollars(view.budget)} (${Math.round(view.percent)}%)` : ""}</b>{claudeQuota.resetAt ? `, resets ${new Date(claudeQuota.resetAt).toLocaleDateString()}` : ""}{claudeQuota.canProject ? <>, on pace for <b className="kv">~{formatDollars(claudeQuota.projected)}</b></> : ""}{claudeQuota.teamBudget > 0 ? `, team ${formatDollars(claudeQuota.teamSpend)} / ${formatDollars(claudeQuota.teamBudget)}` : ""}
          <label className="quota-budget" title="Monthly budget in USD. Defaults to the value the proxy reports; type your own to override it, clear the field to go back."> budget $<input inputMode="decimal" aria-label="Claude monthly budget in USD" value={budgetDraft ?? (view.budget > 0 ? String(view.budget) : "")} placeholder="none" onChange={(event) => setBudgetDraft(event.target.value)} onBlur={() => { if (budgetDraft !== null) commitBudget(budgetDraft); }} onKeyDown={(event) => { if (event.key === "Enter") commitBudget(event.currentTarget.value); if (event.key === "Escape") setBudgetDraft(null); }} /></label>
          {view.overridden && <button className="quota-reset" title={`Use the proxy value${claudeQuota.budget > 0 ? ` (${formatDollars(claudeQuota.budget)})` : ""}`} onClick={() => commitBudget("")}>reset</button>}</p>;
      })()}
      {summary && hasUsage(summary.month) && <p className="home-usage" title={monthTooltip(summary)}>This month: <b className="kv">{usageText(summary.month)}</b>{harnessIds().map((tool) => hasUsage(summary.monthByTool?.[tool]) ? <span key={tool}> | {harnessShort(tool)} <b className="kv">{usageText(summary.monthByTool![tool])}</b></span> : null)}</p>}
      <div className="home-view-bar" role="toolbar" aria-label="Session list view">
        <button className="secondary-button view-button" aria-pressed={grouped} title="Group sessions by workspace, or list them all together" onClick={() => changeGrouped(!grouped)}><Icon name="groupBy" size={14} />Group by workspace</button>
        <button className="secondary-button view-button" aria-pressed={sortedRecent} title={sortedRecent ? "Recent first (running, then last active). Click for your saved order" : "Your saved order. Click to list recently active sessions first"} onClick={() => changeSortedRecent(!sortedRecent)}><Icon name="sortRecent" size={14} />{sortedRecent ? "Sort: Recent first" : "Sort: Default"}</button>
      </div>
      {sessions.length === 0 ? (!loaded ? <div className="loading-card" role="status"><span className="spinner" aria-hidden="true" /><span className="muted">Loading sessions...</span></div> : <div className="empty-card"><div className="empty-icon">+</div><h3>No sessions yet</h3><p>Create a session to connect a project folder to an AI harness such as Claude Code or GitHub Copilot CLI.</p>      <button className="primary-button" onClick={onNew}>New session</button></div>) :
        <>{(() => { const base = grouped ? workspaces : [{ group: ungrouped, sessions }]; return sortedRecent ? sortRecent(base) : base; })().map((workspace) => <div className="home-group" key={workspace.group.key}>
        {grouped && <GroupHeader workspace={workspace} controls={groupControls} variant="home" />}
        {(!grouped || !groupControls.collapsed.has(workspace.group.key)) && <div className="session-grid">{workspace.sessions.map((session) => <article className={`session-card ${session.runningHere ? "running" : ""} ${dragId === session.id ? "dragging" : ""} ${dropClass(session.id)}`} key={session.id} style={sessionColorStyle(session.color)} data-colored={session.color ? "" : undefined} {...(sortedRecent ? {} : dragProps(session.id))}>
          <div className="card-top"><ProjectIcon session={session} /><span className={`state-pill ${session.runningHere ? "running" : ""} ${!session.runningHere && session.status === "failed" ? "status-warn-text" : ""}`}>{session.runningHere ? "Running" : statusLabel(session.status)}</span><span className="card-top-actions"><button className="icon-button card-file-button" aria-label={`Edit the definition of ${session.name}`} title="Edit session YAML (changes apply the next time it starts)" onClick={() => app().OpenSessionFile(session.id).catch((error) => onError("ui.open-session-file", error))}><Icon name="sessionFile" size={16} /></button><button className="delete-button" aria-label={`Delete ${session.name}`} title="Delete session" onClick={() => onDelete(session)}>x</button></span></div>
          <div className="card-title"><h3>{session.name}</h3>{hasUsage(summary?.sessions?.[session.id]) && <span className="card-usage" title="Estimate from harness logs; never reset">{usageText(summary!.sessions![session.id])}</span>}</div><p className="path path-line"><span className="path-text">{session.workingDir}</span>{places[session.id]?.label && <span className="path-place" title={places[session.id].worktree ? "Git worktree" : "Git branch"}>[<Icon name={places[session.id].worktree ? "worktree" : "branch"} size={11} />{places[session.id].label}]</span>}</p><p className="session-meta"><span className="harness-tag"><HarnessIcon tool={session.tool} />{session.tool}</span><span className="runtime-badge" title={session.runtime === "wsl" ? `Runs inside WSL distro ${session.distro}` : "Runs in PowerShell 7 on Windows"}><RuntimeIcon kind={session.runtime === "wsl" ? "wsl" : "powershell"} />{runtimeLabel(session)}</span>{session.worktree && <span className="runtime-badge" title={`Works in the git worktree ${session.worktree}`}>worktree</span>}</p>
          {session.lastError && <p className="card-error">{session.lastError}</p>}
          <SessionNotes session={session} onOpen={(url) => app().OpenSessionLink(session.id, url).catch((error) => onError("ui.open-session-link", error))} />
          <div className="card-sizer" aria-hidden="true"><span className="tile-button">Start fresh</span><span className="tile-button">Resume</span><span className="tile-button">Clone</span><span className="check-option"><input type="checkbox" tabIndex={-1} />Auto resume</span></div>
          <div className="card-actions">
            {session.runningHere ? <button className="tile-button" onClick={() => onLaunch(session, "start")}>Open</button> :
              session.hasResumeId ? <><button className="tile-button" disabled={busy} title={session.resumeNote} onClick={() => setFreshSession(session)}>Start fresh</button>{!session.resumeNote && <button className="tile-button" disabled={busy} onClick={() => onLaunch(session, "restore")}>Resume</button>}</> :
                <button className="tile-button" disabled={busy} onClick={() => onLaunch(session, "start")}>Start</button>}
            <button className="tile-button" disabled={busy} title="Copy this session definition and start the copy" onClick={() => onDuplicate(session)}>Clone</button>
            <label className="check-option" title={autoResumeAll ? "All sessions auto-resume (global setting is on)" : "Resume this session when TauShell starts"}><input type="checkbox" checked={session.autoResume || autoResumeAll} disabled={autoResumeAll} onChange={(event) => onAutoResume(session, event.target.checked)} />Auto resume</label>
          </div>
        </article>)}</div>}</div>)}</>}
      <PanelFeedback feedback={panelFeedback.sessions} onDismiss={() => onDismissPanelFeedback?.("sessions")} />
      <HomePanel title="Tools" actions={<div className="refresh-group">{tools.refreshing && <span className="spinner" role="status" aria-label="Refreshing" />}<button className="secondary-button" disabled={tools.refreshing} title="Reload sessions, re-detect tools and check for tool updates" onClick={onRefresh}>Refresh</button></div>}
        feedback={toolNoticeFeedback(tools, "tools", panelFeedback.tools)} onDismissFeedback={() => dismissToolFeedback(tools, "tools", onDismissPanelFeedback)}
        onFeedbackAction={() => openToolNoticeAction(tools, "tools")}>
        <div className="tool-list">
        {status?.tools.filter((tool) => !isHarness(tool.id)).map((tool) => <ToolRow key={tool.id} tool={tool} actions={tools} />) ?? <div className="loading-card" role="status"><span className="spinner" aria-hidden="true" /><span className="muted">Detecting tools...</span></div>}
        <WSLRow actions={tools} />
        </div>
      </HomePanel>
      <HomePanel title="AI Harnesses" feedback={toolNoticeFeedback(tools, "harnesses", panelFeedback.harnesses)} onDismissFeedback={() => dismissToolFeedback(tools, "harnesses", onDismissPanelFeedback)}
        onFeedbackAction={() => openToolNoticeAction(tools, "harnesses")}>
        <div className="tool-list">
        {status?.tools.filter((tool) => isHarness(tool.id))
          .sort((left, right) => getHarnesses().findIndex(({ id }) => id === left.id) - getHarnesses().findIndex(({ id }) => id === right.id))
          .map((tool) => <ToolRow key={tool.id} tool={tool} actions={tools} />) ?? <div className="loading-card" role="status"><span className="spinner" aria-hidden="true" /><span className="muted">Detecting harnesses...</span></div>}
        </div><UserHarnesses onError={onError} onFeedback={(feedback) => onPanelFeedback?.("harnesses", feedback)} />
      </HomePanel>
      <SkillsPanel skills={skills} unsupportedNote={skills.view?.unsupported} feedback={panelFeedback.skills} onDismissFeedback={() => onDismissPanelFeedback?.("skills")} />
      <PhonePane onError={onError} feedback={panelFeedback.phone} onFeedback={(feedback) => onPanelFeedback?.("phone", feedback)} onDismissFeedback={() => onDismissPanelFeedback?.("phone")} />
      <ControlAudit onError={onError} feedback={panelFeedback.control} onFeedback={(feedback) => onPanelFeedback?.("control", feedback)} onDismissFeedback={() => onDismissPanelFeedback?.("control")} />
      <SecretsPane onError={onError} feedback={panelFeedback.secrets} onFeedback={(feedback) => onPanelFeedback?.("secrets", feedback)} onDismissFeedback={() => onDismissPanelFeedback?.("secrets")} />
      <AppearancePane onError={onError} />
      <div className="home-footer">
        {appUpdate && <button className="link-button home-update" title="View the release and update TauShell" onClick={() => setUpdateOpen(true)}>{appUpdate.tag} available</button>}
        {rollbackVersion && <button className="link-button home-update" title="Restore the previous TauShell version and restart" onClick={() => void app().RollbackAppUpdate().catch((error) => onError("ui.rollback-update", error))}>Roll back to v{rollbackVersion}</button>}
        {appVersion && <span className="home-version" title="TauShell version">{appVersion === "dev" ? "dev build" : `v${appVersion.replace(/^v/, "")}`}</span>}
      </div>
    </div>
    {updateOpen && appUpdate && <UpdateDialog update={appUpdate} onClose={() => setUpdateOpen(false)} onError={onError} />}
    {freshSession && <div className="modal-backdrop"><div className="modal confirm-modal">
      <div className="modal-heading"><div><p className="eyebrow">START FRESH</p><h2>Begin a new harness conversation?</h2></div></div>
      <p className="muted">This replaces the saved resume ID for <strong>{freshSession.name}</strong>. Its previous harness conversation will no longer be restored by TauShell.</p>
      <div className="modal-actions"><button className="secondary-button" onClick={() => setFreshSession(null)}>Cancel</button><button className="primary-button" disabled={busy} onClick={() => { onLaunch(freshSession, "fresh"); setFreshSession(null); }}>Start fresh</button></div>
    </div></div>}
    </>
  );
}

export type SkillsActions = {
  view: SkillsView | null;
  refreshing: boolean;
  updating: boolean;
  saving: boolean;
  onUpdate: () => void;
  onToggle: (name: string, enabled: boolean) => void;
};

const toolLabel = harnessShort;

function SkillsPanel({ skills, unsupportedNote, feedback, onDismissFeedback }: {
  skills: SkillsActions;
  unsupportedNote?: string[];
  feedback?: PanelFeedbackState;
  onDismissFeedback?: () => void;
}) {
  const view = skills.view;
  if (view && !view.status.enabled && !view.status.message) return null;
  const status = view?.status;
  const origin = status?.enabled ? (status.source === "cookbook" ? `ai-cookbook @${status.applied}` : `bundled @${status.applied || "snapshot"}`) : "";
  const warnings = [...(status?.warnings ?? []), ...(unsupportedNote?.length ? [`Skills are not supported by the installed ${unsupportedNote.map(toolLabel).join(", ")} version.`] : [])];
  const localFeedback = skills.updating || skills.saving || !view
    ? { tone: "progress" as const, message: skills.saving ? "Saving skill preferences..." : "Updating skills..." }
    : warnings.length ? { tone: "warning" as const, message: warnings.join(" ") }
      : status?.message ? { tone: status.enabled ? "success" as const : "warning" as const, message: status.message } : undefined;
  return <HomePanel className="skills-panel" title="Skills" description={!view ? "Loading skills..." : status?.enabled ? `${origin}. Applies to new or resumed sessions.` : "Skills are unavailable."}
    actions={<div className="refresh-group">
        {(skills.updating || skills.saving || !view) && <span className="spinner" role="status" aria-label={skills.saving ? "Saving skill preferences" : "Updating skills"} />}
        {status?.updateAvailable && <button className="secondary-button" disabled={skills.updating || skills.refreshing || skills.saving} title={`Download and use ai-cookbook commit ${status.latest}`} onClick={skills.onUpdate}>Update skills ({status.latest})</button>}
      </div>} feedback={feedback ?? localFeedback} onDismissFeedback={feedback ? onDismissFeedback : undefined}>
    <div className="skill-list">
      {view?.skills.map((skill) => <label className="skill-row" key={skill.name} title={skill.description}>
        <input type="checkbox" checked={skill.enabled} disabled={skills.saving} onChange={(event) => skills.onToggle(skill.name, event.target.checked)} />
        <strong>{skill.name}</strong>
        <span className="skill-source">{skill.personal?.length ? `${skill.personal.map(toolLabel).join(", ")}: your copy is used` : skill.source}</span>
      </label>)}
    </div>
  </HomePanel>;
}

type ToolActions = {
  status: SystemStatus | null;
  wsl: WSLStatus | null;
  wslBusy: boolean;
  onWSLChange: (action: WSLAction, distro?: string) => void;
  installing: string;
  notice: InstallResult | null;
  noticeToolId?: string;
  updates: Record<string, string>;
  refreshing: boolean;
  onUpdate: (tool: ToolStatus) => void;
  onInstall: (tool: ToolStatus) => void;
  onOpenSetup: (tool: ToolStatus) => void;
  onDismissNotice: () => void;
  onError: (tool: ToolStatus, error: unknown) => void;
};

export function toolUsable(tool: ToolStatus): boolean {
  return tool.installed && !tool.error && tool.support !== "below-minimum" &&
    (tool.capabilities ?? []).every((capability) => capability.available || capability.optional);
}

function toolNoticeFeedback(tools: ToolActions, panel: "tools" | "harnesses", fallback?: PanelFeedbackState): PanelFeedbackState | undefined {
  const activeTool = tools.status?.tools.find((tool) => tool.id === tools.installing);
  if (activeTool && (isHarness(activeTool.id) ? "harnesses" : "tools") === panel) {
    return { tone: "progress", message: `Installing or updating ${activeTool.name}...` };
  }
  if (!tools.notice) return fallback;
  const target = tools.noticeToolId && isHarness(tools.noticeToolId) ? "harnesses" : "tools";
  if (target !== panel) return fallback;
  const tone = tools.notice.outcome === "installed" ? "success" : tools.notice.outcome === "failed" ? "error" : "warning";
  const vendorTool = tools.notice.vendorUrl ? tools.status?.tools.find((tool) => tool.install.vendorUrl === tools.notice?.vendorUrl) : undefined;
  return {
    tone,
    message: tools.notice.message,
    ...(vendorTool ? { actionLabel: "Open vendor setup page", actionToolId: vendorTool.id } : {}),
  };
}

function dismissToolFeedback(tools: ToolActions, panel: "tools" | "harnesses", dismiss?: (panel: HomePanelId) => void) {
  const target = tools.noticeToolId && isHarness(tools.noticeToolId) ? "harnesses" : "tools";
  if (tools.notice && target === panel) tools.onDismissNotice();
  else dismiss?.(panel);
}

function openToolNoticeAction(tools: ToolActions, panel: "tools" | "harnesses") {
  const feedback = toolNoticeFeedback(tools, panel);
  const tool = feedback?.actionToolId ? tools.status?.tools.find((item) => item.id === feedback.actionToolId) : undefined;
  if (tool) tools.onOpenSetup(tool);
}

export function Setup({ powershell, actions, onRecheck, onContinue, feedback, onDismissFeedback }: {
  powershell: ToolStatus;
  actions: ToolActions;
  onRecheck: () => void;
  onContinue: () => void;
  feedback?: PanelFeedbackState;
  onDismissFeedback?: () => void;
}) {
  return <div className="home setup-screen">
    <div className="hero"><div><p className="eyebrow">FIRST-RUN SETUP</p><h1>PowerShell 7 or WSL is required.</h1>
      <p className="muted">TauShell hosts sessions through PowerShell 7, or inside a WSL distribution if you prefer. Install either, then re-check to continue.</p></div></div>
    <HomePanel feedback={toolNoticeFeedback(actions, "tools", feedback)} onDismissFeedback={() => dismissToolFeedback(actions, "tools", onDismissFeedback ? () => onDismissFeedback() : undefined)}
      onFeedbackAction={() => openToolNoticeAction(actions, "tools")}>
      <div className="tool-list"><ToolRow tool={powershell} actions={actions} /><WSLRow actions={actions} /></div>
    </HomePanel>
    <div className="modal-actions"><button className="secondary-button" onClick={onRecheck}>Re-check</button>
      <button className="text-button" onClick={onContinue}>Continue without PowerShell 7 or WSL</button></div>
  </div>;
}

// Mirrors the Go installer: winget when a package is declared, otherwise npm.
function canAutoInstall(tool: ToolStatus, status: SystemStatus | null): boolean {
  if (tool.install.managedStrategy) return true;
  if (tool.install.wingetId) return !!status?.wingetAvailable;
  return !!tool.install.npmPackage;
}

function installHint(tool: ToolStatus, status: SystemStatus | null): string {
  if (tool.install.managedStrategy === "node-lts") return "Install the current Node.js LTS ZIP into TauShell's per-user tools folder; no administrator rights or PATH registry changes.";
  if (tool.install.managedStrategy === "portable-git") return "Install full Git for Windows PortableGit into TauShell's per-user tools folder; no administrator rights or PATH registry changes.";
  if (tool.install.npmPackage && !status?.npmAvailable) return `Install ${tool.name} with npm? TauShell will first offer a separate Node.js LTS install.`;
  if (canAutoInstall(tool, status)) {
    return `Install ${tool.name} with ${tool.install.wingetId ? "winget" : "npm"} (asks first)`;
  }
  return `winget was not found. Open the vendor setup instructions for ${tool.name}.`;
}

// WSL is optional. Install and update actions only start wsl.exe after a native confirmation; TauShell never elevates itself or restarts Windows.
function WSLRow({ actions }: { actions: ToolActions }) {
  const wsl = actions.wsl;
  const [online, setOnline] = useState<string[] | null>(null);
  const [choice, setChoice] = useState("");
  const [loadError, setLoadError] = useState("");
  if (!wsl) return null;
  const ready = wsl.installed && wsl.distros.length > 0;
  const details = [wsl.version && `WSL ${wsl.version}`, wsl.kernelVersion && `kernel ${wsl.kernelVersion}`, wsl.wslgVersion && `WSLg ${wsl.wslgVersion}`].filter(Boolean).join(" | ");
  const failed = wsl.checks.filter((check) => !check.ok);
  const notes = [wsl.error, ...failed.map((check) => check.detail ? `${check.name}: ${check.detail}` : check.name), ...wsl.warnings].filter(Boolean);
  const loadOnline = () => {
    setLoadError("");
    app().GetWSLOnlineDistros().then((names) => { setOnline(names); setChoice((current) => current || (names.includes("Ubuntu") ? "Ubuntu" : names[0] ?? "")); })
      .catch((error) => setLoadError(error instanceof Error ? error.message : String(error)));
  };
  const installed = new Set(wsl.distros.map((distro) => distro.name.toLowerCase()));
  const available = (online ?? []).filter((name) => !installed.has(name.toLowerCase()));
  return <div className="tool-row" title={notes.join("\n") || undefined}>
    <span className={`status-dot ${ready && failed.length === 0 ? "online" : wsl.installed ? "warn" : ""}`} />
    <div className="tool-details">
      <div className="tool-heading"><strong className="tool-name"><ToolIcon id="wsl" />WSL (optional)</strong><span className="tool-version">{wsl.installed ? details || "Installed" : "Not installed"}</span></div>
      {wsl.distros.length > 0 && <span className="tool-path">{wsl.distros.map((distro) => `${distro.name}${distro.default ? " (default)" : ""}: ${distro.running ? "running" : "stopped"}`).join(" | ")}</span>}
      {notes.map((note) => <span key={note} className="tool-warning tool-support-note">{note}</span>)}
      {loadError && <span className="tool-warning tool-support-note">{loadError}</span>}
      {!wsl.installed && <span className="muted">Optional: run harnesses inside WSL instead of PowerShell 7. Installing needs administrator approval and may need a Windows restart; TauShell asks first and never restarts Windows.</span>}
      {wsl.installed && wsl.distros.length === 0 && <span className="muted">No distribution found. Install one below, then open it once to create your Linux user.</span>}
      <div className="tool-actions">
        {!wsl.installed && <button className="secondary-button tool-action" disabled={actions.wslBusy} title="Run wsl --install --no-distro (asks first)" onClick={() => actions.onWSLChange("wsl")}>{actions.wslBusy ? "Working..." : "Install WSL"}</button>}
        {wsl.installed && actions.updates.wsl && <button className="secondary-button tool-action" disabled={actions.wslBusy} title={`Run wsl --update to reach ${actions.updates.wsl} (asks first)`} onClick={() => actions.onWSLChange("update")}>{actions.wslBusy ? "Working..." : `Update to ${actions.updates.wsl}`}</button>}
        {wsl.installed && online === null && <button className="secondary-button tool-action" disabled={actions.wslBusy} title="List the distributions Microsoft offers" onClick={loadOnline}>Install a distribution...</button>}
        {wsl.installed && online !== null && (available.length > 0 ? <>
          <select aria-label="Distribution to install" value={choice} onChange={(event) => setChoice(event.target.value)}>{available.map((name) => <option key={name} value={name}>{name}</option>)}</select>
          <button className="secondary-button tool-action" disabled={actions.wslBusy || !choice} title={`Run wsl --install -d ${choice} (asks first)`} onClick={() => actions.onWSLChange("distro", choice)}>Install</button>
        </> : <span className="muted">Every offered distribution is already installed.</span>)}
        {wsl.installed && online !== null && <button className="secondary-button tool-action" disabled={actions.wslBusy} onClick={() => setOnline(null)}>Cancel</button>}
      </div>
    </div>
  </div>;
}
function ToolRow({ tool, actions }: { tool: ToolStatus; actions: ToolActions }) {
  const usable = toolUsable(tool);
  const supportNote = tool.installed && tool.support === "below-minimum" ? `below minimum ${tool.minimum}` :
    tool.installed && tool.support === "untested" ? `untested (tested: ${tool.testedRange})` :
    tool.installed && tool.support === "unknown" && tool.version ? "version support not tested" : "";
  const unavailableCapabilities = (tool.capabilities ?? []).filter((capability) => !capability.available && !capability.optional);
  const installing = actions.installing === tool.id;
  const supportWarnings = (tool.warnings ?? []).filter((warning) =>
    !(tool.support === "untested" && warning.includes("outside the tested range")) &&
    !(tool.support === "below-minimum" && warning.includes("below the minimum supported version")));
  const title = [tool.error, ...supportWarnings].filter(Boolean).join("\n");
  const companionVersions = (tool.companions ?? []).map((companion) => `${companion.name} ${companion.version || (companion.installed ? "installed" : "not found")}`);
  const label = tool.id === "node" && tool.version ? [`Node ${tool.version}`, ...companionVersions].join(" | ") :
    tool.installed ? tool.version || "Installed" : "Not found";
  const executablePaths = [
    tool.executablePath && (tool.id === "node" ? `Node: ${tool.executablePath}` : tool.executablePath),
    ...(tool.companions ?? []).map((companion) => companion.executablePath && `${companion.name}: ${companion.executablePath}`),
  ].filter((path): path is string => !!path);
  const pathLabel = executablePaths.join(" | ");
  const availableUpdate = actions.updates[tool.id];
  const needsManagedInstall = !!tool.install.managedStrategy && !usable && !availableUpdate;
  const needsInstall = !tool.installed || needsManagedInstall;
  const warning = [...new Set([supportNote, tool.error, ...supportWarnings,
    unavailableCapabilities.length > 0 ? `missing: ${unavailableCapabilities.map((capability) => capability.option).join(", ")}` : "",
  ].filter(Boolean))].join("; ");
  return <div className="tool-row" title={title || undefined}>
    <span className={`status-dot ${usable ? "online" : tool.installed || tool.error ? "warn" : ""}`} />
    <div className="tool-details">
      <div className="tool-heading"><strong className="tool-name" title={tool.name}>{isHarness(tool.id) ? <HarnessIcon tool={tool.id} /> : <ToolIcon id={tool.id} />}{tool.name}</strong><span className="tool-version">{label}</span></div>
      {pathLabel && <code className="tool-path" title={executablePaths.join("\n")}>{pathLabel}</code>}
      {warning && <span className="tool-warning tool-support-note">{warning}</span>}
      {(needsInstall || availableUpdate) && <div className="tool-actions">
    {tool.installed && availableUpdate && <button className="secondary-button tool-action" disabled={installing || actions.installing !== ""} title={`Update ${tool.name} to ${availableUpdate}; TauShell asks before changing anything`} onClick={() => actions.onUpdate(tool)}>{installing ? "Updating..." : `Update to ${availableUpdate}`}</button>}
    {needsInstall && <button className="secondary-button tool-action" disabled={installing || actions.installing !== ""} title={installHint(tool, actions.status)} onClick={() => actions.onInstall(tool)}>
      {installing ? "Installing..." : canAutoInstall(tool, actions.status) ? "Install" : "Setup instructions"}
    </button>}
    {needsInstall && <button className="text-button" onClick={() => actions.onOpenSetup(tool)}>Vendor page</button>}
      </div>}
    </div>
    {isHarness(tool.id) && <button className="sidebar-toggle tool-gear" aria-label={`Open ${tool.name} config file`} title={`Open ${tool.name} config file in your default editor`} onClick={() => void app().OpenToolConfig(tool.id).catch((err) => actions.onError(tool, err))}><GearIcon /></button>}
  </div>;
}
