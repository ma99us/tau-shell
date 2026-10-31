import { useEffect, useState } from "react";
import { app, CopilotQuota, InstallResult, Session, SkillsView, SystemStatus, ToolStatus } from "./bridge";
import { UsageSummary, hasUsage, monthTooltip, usageText } from "./usage";
import { GearIcon } from "./harnessIcons";
import { harnesses, harnessIds, harnessShort, isHarness } from "./harnesses";
import { ProjectIcon, statusLabel } from "./sessionVisuals";

export function Home({ sessions, loaded, autoResumeAll, onAutoResumeAll, onAutoResume, status, busy, tools, skills, onNew, onRefresh, onHome, onLaunch, onDelete, onDuplicate, dragProps, dropClass, dragId }: {
  skills: SkillsActions;
  autoResumeAll: boolean;
  onAutoResumeAll: (enabled: boolean) => void;
  onAutoResume: (session: Session, enabled: boolean) => void;
  onDuplicate: (session: Session) => void;
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
  const [freshSession, setFreshSession] = useState<Session | null>(null);
  const [summary, setSummary] = useState<UsageSummary | null>(null);
  useEffect(() => {
    let cancelled = false;
    const load = () => { void app().GetUsageSummary().then((value) => { if (!cancelled) setSummary(value); }).catch(() => { if (!cancelled) setSummary(null); }); };
    load();
    const timer = window.setInterval(load, 15000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, []);
  const [quota, setQuota] = useState<CopilotQuota | null>(null);
  useEffect(() => {
    let cancelled = false;
    const load = () => { void app().GetCopilotQuota(false).then((value) => { if (!cancelled) setQuota(value ?? null); }).catch(() => { if (!cancelled) setQuota(null); }); };
    load();
    const timer = window.setInterval(load, 300000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, []);
  const [resumeProgress, setResumeProgress] = useState("");
  const [appVersion, setAppVersion] = useState("");
  const [appUpdate, setAppUpdate] = useState<{ tag: string; url: string } | null>(null);
  useEffect(() => {
    let cancelled = false;
    void app().GetAppVersion().then((value) => { if (!cancelled) setAppVersion(value); }).catch(() => undefined);
    void app().CheckAppUpdate().then((value) => { if (!cancelled) setAppUpdate(value); }).catch(() => undefined);
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
      {quota && !quota.unlimited && <p className={`home-usage home-quota${quota.canProject && quota.projected > quota.entitlement ? " warn" : ""}`} title={`Copilot plan: ${quota.plan}. Read through your GitHub CLI sign-in. ${quota.daysLeft} days left in the cycle.`}>Copilot: {Math.round(quota.used)} / {Math.round(quota.entitlement)} credits ({Math.round(quota.percentUsed)}%), resets {quota.resetDate}{quota.canProject ? `, on pace for ~${Math.round(quota.projected)}` : ""}</p>}
      {summary && hasUsage(summary.month) && <p className="home-usage" title={monthTooltip(summary)}>This month: {usageText(summary.month)}{harnessIds.map((tool) => hasUsage(summary.monthByTool?.[tool]) ? <span key={tool}> | {harnessShort(tool)} {usageText(summary.monthByTool![tool])}</span> : null)}</p>}
      {sessions.length === 0 ? (!loaded ? <div className="loading-card" role="status"><span className="spinner" aria-hidden="true" /><span className="muted">Loading sessions...</span></div> : <div className="empty-card"><div className="empty-icon">+</div><h3>No sessions yet</h3><p>Create a session to connect a project folder to an AI harness such as Claude Code or GitHub Copilot CLI.</p>      <button className="primary-button" onClick={onNew}>New session</button></div>) :
        <div className="session-grid">{sessions.map((session) => <article className={`session-card ${dragId === session.id ? "dragging" : ""} ${dropClass(session.id)}`} key={session.id} {...dragProps(session.id)}>
          <div className="card-top"><ProjectIcon session={session} /><span className={`state-pill ${session.runningHere ? "running" : ""} ${!session.runningHere && session.status === "failed" ? "status-warn-text" : ""}`}>{session.runningHere ? "Running" : statusLabel(session.status)}</span><button className="delete-button" aria-label={`Delete ${session.name}`} title="Delete session" onClick={() => onDelete(session)}>x</button></div>
          <div className="card-title"><h3>{session.name}</h3>{hasUsage(summary?.sessions?.[session.id]) && <span className="card-usage" title="Estimate from harness logs; never reset">{usageText(summary!.sessions![session.id])}</span>}</div><p className="path">{session.workingDir}</p><p className="session-meta">{session.tool}</p>
          {session.lastError && <p className="card-error">{session.lastError}</p>}
          <div className="card-actions">
            {session.runningHere ? <button className="tile-button" onClick={() => onLaunch(session, "start")}>Open</button> :
              session.hasResumeId ? <><button className="tile-button" disabled={busy} title={session.resumeNote} onClick={() => setFreshSession(session)}>Start fresh</button>{!session.resumeNote && <button className="tile-button" disabled={busy} onClick={() => onLaunch(session, "restore")}>Resume</button>}</> :
                <button className="tile-button" disabled={busy} onClick={() => onLaunch(session, "start")}>Start</button>}
            <button className="tile-button" disabled={busy} title="Copy this session definition and start the copy" onClick={() => onDuplicate(session)}>Clone</button>
            <label className="check-option" title={autoResumeAll ? "All sessions auto-resume (global setting is on)" : "Resume this session when TauShell starts"}><input type="checkbox" checked={session.autoResume || autoResumeAll} disabled={autoResumeAll} onChange={(event) => onAutoResume(session, event.target.checked)} />Auto resume</label>
          </div>
        </article>)}</div>}
      <div className="tools-panel"><div className="tools-header"><h2>Tools</h2><div className="refresh-group">{tools.refreshing && <span className="spinner" role="status" aria-label="Refreshing" />}<button className="secondary-button" disabled={tools.refreshing} title="Reload sessions, re-detect tools and check for tool updates" onClick={onRefresh}>Refresh</button></div></div><div className="tool-list">
        {status?.tools.filter((tool) => !isHarness(tool.id)).map((tool) => <ToolRow key={tool.id} tool={tool} actions={tools} />) ?? <div className="loading-card" role="status"><span className="spinner" aria-hidden="true" /><span className="muted">Detecting tools...</span></div>}
      </div></div>
      <div className="tools-panel"><div className="tools-header"><h2>AI Harnesses</h2></div><div className="tool-list">
        {status?.tools.filter((tool) => isHarness(tool.id))
          .sort((left, right) => harnesses.findIndex(({ id }) => id === left.id) - harnesses.findIndex(({ id }) => id === right.id))
          .map((tool) => <ToolRow key={tool.id} tool={tool} actions={tools} />) ?? <div className="loading-card" role="status"><span className="spinner" aria-hidden="true" /><span className="muted">Detecting harnesses...</span></div>}
      </div></div>
      <SkillsPanel skills={skills} unsupportedNote={skills.view?.unsupported} />
      {toolNoticeBanner(tools)}
      <div className="home-footer">
        {appUpdate && <button className="link-button home-update" title="Open the TauShell releases page" onClick={() => void app().OpenReleasePage()}>{appUpdate.tag} available</button>}
        {appVersion && <span className="home-version" title="TauShell version">{appVersion === "dev" ? "dev build" : `v${appVersion.replace(/^v/, "")}`}</span>}
      </div>
    </div>
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
  onUpdate: () => void;
  onToggle: (name: string, enabled: boolean) => void;
};

const toolLabel = harnessShort;

function SkillsPanel({ skills, unsupportedNote }: { skills: SkillsActions; unsupportedNote?: string[] }) {
  const view = skills.view;
  if (view && !view.status.enabled) return null;
  const status = view?.status;
  const origin = status ? (status.source === "cookbook" ? `ai-cookbook @${status.applied}` : `bundled @${status.applied || "snapshot"}`) : "";
  return <div className="tools-panel skills-panel">
    <div className="tools-header">
      <div><h2>Skills</h2><p className="muted">{view ? `${origin}. Applies to new or resumed sessions.` : "Loading skills..."}</p></div>
      <div className="refresh-group">
        {(skills.updating || !view) && <span className="spinner" role="status" aria-label="Updating skills" />}
        {status?.updateAvailable && <button className="secondary-button" disabled={skills.updating || skills.refreshing} title={`Download and use ai-cookbook commit ${status.latest}`} onClick={skills.onUpdate}>Update skills ({status.latest})</button>}
      </div>
    </div>
    <div className="skill-list">
      {view?.skills.map((skill) => <label className="skill-row" key={skill.name} title={skill.description}>
        <input type="checkbox" checked={skill.enabled} onChange={(event) => skills.onToggle(skill.name, event.target.checked)} />
        <strong>{skill.name}</strong>
        <span className="skill-source">{skill.personal?.length ? `${skill.personal.map(toolLabel).join(", ")}: your copy is used` : skill.source}</span>
      </label>)}
    </div>
    {status?.message && <p className="muted skill-note">{status.message}</p>}
    {unsupportedNote?.length ? <p className="skill-note skill-warn">Skills are not supported by the installed {unsupportedNote.map(toolLabel).join(", ")} version.</p> : null}
    {status?.warnings?.map((warning) => <p className="skill-note skill-warn" key={warning}>{warning}</p>)}
  </div>;
}

type ToolActions = {
  status: SystemStatus | null;
  installing: string;
  notice: InstallResult | null;
  updates: Record<string, string>;
  refreshing: boolean;
  onUpdate: (tool: ToolStatus) => void;
  onInstall: (tool: ToolStatus) => void;
  onOpenSetup: (tool: ToolStatus) => void;
  onDismissNotice: () => void;
  onError: (error: unknown) => void;
};

export function toolUsable(tool: ToolStatus): boolean {
  return tool.installed && !tool.error && tool.support !== "below-minimum" &&
    (tool.capabilities ?? []).every((capability) => capability.available || capability.optional);
}

function toolNoticeBanner(tools: ToolActions) {
  if (!tools.notice) return null;
  const tone = tools.notice.outcome === "installed" ? "notice-success" : tools.notice.outcome === "failed" ? "notice-error" : "notice-info";
  // Open through the Go binding; a target=_blank link would spawn an unguarded WebView2 popup.
  const vendorTool = tools.notice.vendorUrl ? tools.status?.tools.find((tool) => tool.install.vendorUrl === tools.notice?.vendorUrl) : undefined;
  return <div className={`tool-notice ${tone}`}>
    <span>{tools.notice.message}</span>
    {vendorTool && <button className="link-button" onClick={() => tools.onOpenSetup(vendorTool)}>Open vendor setup page</button>}
    <button className="icon-button" onClick={tools.onDismissNotice}>x</button>
  </div>;
}

export function Setup({ powershell, actions, onRecheck, onContinue }: {
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

function ToolRow({ tool, actions }: { tool: ToolStatus; actions: ToolActions }) {
  const usable = toolUsable(tool);
  const supportNote = tool.installed && tool.support === "below-minimum" ? `below minimum ${tool.minimum}` :
    tool.installed && tool.support === "untested" ? `untested (tested: ${tool.testedRange})` :
    tool.installed && tool.support === "unknown" && tool.version ? "version support not tested" : "";
  const unavailableCapabilities = (tool.capabilities ?? []).filter((capability) => !capability.available && !capability.optional);
  const installing = actions.installing === tool.id;
  const title = [tool.error, ...(tool.warnings ?? [])].filter(Boolean).join("\n");
  const companionVersions = (tool.companions ?? []).map((companion) => `${companion.name} ${companion.version || (companion.installed ? "installed" : "not found")}`);
  const label = tool.id === "node" && tool.version ? [`Node ${tool.version}`, ...companionVersions].join(" | ") :
    tool.installed ? tool.version || "Installed" : "Not found";
  const executablePaths = [
    tool.executablePath && (tool.id === "node" ? `Node: ${tool.executablePath}` : tool.executablePath),
    ...(tool.companions ?? []).map((companion) => companion.executablePath && `${companion.name}: ${companion.executablePath}`),
  ].filter((path): path is string => !!path);
  const pathLabel = executablePaths.join(" | ");
  const needsManagedInstall = !!tool.install.managedStrategy && !usable;
  const needsInstall = !tool.installed || needsManagedInstall;
  const canUpdateManaged = tool.managed && usable && !!tool.install.managedStrategy;
  const warning = [...new Set([supportNote, tool.error, ...(tool.warnings ?? []),
    unavailableCapabilities.length > 0 ? `missing: ${unavailableCapabilities.map((capability) => capability.option).join(", ")}` : "",
  ].filter(Boolean))].join("; ");
  return <div className="tool-row" title={title || undefined}>
    <span className={`status-dot ${usable ? "online" : tool.installed || tool.error ? "warn" : ""}`} />
    <div className="tool-details">
      <div className="tool-heading"><strong title={tool.name}>{tool.name}</strong><span className="tool-version">{label}</span></div>
      {pathLabel && <code className="tool-path" title={executablePaths.join("\n")}>{pathLabel}</code>}
      {warning && <span className="tool-warning tool-support-note">{warning}</span>}
      {(needsInstall || actions.updates[tool.id] || canUpdateManaged) && <div className="tool-actions">
    {canUpdateManaged && <button className="secondary-button tool-action" disabled={installing || actions.installing !== ""} title={installHint(tool, actions.status)} onClick={() => actions.onInstall(tool)}>{installing ? "Updating..." : "Update"}</button>}
    {tool.installed && actions.updates[tool.id] && <button className="secondary-button tool-action" disabled={installing || actions.installing !== ""} title={`Update ${tool.name} to ${actions.updates[tool.id]} with winget (asks first)`} onClick={() => actions.onUpdate(tool)}>{installing ? "Updating..." : `Update to ${actions.updates[tool.id]}`}</button>}
    {needsInstall && <button className="secondary-button tool-action" disabled={installing || actions.installing !== ""} title={installHint(tool, actions.status)} onClick={() => actions.onInstall(tool)}>
      {installing ? "Installing..." : canAutoInstall(tool, actions.status) ? "Install" : "Setup instructions"}
    </button>}
    {needsInstall && <button className="text-button" onClick={() => actions.onOpenSetup(tool)}>Vendor page</button>}
      </div>}
    </div>
    {isHarness(tool.id) && <button className="sidebar-toggle tool-gear" aria-label={`Open ${tool.name} config file`} title={`Open ${tool.name} config file in your default editor`} onClick={() => void app().OpenToolConfig(tool.id).catch((err) => actions.onError(err))}><GearIcon /></button>}
  </div>;
}
