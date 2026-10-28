// Single list of supported harnesses for the UI. Keep in sync with
// session.SupportedHarnesses in the Go backend.
export type HarnessId = "claude-code" | "copilot-cli" | "gsd" | "codex" | "opencode";

export const harnesses: ReadonlyArray<{ id: HarnessId; name: string; short: string }> = [
  { id: "copilot-cli", name: "GitHub Copilot CLI", short: "Copilot" },
  { id: "claude-code", name: "Claude Code", short: "Claude" },
  { id: "codex", name: "OpenAI Codex CLI", short: "Codex" },
  { id: "opencode", name: "OpenCode", short: "OpenCode" },
  { id: "gsd", name: "GSD", short: "GSD" },
];

export const harnessIds: HarnessId[] = harnesses.map((harness) => harness.id);

export const harnessName = (id: string) => harnesses.find((harness) => harness.id === id)?.name ?? id;
export const harnessShort = (id: string) => harnesses.find((harness) => harness.id === id)?.short ?? id;
export const isHarness = (id: string): id is HarnessId => harnessIds.includes(id as HarnessId);
