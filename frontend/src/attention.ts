import type { Terminal } from "@xterm/xterm";

// "working" / "idle" when the harness itself says so; undefined when the
// harness gives no usable cue (callers fall back to output timing).
export type HarnessSignal = "working" | "idle" | undefined;

const brailleSpinner = /^[\u2800-\u28FF]/;
const copilotWorkingStatus = /^\S{0,2}\s*Working\b.*\besc\b/i;
const copilotIdleFooter = /\? help/;

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
    if (brailleSpinner.test(title)) return "working";
    if (title.startsWith("\u2733")) return "idle";
    return undefined;
  }
  if (tool === "copilot-cli" && terminal) {
    const rows = lastRows(terminal, 2);
    if (rows.some((row) => copilotWorkingStatus.test(row))) return "working";
    if (rows.some((row) => copilotIdleFooter.test(row))) return "idle";
  }
  return undefined;
}
