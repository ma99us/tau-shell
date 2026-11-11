import type { Terminal } from "@xterm/xterm";
import { cuePattern, findHarness } from "./harnesses.ts";

// "working" / "idle" / "input" (waiting for the user to answer a prompt) when
// the harness itself says so; undefined when the harness gives no usable cue
// (callers fall back to output timing).
export type HarnessSignal = "working" | "idle" | "input" | undefined;
export type SendPlan = "blocked" | "submit" | "insert";

export function planSend(signal: HarnessSignal, submit: boolean, draft = ""): SendPlan {
  if (signal === "input") return "blocked";
  // An empty prompt is submitted whatever the harness shows (a busy harness queues it); an unsent draft is never sent.
  return submit && !draft.trim() ? "submit" : "insert";
}

export type AttentionKind = "input" | "done";
export const minWorkMsForDone = 3000;

export type Activity = { working: boolean; input: boolean; unseen: boolean };
// Output newer than this means the session is "working"; older means "quiet".
export const workingWindowMs = 2500;
// A visual "working" cue with no output for this long is treated as stale: a live
// harness animates its cue (spinner/timer), which produces output.
export const staleWorkingMs = 15000;

// One poll step for a running session. The harness cue wins; output timing is the
// fallback when the harness gives none. lastOutputAt excludes keystroke echo.
// "seen" means the session is selected and TauShell has focus.
export function nextActivity(old: Activity, signal: HarnessSignal, lastOutputAt: number | undefined, now: number, seen: boolean, cueTool = false, activityReader = false): Activity {
  const outputAge = lastOutputAt === undefined ? Infinity : now - lastOutputAt;
  // Reliable cues and log readers never fall back to output timing: idle redraws
  // would otherwise look like work and raise false "done" alerts.
  const working = signal === undefined ? !cueTool && !activityReader && outputAge < workingWindowMs :
    signal === "working" && (activityReader || outputAge <= staleWorkingMs);
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

// Cue rules live in the harness manifests (see builtin claude-code.yaml for the
// observed Claude Code title and footer behavior). Answering a harness prompt from
// the phone is allowed only for Claude Code's trust and permission prompts, where
// Enter takes the default "Yes" and Esc declines. Question and plan-approval
// prompts are excluded: their first choice is not a plain yes.
const claudePermissionFooter = /\bEnter to confirm\b.*\bEsc to cancel\b|\bEsc to cancel\b.*\bTab to amend\b/i;

export type AnswerPlan = { keys: string[]; reason: "" } | { keys: []; reason: string };

export function answerPlan(tool: string, terminal: Terminal | undefined, title: string, answer: string): AnswerPlan {
  const reader = findHarness(tool)?.phoneAnswer;
  if (reader === "copilot-permission") return copilotAnswerPlan(terminal, title, answer);
  if (reader !== "claude-permission") return { keys: [], reason: "answering from the phone is only verified for Claude Code and Copilot CLI" };
  if (!terminal || readHarnessSignal(tool, terminal, title) !== "input") return { keys: [], reason: "the harness is not waiting for an answer" };
  if (!lastTextRows(terminal, 2).some((row) => claudePermissionFooter.test(row))) return { keys: [], reason: "this prompt is a question or plan; answer it on the PC" };
  if (answer !== "yes") return { keys: ["\u001b"], reason: "" };
  if (!highlightedIsYes(lastRows(terminal, 14))) return { keys: [], reason: "the highlighted choice is not Yes; answer it on the PC" };
  return { keys: ["\r"], reason: "" };
}

// A waiting harness shows either a permission prompt or a question/choice list. Only
// the two verified harnesses can tell them apart; anything else is "unknown" and is
// never treated as a question.
export function promptKind(tool: string, terminal: Terminal): "permission" | "question" | "unknown" {
  const reader = findHarness(tool)?.phoneAnswer;
  if (reader === "copilot-permission") return copilotPermission(lastRows(terminal, 24)).permission ? "permission" : "question";
  if (reader === "claude-permission") return lastTextRows(terminal, 2).some((row) => claudePermissionFooter.test(row)) ? "permission" : "question";
  return "unknown";
}

// Enter picks the highlighted option, and Claude's trust prompt highlights "No, exit"
// by default, so Yes is sent only when the highlighted row is a Yes choice.
export function highlightedIsYes(rows: string[]): boolean {
  const marked = rows.find((row) => /^[❯>]\s*/.test(row));
  return !!marked && /^[❯>]\s*(\d+\.\s*)?Yes\b/.test(marked);
}

// Copilot CLI's permission box (observed on 1.0.105) reads "Do you want to <action>?" with
// "1. Yes", "2. Yes, and don't ask again ...", "3. No, and tell Copilot ... (Esc to stop)" and
// the same "enter to select · esc to cancel" footer as its other pickers, so the question and
// the No row are required too. Rows sit inside a box, so the border is stripped first.
export function copilotPermission(rows: string[]): { permission: boolean; yesHighlighted: boolean } {
  const inner = rows.map((row) => row.replace(/^[│|\s]+/, "").replace(/[│|\s]+$/, ""));
  const permission = inner.some((row) => /^Do you want to\b/.test(row)) && inner.some((row) => /^(?:[❯>]\s*)?\d+\.\s*No\b/.test(row));
  // Only the plain first "Yes": option 2 grants a standing permission.
  const yesHighlighted = inner.some((row) => /^[❯>]\s*1\.\s*Yes$/.test(row));
  return { permission, yesHighlighted };
}

function copilotAnswerPlan(terminal: Terminal | undefined, title: string, answer: string): AnswerPlan {
  if (!terminal || readHarnessSignal("copilot-cli", terminal, title) !== "input") return { keys: [], reason: "the harness is not waiting for an answer" };
  const found = copilotPermission(lastRows(terminal, 24));
  if (!found.permission) return { keys: [], reason: "this prompt is not a permission request; answer it on the PC" };
  if (answer !== "yes") return { keys: ["\u001b"], reason: "" };
  if (!found.yesHighlighted) return { keys: [], reason: "the highlighted choice is not a plain Yes; answer it on the PC" };
  return { keys: ["\r"], reason: "" };
}

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

// OpenCode draws its input as an indented box: a bare bar row, the text rows, a bare bar
// row, a status row ("Build - model"), then a bottom border row. The box floats mid-screen
// on an empty conversation, so find the border anywhere in view and read the rows above it.
function readOpenCodeDraft(terminal: Terminal): string | undefined {
  const buffer = terminal.buffer.active;
  const end = buffer.baseY + terminal.rows;
  const cell = buffer.getNullCell();
  for (let border = end - 1; border >= Math.max(0, end - 40); border--) {
    if (!/^\s*\u2579\u2580+\s*$/.test(buffer.getLine(border)?.translateToString(true) ?? "")) continue;
    // border-1 is the status row and border-2 the bottom padding; the text rows end at border-3
    // and the blank top padding row closes the box.
    const rows: string[] = [];
    for (let y = border - 3; y >= 0; y--) {
      const line = buffer.getLine(y);
      const marker = line && /^\s*\u2503(?:\s\s|$)/.exec(line.translateToString(true));
      if (!line || !marker) break;
      let text = "";
      for (let x = marker[0].length; x < line.length; x++) {
        const current = line.getCell(x, cell);
        if (!current || current.isDim() || current.isInverse()) continue;
        text += current.getChars() || " ";
      }
      if (y !== border - 3 && !text.trim()) break;
      rows.unshift(text.trim());
    }
    // The placeholder is drawn in plain gray, not dim, so recognise its fixed lead-in.
    const draft = rows.join(" ").trim();
    return /^Ask anything\u2026/.test(draft) ? "" : draft;
  }
  return undefined;
}

export function readDraft(terminal: Terminal): string {
  const openCode = readOpenCodeDraft(terminal);
  if (openCode !== undefined) return openCode;
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
  if (/^\s*[/!]/.test(text)) {
    if (!draft.trim()) return text;
    return `${draft}${/\s$/.test(draft) ? "" : " "}${text}`;
  }
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

export const hasCueSignal = (tool: string) => (findHarness(tool)?.cues?.length ?? 0) > 0;

export type ControlSendPlan = { allowed: true } | { allowed: false; reason: string };

// Re-check on the live screen for a send that arrived over the control channel.
// Go already required a confirmed state; the window refuses again because the
// screen may have changed in between. Nothing is typed unless this allows it.
export function controlSendPlan(signal: HarnessSignal, submit: boolean, draft: string, question = false, allowBusySubmit = false): ControlSendPlan {
  if (signal === undefined) return { allowed: false, reason: "the harness shows no readable state" };
  if (signal === "input") return question ? { allowed: true } : { allowed: false, reason: "the harness is waiting for an answer" };
  if (submit && signal !== "idle" && !(allowBusySubmit && signal === "working")) return { allowed: false, reason: "the harness is working" };
  if (submit && draft.trim() && !allowBusySubmit) return { allowed: false, reason: "there is unsent text in the input line" };
  return { allowed: true };
}

export const logActivityTtlMs = 5000;
// Latest harness-log busy/idle answer per session; main.tsx refreshes it, the queue and sends read it.
export const logActivityCache = new Map<string, { state: string; at: number }>();
export function freshLogState(id: string, now: number): string | undefined {
  const log = logActivityCache.get(id);
  return log && now - log.at <= logActivityTtlMs ? log.state : undefined;
}

// The harness's own log is the most generic busy/idle source, so it beats screen
// text for working/idle; a visible prompt on screen (input) still wins because the
// log does not record it. Without a fresh log answer the screen cue is used.
export function combineSignal(screen: HarnessSignal, log: string | undefined): HarnessSignal {
  if (screen === "input") return screen;
  if (log === "working" || log === "idle") return log;
  return screen;
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

// Cues come from the harness manifest: ordered rules, first match wins. Sources:
// the terminal title, the last N screen rows (never the whole screen), or the last
// N non-empty rows within the bottom six.
export function readHarnessSignal(tool: string, terminal: Terminal | undefined, title: string): HarnessSignal {
  for (const cue of findHarness(tool)?.cues ?? []) {
    const pattern = cuePattern(cue);
    if (!pattern) continue;
    let candidates: string[];
    if (cue.source === "title") candidates = [title];
    else if (!terminal) continue;
    else if (cue.source === "last-rows") candidates = lastRows(terminal, cue.rows ?? 1);
    else candidates = lastTextRows(terminal, cue.rows ?? 1);
    if (candidates.some((candidate) => pattern.test(candidate))) return cue.state;
  }
  return undefined;
}
