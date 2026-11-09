import type { WSLDistro, WSLStatus } from "./bridge";

// Harnesses that find their own conversation in Windows folders cannot run in WSL yet.
const unsupportedInWSL = new Set(["codex", "opencode"]);

export function wslHarnessSupported(tool: string): boolean {
  return !unsupportedInWSL.has(tool);
}

// Distros offered in the create dialog: WSL must be installed and report distros.
export function wslChoices(status: WSLStatus | null): WSLDistro[] {
  return status?.installed ? status.distros ?? [] : [];
}

export function defaultDistro(distros: WSLDistro[]): string {
  return (distros.find((distro) => distro.default) ?? distros[0])?.name ?? "";
}

// A project under /mnt/<drive> lives on the Windows disk and is slow and loses Linux file semantics.
export function isWindowsMount(path: string): boolean {
  return /^\/mnt\/[a-zA-Z](\/|$)/.test(path.trim());
}
