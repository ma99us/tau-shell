export type Session = {
  id: string;
  name: string;
  color?: string;
  tool: "claude-code" | "copilot-cli";
  workingDir: string;
  status: string;
  lastError?: string;
  runningHere: boolean;
  hasResumeId: boolean;
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
  id: "powershell" | "claude-code" | "copilot-cli";
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
  install: { wingetId?: string; source?: string; vendorUrl: string };
};

export type HarnessWidgets = {
  commands: { label: string; text: string }[];
  models: string[];
  interruptKeys: string[];
  compactText: string;
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

export type SystemStatus = {
  webView2Version: string;
  diagnosticsLogPath: string;
  wingetAvailable: boolean;
  tools: ToolStatus[];
};

export type SessionTemplate = {
  tool: "claude-code" | "copilot-cli";
  launchArgs: string[];
};

export type LaunchPreview = {
  executable: string;
  arguments: string[];
  warnings: string[];
};

type WailsApp = {
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
