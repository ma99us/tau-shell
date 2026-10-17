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
  name: string;
  installed: boolean;
  version?: string;
  error?: string;
};

export type SystemStatus = {
  webView2Version: string;
  powershell: ToolStatus;
  claudeCode: ToolStatus;
  copilotCLI: ToolStatus;
};

type WailsApp = {
  GetSessions(): Promise<Session[]>;
  GetSystemStatus(): Promise<SystemStatus>;
  SelectWorkingDirectory(): Promise<string>;
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
