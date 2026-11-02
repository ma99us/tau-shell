import type { Terminal } from "@xterm/xterm";

// "working" / "idle" / "input" (waiting for the user to answer a prompt) when
// the harness itself says so; undefined when the harness gives no usable cue
// (callers fall back to output timing).
export type HarnessSignal = "working" | "idle" | "input" | undefined;

export type AttentionKind = "input" | "done";
export const minWorkMsForDone = 3000;

export type Activity = { working: boolean; input: boolean; unseen: boolean };
// Output newer than this means the session is "working"; older means "quiet".
export const workingWindowMs = 2500;
// A harness "working" cue with no output for this long is treated as stale: a live
// harness animates its cue (spinner/timer), which produces output.
export const staleWorkingMs = 15000;

// One poll step for a running session. The harness cue wins; output timing is the
// fallback when the harness gives none. lastOutputAt excludes keystroke echo.
// "seen" means the session is selected and TauShell has focus.
export function nextActivity(old: Activity, signal: HarnessSignal, lastOutputAt: number | undefined, now: number, seen: boolean, cueTool = false): Activity {
  const outputAge = lastOutputAt === undefined ? Infinity : now - lastOutputAt;
  // Tools with reliable cues never fall back to output timing: idle redraws
  // (spinners, promos, clocks) would otherwise look like work and raise "done" alerts.
  const working = signal === undefined ? !cueTool && outputAge < workingWindowMs : signal === "working" && outputAge <= staleWorkingMs;
  const input = signal === "input";
  const unseen = seen ? false : old.unseen || (old.working && !working) || (input && !old.input);
  return { working, input, unseen };
}

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
// Claude Code 2.1.316 animates the title prefix with U+25D0-25D3 (half circles)
// while working and shows U+2733 when idle (also while a permission prompt is
// open, so the title cannot signal input). Prompt footers: the trust-folder
// prompt shows "Enter to confirm · Esc to cancel"; shell-command and file-edit
// permission prompts show "Esc to cancel · Tab to amend".
// Question and plan-approval prompts (2.1.316) show "Enter to select · ↑/↓ to navigate ·
// Esc to cancel" and "ctrl+g to edit in Notepad · <plan path> · /plan to preview".
const claudePromptFooter =
  /\bEnter to (?:confirm|select)\b.*\bEsc to cancel\b|\bEsc to cancel\b.*\bTab to amend\b|\bctrl\+g to edit\b.*\/plan to preview/i;
// Answering a harness prompt from the phone is allowed only for Claude Code's trust
// and permission prompts, where Enter takes the default "Yes" and Esc declines. Question
// and plan-approval prompts are excluded: their first choice is not a plain yes.
const claudePermissionFooter = /\bEnter to confirm\b.*\bEsc to cancel\b|\bEsc to cancel\b.*\bTab to amend\b/i;

export type AnswerPlan = { keys: string[]; reason: "" } | { keys: []; reason: string };

export function answerPlan(tool: string, terminal: Terminal | undefined, title: string, answer: string): AnswerPlan {
  if (tool !== "claude-code") return { keys: [], reason: "answering from the phone is only verified for Claude Code" };
  if (!terminal || readHarnessSignal(tool, terminal, title) !== "input") return { keys: [], reason: "the harness is not waiting for an answer" };
  if (!lastTextRows(terminal, 2).some((row) => claudePermissionFooter.test(row))) return { keys: [], reason: "this prompt is a question or plan; answer it on the PC" };
  if (answer !== "yes") return { keys: ["\u001b"], reason: "" };
  if (!highlightedIsYes(lastRows(terminal, 14))) return { keys: [], reason: "the highlighted choice is not Yes; answer it on the PC" };
  return { keys: ["\r"], reason: "" };
}

// Enter picks the highlighted option, and Claude's trust prompt highlights "No, exit"
// by default, so Yes is sent only when the highlighted row is a Yes choice.
export function highlightedIsYes(rows: string[]): boolean {
  const marked = rows.find((row) => /^[❯>]\s*/.test(row));
  return !!marked && /^[❯>]\s*(\d+\.\s*)?Yes\b/.test(marked);
}

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

// The last non-empty rows within the bottom six; Claude Code does not always draw
// its footer on the very last screen row.
function lastTextRows(terminal: Terminal, count: number): string[] {
  const buffer = terminal.buffer.active;
  const end = buffer.baseY + terminal.rows;
  const rows: string[] = [];
  for (let index = end - 1; index >= end - 6 && rows.length < count; index--) {
    const text = buffer.getLine(index)?.translateToString(true).trim();
    if (text) rows.push(text);
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
// readScreen returns the last `count` visible rows as plain text (the terminal keeps no scrollback).
export function readScreen(terminal: Terminal, count: number): string[] {
  const buffer = terminal.buffer.active;
  const end = buffer.baseY + terminal.rows;
  const rows: string[] = [];
  for (let index = Math.max(buffer.baseY, end - count); index < end; index++) {
    rows.push(buffer.getLine(index)?.translateToString(true) ?? "");
  }
  return rows;
}

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

export const hasCueSignal = (tool: string) => tool === "claude-code" || tool === "copilot-cli";

export type ControlSendPlan = { allowed: true } | { allowed: false; reason: string };

// Re-check on the live screen for a send that arrived over the control channel.
// Go already required a confirmed state; the window refuses again because the
// screen may have changed in between. Nothing is typed unless this allows it.
export function controlSendPlan(signal: HarnessSignal, submit: boolean, draft: string): ControlSendPlan {
  if (signal === undefined) return { allowed: false, reason: "the harness shows no readable state" };
  if (signal === "input") return { allowed: false, reason: "the harness is waiting for an answer" };
  if (submit && signal !== "idle") return { allowed: false, reason: "the harness is working" };
  if (submit && draft.trim()) return { allowed: false, reason: "there is unsent text in the input line" };
  return { allowed: true };
}

export type ReportedState = { state: "working" | "input" | "idle" | "unknown"; source: "cue" | "heuristic" };

// What the window tells Go about a session. Only a harness cue is "cue"; output
// timing is "heuristic" and Go will not let automation act on it. A "working"
// cue that went stale (no output for staleWorkingMs) is reported as unknown.
export function reportedState(signal: HarnessSignal, activity: Activity): ReportedState {
  if (signal === "input") return { state: "input", source: "cue" };
  if (signal === "idle") return { state: "idle", source: "cue" };
  if (signal === "working") return activity.working ? { state: "working", source: "cue" } : { state: "unknown", source: "cue" };
  return { state: activity.working ? "working" : "idle", source: "heuristic" };
}

// Harness-specific cues only: terminal title for Claude Code, the status/footer
// row (last two rows, never the whole screen) for Copilot CLI.
export function readHarnessSignal(tool: string, terminal: Terminal | undefined, title: string): HarnessSignal {
  if (tool === "claude-code") {
    if (terminal && lastTextRows(terminal, 2).some((row) => claudePromptFooter.test(row))) return "input";
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
