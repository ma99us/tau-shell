export type Session = {
  id: string;
  name: string;
  tool: "claude-code" | "copilot-cli";
  workingDir: string;
  status: string;
  lastError?: string;
  runningHere: boolean;
  hasResumeId: boolean;
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
  capabilities: { name: string; option: string; available: boolean }[] | null;
  warnings: string[] | null;
  error?: string;
  install: { wingetId?: string; source?: string; vendorUrl: string };
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
  LogFrontendFailure(source: string, message: string, stack: string): Promise<void>;
  LogTerminalPointerEvent(id: string, event: "pointerdown" | "mousedown"): Promise<void>;
  LogTerminalMouseTrackingMode(id: string, mode: "none" | "x10" | "vt200" | "drag" | "any"): Promise<void>;
  SelectWorkingDirectory(): Promise<string>;
  GetSessionTemplate(tool: string): Promise<SessionTemplate>;
  PreviewSessionLaunch(tool: string, launchArgs: string[]): Promise<LaunchPreview>;
  CreateSession(name: string, tool: string, workingDir: string, launchArgs: string[]): Promise<Session>;
  StartSession(id: string, columns: number, rows: number): Promise<Session>;
  RestoreSession(id: string, columns: number, rows: number): Promise<Session>;
  StartFresh(id: string, columns: number, rows: number): Promise<Session>;
  WriteTerminal(id: string, input: string): Promise<void>;
  WriteTerminalBinary(id: string, input: string): Promise<void>;
  ResizeTerminal(id: string, columns: number, rows: number): Promise<void>;
  StopSession(id: string): Promise<void>;
};

type WailsRuntime = {
  EventsOnMultiple(name: string, callback: (event: SessionEvent) => void, maxCallbacks: number): void;
  EventsOff(name: string): void;
};

declare global {
  interface Window {
    go: { main: { App: WailsApp } };
    runtime: WailsRuntime;
  }
}

export const app = () => window.go.main.App;
export const events = () => window.runtime;
