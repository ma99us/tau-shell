export type Progress = { phase: string; done: number; total: number };

const phaseText: Record<string, string> = {
  downloading: "Downloading",
  verifying: "Verifying checksum",
  installing: "Installing",
  restarting: "Restarting TauShell",
};

export function progressText(progress: Progress): string {
  const label = phaseText[progress.phase] ?? progress.phase;
  if (progress.phase === "downloading" && progress.total > 0) return `${label} ${Math.min(100, Math.floor((progress.done / progress.total) * 100))}%`;
  return `${label}...`;
}
