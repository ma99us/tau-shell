import type { Terminal } from "@xterm/xterm";

// "working" / "idle" / "input" (waiting for the user to answer a prompt) when
// the harness itself says so; undefined when the harness gives no usable cue
// (callers fall back to output timing).
export type HarnessSignal = "working" | "idle" | "input" | undefined;

export type AttentionKind = "input" | "done";
export const minWorkMsForDone = 3000;

// Edge detector for background alerts: fires once when the harness starts
// waiting for an answer, or when it stops working after a real piece of work
// (not a brief blip and not a hand-off to a prompt).
export function attentionEdge(
  old: { working: boolean; input: boolean },
  next: { working: boolean; input: boolean },
  workedMs: number,
): AttentionKind | undefined {
  if (next.input && !old.input) return "input";
  if (old.working && !next.working && !next.input && workedMs >= minWorkMsForDone) return "done";
  return undefined;
}

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

const sentenceEnd = /[.!?:;]$/;

// Text typed in the harness input, or "" when the input is empty. Both harnesses
// are Ink UIs: they hide the terminal cursor and draw their own, so the cursor
// position is useless. Instead find the lowest input row and read its cells,
// skipping dim (placeholder) and inverse (drawn cursor) cells. The input row starts
// with a prompt ("> ", "\u276F ", possibly inside a box border, as in Claude Code) or
// a left bar in column 0 (Copilot CLI).
export function readDraft(terminal: Terminal): string {
  const buffer = terminal.buffer.active;
  const end = buffer.baseY + terminal.rows;
  const cell = buffer.getNullCell();
  for (let y = end - 1; y >= Math.max(0, end - 12); y--) {
    const line = buffer.getLine(y);
    if (!line) continue;
    const marker = /^(?:[\u2502\u2503\u2551\u2580-\u259F|]|[\u2502\u2503|]?\s*[>\u276F\u203A])(?:\s|$)/.exec(line.translateToString(true));
    if (!marker) continue;
    let text = "";
    for (let x = marker[0].length; x < line.length; x++) {
      const current = line.getCell(x, cell);
      if (!current || current.isDim() || current.isInverse()) continue;
      text += current.getChars() || " ";
    }
    return text.replace(/[\s\u2502\u2503|]+$/, "");
  }
  return "";
}

// Joins text we inject with what the user already typed: separate it from the
// draft by ". " when needed, and leave ". " after an insert-only prompt so the
// user can keep typing. Slash and shell commands are passed through untouched.
export function composeInjection(draft: string, text: string, submit: boolean): string {
  if (/^\s*[/!]/.test(text)) return text;
  let out = text;
  if (draft.trim()) {
    const endsWithSpace = /\s$/.test(draft);
    const punctuation = sentenceEnd.test(draft.trimEnd()) || endsWithSpace ? "" : ".";
    out = `${punctuation}${endsWithSpace ? "" : " "}${out}`;
  }
  if (!submit) {
    const body = out.trimEnd();
    out = `${body}${sentenceEnd.test(body) ? "" : "."} `;
  }
  return out;
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
