import type { SessionUsage, UsageSummary } from "./usage";

export type CopilotQuota = {
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
import type { HarnessId } from "./harnesses";

export type Session = {
  id: string;
  name: string;
  color?: string;
  tool: HarnessId;
  workingDir: string;
  status: string;
  lastError?: string;
  runningHere: boolean;
  hasResumeId: boolean;
  autoResume: boolean;
  resumeNote?: string;
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
  id: "powershell" | HarnessId;
  name: string;
  required: boolean;
  installed: boolean;
  executablePath?: string;
  version?: string;
  versionOutput?: string;
  minimum?: string;
  testedRange: string;
  support: "tested" | "untested" | "below-minimum" | "unknown";
  capabilities: { name: string; option: string; optional?: boolean; available: boolean }[] | null;
  warnings: string[] | null;
  error?: string;
  widgets?: HarnessWidgets;
  install: { wingetId?: string; npmPackage?: string; source?: string; vendorUrl: string };
};

export type HarnessWidgets = {
  commands: { label: string; text: string }[];
  models: string[];
  interruptKeys: string[];
  compactText: string;
  clearText: string;
};

export type ResolvedPreset = {
  name: string;
  text: string;
  submit: boolean;
  unknown?: string[];
};

export type InstallResult = {
  outcome: "installed" | "cancelled" | "winget-unavailable" | "failed";
  message: string;
  vendorUrl?: string;
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

export type SkillsView = {
  status: SkillsStatus;
  skills: SkillRow[];
  unsupported?: string[];
};

type WailsApp = {
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
  GetSessionTemplate(tool: string): Promise<SessionTemplate>;
  PreviewSessionLaunch(tool: string, launchArgs: string[]): Promise<LaunchPreview>;
  CreateSession(name: string, tool: string, workingDir: string, launchArgs: string[]): Promise<Session>;
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
  SetSessionAutoResume(id: string, enabled: boolean): Promise<void>;
  GetAutoResumeAll(): Promise<boolean>;
  SetAutoResumeAll(enabled: boolean): Promise<void>;
  SetSessionOrder(ids: string[]): Promise<void>;
  NotifyAttention(id: string, kind: "input" | "done"): Promise<void>;
  OpenSettingsFile(): Promise<void>;
  OpenToolConfig(toolId: string): Promise<void>;
  GetToolUpdates(): Promise<Record<string, string>>;
  CheckToolUpdates(): Promise<Record<string, string>>;
  UpdateTool(id: string): Promise<InstallResult>;
  GetGitBranch(sessionId: string): Promise<string>;
  GetSessionModel(sessionId: string): Promise<string>;
  GetSessionUsage(sessionId: string): Promise<SessionUsage>;
  GetUsageSummary(): Promise<UsageSummary>;
  GetCopilotQuota(refresh: boolean): Promise<CopilotQuota | null>;
  GetAppVersion(): Promise<string>;
  CheckAppUpdate(): Promise<{ tag: string; url: string } | null>;
  OpenReleasePage(): Promise<void>;
  DuplicateSession(id: string): Promise<Session>;
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
export const events = () => window.runtime;
