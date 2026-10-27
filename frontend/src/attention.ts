import type { Terminal } from "@xterm/xterm";

// "working" / "idle" / "input" (waiting for the user to answer a prompt) when
// the harness itself says so; undefined when the harness gives no usable cue
// (callers fall back to output timing).
export type HarnessSignal = "working" | "idle" | "input" | undefined;

const brailleSpinner = /^[\u2800-\u28FF\u25D0-\u25D3]/;
// Claude Code 2.1.311 animates the title prefix with U+25D0-25D3 (half circles)
// while working and shows U+2733 when idle; the trust-folder prompt footer is
// "Enter to confirm · Esc to cancel".
const claudePromptFooter = /\bEnter to confirm\b.*\bEsc to cancel\b/i;
const copilotWorkingStatus = /^\S{0,2}\s*Working\b.*\besc\b/i;
const copilotIdleFooter = /\? help/;
// Footer of Copilot CLI's selection prompts (permission requests, pickers),
// observed on Copilot CLI 1.0.103 as "navigate · enter to select · esc to cancel".
const copilotPromptFooter = /\benter to select\b.*\besc to cancel\b/i;

function lastRows(terminal: Terminal, count: number): string[] {
  const buffer = terminal.buffer.active;
  const end = buffer.baseY + terminal.rows;
  const rows: string[] = [];
  for (let index = end - 1; index >= end - count; index--) {
    const line = buffer.getLine(index);
    if (line) rows.push(line.translateToString(true).trim());
  }
  return rows;
}

// Harness-specific cues only: terminal title for Claude Code, the status/footer
// row (last two rows, never the whole screen) for Copilot CLI.
export function readHarnessSignal(tool: string, terminal: Terminal | undefined, title: string): HarnessSignal {
  if (tool === "claude-code") {
    if (terminal && lastRows(terminal, 2).some((row) => claudePromptFooter.test(row))) return "input";
    if (brailleSpinner.test(title)) return "working";
    if (title.startsWith("\u2733")) return "idle";
    return undefined;
  }
  if (tool === "copilot-cli" && terminal) {
    const rows = lastRows(terminal, 2);
    if (rows.some((row) => copilotPromptFooter.test(row))) return "input";
    if (rows.some((row) => copilotWorkingStatus.test(row))) return "working";
    if (rows.some((row) => copilotIdleFooter.test(row))) return "idle";
  }
  return undefined;
}
