import type { SessionUsage, UsageSummary } from "./usage";
import type { ClaudeQuota } from "./claudeQuota";

export type CopilotQuota = {
  account: string;
  plan: string;
  unlimited: boolean;
  entitlement: number;
  used: number;
  remaining: number;
  percentUsed: number;
  resetDate: string;
  cycleDays: number;
  daysLeft: number;
  projected: number;
  overLimit: boolean;
  overagesPermitted: boolean;
  canProject: boolean;
};

export type CodexQuota = {
  plan: string;
  windows: { limitId: string; label: string; usedPercent: number; windowMinutes: number; resetAt?: string }[];
};
import type { HarnessId, HarnessInfo, HarnessLoadError } from "./harnesses.ts";
import type { WorkspaceGroup } from "./workspaces";
import type { Appearance } from "./theme/appearanceModel";

export type Session = {
  id: string;
  name: string;
  color?: string;
  notes?: string;
  link?: string;
  linkLabel?: string;
  tags?: string[];
  tool: HarnessId;
  workingDir: string;
  runtime?: string;
  distro?: string;
  worktree?: string;
  status: string;
  lastError?: string;
  runningHere: boolean;
  hasResumeId: boolean;
  autoResume: boolean;
  resumeNote?: string;
  launchNote?: string;
};

export type DialogRequest = {
  id: string;
  kind: "info" | "warning" | "error" | "question";
  title: string;
  message: string;
  buttons: string[];
  defaultButton: string;
  cancelButton: string;
};

export type SessionEvent = {
  kind: "output" | "state";
  sessionId: string;
  dataBase64?: string;
  session?: Session;
};

export type ToolStatus = {
  id: "powershell" | "node" | "git" | HarnessId;
  name: string;
  required: boolean;
  installed: boolean;
  managed: boolean;
  executablePath?: string;
  version?: string;
  versionOutput?: string;
  minimum?: string;
  testedRange: string;
  support: "tested" | "untested" | "below-minimum" | "unknown";
  capabilities: { name: string; option: string; optional?: boolean; available: boolean }[] | null;
  companions?: { name: string; executablePath?: string; version?: string; installed: boolean; error?: string }[];
  warnings: string[] | null;
  error?: string;
  widgets?: HarnessWidgets;
  install: { wingetId?: string; npmPackage?: string; managedStrategy?: "node-lts" | "portable-git"; source?: string; vendorUrl: string };
};

export type HarnessWidgets = {
  commands: { label: string; text: string }[];
  models: string[];
  modelPicker?: string;
  interruptKeys: string[];
  compactText: string;
  clearText: string;
};

export type WorktreeInfo = { name: string; path: string; mainRepo: string; branch: string };
export type GitStatus = { branch: string; dirty: boolean; ahead: number; behind: number; worktree?: WorktreeInfo };
export type WorktreeOptions = { name: string; branch: string; base: string };
export type WorktreePlan = {
  name: string;
  branch: string;
  path: string;
  repo: string;
  base: string;
  branches: string[];
  dirty: boolean;
};
export type WorktreeEntry = { path: string; branch: string; main: boolean; detached: boolean; locked: boolean; prunable: boolean };
export type WorktreeDeletePlan = {
  managed: boolean;
  name: string;
  branch: string;
  path: string;
  dirty: boolean;
  unpushed: number;
  exists: boolean;
  sharedBy?: string;
};
export type GitPathInfo = { repo: boolean; tracked: boolean; staged: boolean; unstaged: boolean; ignored: boolean };
export type GitChange = { path: string; code: string; staged: boolean };
export type ProjectEntry = { name: string; path: string; dir: boolean; size: number };
export type ProjectListing = { entries: ProjectEntry[]; truncated: boolean };
export type AppUpdate = { tag: string; url: string; notes: string; canInstall: boolean; reason: string };
export type UpdateResult = { from: string; to: string; sessions: string[] | null };

export type ProjectFile = { path: string; text: string; imageUrl?: string; truncated: boolean; unsupported?: string };
export type SecretInfo = { name: string; usedBy: string[]; missing: boolean };
export type Conversation = { harnessSessionId: string; title: string; model: string; firstDay: string; lastDay: string; tokens: number; usd: number; unpriced: boolean; current: boolean; unavailable: string };

export type ResolvedPreset = {
  name: string;
  text: string;
  submit: boolean;
  remote?: boolean;
  unknown?: string[];
};

export type InstallResult = {
  outcome: "installed" | "cancelled" | "winget-unavailable" | "npm-unavailable" | "prerequisite-required" | "failed";
  message: string;
  vendorUrl?: string;
  prerequisiteId?: "node";
  status?: ToolStatus;
};

export type ShutdownProgress = {
  phase: "stopping" | "done";
  sessions: { name: string; state: "stopping" | "stopped" | "force-stopped" }[];
  elapsedSeconds: number;
};

export type SystemStatus = {
  webView2Version: string;
  diagnosticsLogPath: string;
  wingetAvailable: boolean;
  npmAvailable: boolean;
  tools: ToolStatus[];
};

export type SessionTemplate = {
  tool: HarnessId;
  launchArgs: string[];
};

export type WSLDistro = { name: string; default: boolean; running: boolean; version: number; state: string };
export type WSLCheck = { name: string; ok: boolean; detail?: string };
export type WSLStatus = {
  installed: boolean;
  version?: string;
  kernelVersion?: string;
  wslgVersion?: string;
  defaultDistro?: string;
  distros: WSLDistro[];
  checks: WSLCheck[];
  warnings: string[];
  error?: string;
};

export type WSLInstallResult = { outcome: "installed" | "restart-required" | "cancelled" | "failed"; message: string; status: WSLStatus };
export type WSLAction = "wsl" | "distro" | "update";

export type LaunchPreview = {
  executable: string;
  arguments: string[];
  warnings: string[];
};

export type SkillRow = {
  name: string;
  description: string;
  source: string;
  enabled: boolean;
  personal?: string[];
};

export type SkillsStatus = {
  enabled: boolean;
  repo?: string;
  ref?: string;
  source: string;
  applied?: string;
  latest?: string;
  updateAvailable: boolean;
  message?: string;
  warnings?: string[];
};

export type ControlAuditEntry = {
  time: string;
  pid: number;
  image: string;
  op: string;
  session?: string;
  outcome: string;
};

export type ControlStatus = {
  enabled: boolean;
  audit: ControlAuditEntry[];
};

export type PhoneDevice = { id: string; name: string; fingerprint: string; created: string; expires: string; lastSeen?: string };

export type MobileStatus = {
  enabled: boolean;
  interface?: string;
  address?: string;
  pairing: "idle" | "open" | "awaiting-approval";
  pendingName?: string;
  devices: PhoneDevice[];
  audit: ControlAuditEntry[];
};

export type MobileInterface = { name: string; address: string };

export type PairingOffer = { host: string; port: number; fingerprint: string; code: string; expiresAt: string; link: string; qr: string };

export type SkillsView = {
  status: SkillsStatus;
  skills: SkillRow[];
  unsupported?: string[];
};

type WailsApp = {
  GetAppearance(): Promise<Appearance>;
  SetAppearance(value: Appearance): Promise<void>;
  ReportTheme(resolved: string): Promise<void>;
  SetControlEnabled(enabled: boolean): Promise<void>;
  GetControlStatus(): Promise<ControlStatus>;
  SetMobileEnabled(enabled: boolean): Promise<void>;
  GetMobileStatus(): Promise<MobileStatus>;
  GetMobileInterfaces(): Promise<MobileInterface[]>;
  SetMobileInterface(address: string): Promise<void>;
  StartPairing(): Promise<PairingOffer>;
  CancelPairing(): Promise<void>;
  DecidePairing(approve: boolean): Promise<void>;
  RevokePhone(id: string): Promise<void>;
  GetSkills(): Promise<SkillsView>;
  CheckSkills(): Promise<SkillsView>;
  UpdateSkills(): Promise<SkillsView>;
  SetSkillEnabled(name: string, enabled: boolean): Promise<void>;
  GetSessions(): Promise<Session[]>;
  GetSystemStatus(): Promise<SystemStatus>;
  InstallTool(id: string): Promise<InstallResult>;
  OpenToolSetupPage(id: string): Promise<void>;
  GetPresets(sessionId: string): Promise<ResolvedPreset[] | null>;
  OpenPresetsFile(): Promise<void>;
  LogFrontendFailure(source: string, message: string, stack: string): Promise<void>;
  LogTerminalPointerEvent(id: string, event: "pointerdown" | "mousedown"): Promise<void>;
  LogTerminalMouseTrackingMode(id: string, mode: "none" | "x10" | "vt200" | "drag" | "any"): Promise<void>;
  SelectWorkingDirectory(): Promise<string>;
  SelectWSLWorkingDirectory(distro: string): Promise<string>;
  GetWSLStatus(): Promise<WSLStatus>;
  GetWSLOnlineDistros(): Promise<string[]>;
  InstallWSL(action: WSLAction, distro: string): Promise<WSLInstallResult>;
  GetSessionTemplate(tool: string): Promise<SessionTemplate>;
  GetHarnesses(): Promise<{ harnesses: HarnessInfo[]; errors: HarnessLoadError[] }>;
  ConfirmHarness(id: string): Promise<void>;
  PreviewSessionLaunch(tool: string, launchArgs: string[]): Promise<LaunchPreview>;
  CreateSession(name: string, tool: string, workingDir: string, launchArgs: string[]): Promise<Session>;
  CreateSessionIn(runtime: string, distro: string, name: string, tool: string, workingDir: string, launchArgs: string[]): Promise<Session>;
  PreviewSessionLaunchIn(runtime: string, distro: string, tool: string, launchArgs: string[]): Promise<LaunchPreview>;
  PreviewSessionYAMLIn(runtime: string, distro: string, name: string, tool: string, workingDir: string, launchArgs: string[]): Promise<string>;
  PreviewSessionYAML(name: string, tool: string, workingDir: string, launchArgs: string[]): Promise<string>;
  CreateSessionFromYAML(text: string): Promise<Session>;
  StartSession(id: string, columns: number, rows: number): Promise<Session>;
  RestoreSession(id: string, columns: number, rows: number): Promise<Session>;
  StartFresh(id: string, columns: number, rows: number): Promise<Session>;
  WriteTerminal(id: string, input: string): Promise<void>;
  WriteTerminalBinary(id: string, input: string): Promise<void>;
  ResizeTerminal(id: string, columns: number, rows: number): Promise<void>;
  StopSession(id: string): Promise<void>;
  DeleteSession(id: string): Promise<void>;
  RenameSession(id: string, name: string): Promise<void>;
  SetSessionColor(id: string, color: string): Promise<void>;
  SetSessionNotes(id: string, notes: string): Promise<void>;
  SetSessionLink(id: string, link: string): Promise<void>;
  SetSessionTags(id: string, tags: string[]): Promise<void>;
  OpenSessionLink(id: string, url: string): Promise<void>;
  SetSessionAutoResume(id: string, enabled: boolean): Promise<void>;
  GetAutoResumeAll(): Promise<boolean>;
  SetAutoResumeAll(enabled: boolean): Promise<void>;
  SetSessionOrder(ids: string[]): Promise<void>;
  NotifyAttention(id: string, kind: "input" | "done"): Promise<void>;
  ReportSessionState(id: string, state: string, source: string, hasDraft: boolean, question: boolean): Promise<void>;
  ProvideScreen(request: string, rows: string[]): Promise<void>;
  SetQueuedPrompts(count: number): Promise<void>;
  LoadPromptQueues(): Promise<Record<string, string[]>>;
  SavePromptQueue(sessionId: string, prompts: string[]): Promise<void>;
  RecordQueueSend(session: string, outcome: string): Promise<void>;
  OpenSettingsFile(): Promise<void>;
  OpenToolConfig(toolId: string): Promise<void>;
  GetToolUpdates(): Promise<Record<string, string>>;
  CheckToolUpdates(): Promise<Record<string, string>>;
  UpdateTool(id: string): Promise<InstallResult>;
  GetGitBranch(sessionId: string): Promise<string>;
  GetGitStatus(sessionId: string): Promise<GitStatus | null>;
  PlanWorktree(runtime: string, distro: string, dir: string, sessionName: string): Promise<WorktreePlan>;
  ListWorktrees(runtime: string, distro: string, dir: string): Promise<WorktreeEntry[]>;
  CreateSessionInWorktree(
    runtime: string,
    distro: string,
    name: string,
    tool: string,
    workingDir: string,
    launchArgs: string[],
    options: WorktreeOptions,
  ): Promise<Session>;
  CloneSessionInWorktree(id: string, tool: string, launchArgs: string[], options: WorktreeOptions): Promise<Session>;
  GetWorktreeDeletePlan(id: string): Promise<WorktreeDeletePlan>;
  DeleteSessionWithWorktree(id: string, removeFolder: boolean, deleteBranch: boolean, force: boolean): Promise<void>;
  OpenPullRequest(sessionId: string): Promise<void>;
  SyncGit(sessionId: string): Promise<string>;
  FetchGit(sessionId: string): Promise<boolean>;
  ListGitChanges(sessionId: string): Promise<GitChange[] | null>;
  GetHarnessActivity(sessionId: string): Promise<string>;
  ListProjectDir(sessionId: string, rel: string, showAll: boolean): Promise<ProjectListing>;
  ReadProjectFile(sessionId: string, rel: string): Promise<ProjectFile>;
  ListConversations(sessionId: string): Promise<Conversation[]>;
  GetWorkspaces(): Promise<Record<string, WorkspaceGroup>>;
  ListSecrets(): Promise<SecretInfo[]>;
  SetSecret(name: string, value: string): Promise<void>;
  DeleteSecret(name: string): Promise<void>;
  ResumeConversation(sessionId: string, harnessSessionId: string, columns: number, rows: number): Promise<Session>;
  RevealProjectFile(sessionId: string, rel: string): Promise<void>;
  RevealProjectRoot(sessionId: string): Promise<void>;
  OpenProjectFile(sessionId: string, rel: string): Promise<void>;
  ProjectFullPath(sessionId: string, rel: string): Promise<string>;
  GetGitPathInfo(sessionId: string, rel: string): Promise<GitPathInfo>;
  GitPathAction(sessionId: string, rel: string, action: "stage" | "unstage" | "untrack" | "ignore"): Promise<string>;
  GetSessionModel(sessionId: string): Promise<string>;
  GetSessionUsage(sessionId: string): Promise<SessionUsage>;
  GetUsageSummary(): Promise<UsageSummary>;
  GetCopilotQuota(refresh: boolean): Promise<CopilotQuota | null>;
  GetCodexQuota(refresh: boolean): Promise<CodexQuota | null>;
  GetClaudeQuota(refresh: boolean): Promise<ClaudeQuota | null>;
  GetAppVersion(): Promise<string>;
  CheckAppUpdate(): Promise<AppUpdate | null>;
  GetUpdateResult(): Promise<UpdateResult | null>;
  GetRollbackVersion(): Promise<string>;
  InstallAppUpdate(): Promise<void>;
  RollbackAppUpdate(): Promise<void>;
  OpenReleasePage(): Promise<void>;
  DuplicateSession(id: string, tool: string, launchArgs: string[]): Promise<Session>;
  GetSessionLaunchArgs(id: string): Promise<string[]>;
  DialogHostReady(): Promise<DialogRequest[]>;
  AnswerDialog(id: string, choice: string): Promise<void>;
};

type WailsRuntime = {
  EventsOnMultiple(name: string, callback: (event: any) => void, maxCallbacks: number): void;
  EventsOff(name: string): void;
  ClipboardGetText(): Promise<string>;
  ClipboardSetText(text: string): Promise<boolean>;
};

declare global {
  interface Window {
    go: { main: { App: WailsApp } };
    runtime: WailsRuntime;
  }
}

export const app = () => window.go.main.App;
