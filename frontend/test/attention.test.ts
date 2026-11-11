// Run with `npm test` (Node runs this TypeScript directly; no test framework).
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import type { Terminal } from "@xterm/xterm";
import { answerPlan, attentionEdge, composeInjection, controlSendPlan, copilotPermission, hasCueSignal, highlightedIsYes, nextActivity, planSend, promptKind, readHarnessSignal, reportedState, staleWorkingMs, workingWindowMs } from "../src/attention.ts";
import { hasActivityReader, setHarnesses, type HarnessInfo } from "../src/harnesses.ts";

// The same JSON the host serves; a Go test keeps it equal to the built-in manifests.
setHarnesses(JSON.parse(readFileSync(new URL("./fixtures/harnesses.json", import.meta.url), "utf8")) as HarnessInfo[]);

test("cues and phone answers come from the manifests", () => {
  assert.equal(hasCueSignal("claude-code"), true);
  assert.equal(hasCueSignal("copilot-cli"), true);
  assert.equal(hasCueSignal("gsd"), false);
  assert.equal(hasActivityReader("gsd"), true);
  assert.equal(hasCueSignal("unknown-harness"), false);
  assert.equal(readHarnessSignal("gsd", fakeTerminal(["", "? help"]), "\u2733"), undefined);
  assert.match(answerPlan("gsd", undefined, "", "yes").reason, /only verified/);
});

test("Yes is sent only when the highlighted choice is Yes", () => {
  assert.equal(highlightedIsYes(["Enter to confirm · Esc to cancel", "  Yes, I trust this folder", "❯ No, exit"]), false);
  assert.equal(highlightedIsYes(["❯ 1. Yes", "  2. No"]), true);
  assert.equal(highlightedIsYes(["  1. Yes", "  2. No"]), false);
  assert.deepEqual(answerPlan("copilot-cli", undefined, "", "yes").keys, []);
});

test("Copilot permission boxes are recognised and Yes needs the plain first choice", () => {
  const box = (marked: number) => [
    "│ Do you want to create C:\\tmp\\hello.txt?                       │",
    `│ ${marked === 1 ? "❯" : " "} 1. Yes                                                  │`,
    `│ ${marked === 2 ? "❯" : " "} 2. Yes, and don't ask again for file operations         │`,
    `│ ${marked === 3 ? "❯" : " "} 3. No, and tell Copilot what to do differently (Esc to stop) │`,
    "│ ↑/↓ to navigate · enter to select · esc to cancel              │",
  ];
  assert.deepEqual(copilotPermission(box(1)), { permission: true, yesHighlighted: true });
  assert.deepEqual(copilotPermission(box(2)), { permission: true, yesHighlighted: false });
  assert.deepEqual(copilotPermission(box(3)), { permission: true, yesHighlighted: false });
  // A plain picker with the same footer is not a permission request.
  assert.equal(copilotPermission(["│ Pick a model │", "│ ❯ 1. Yes-model │", "│ enter to select · esc to cancel │"]).permission, false);
});

test("reportedState marks only harness cues as cue and stale working as unknown", () => {
  const busy = { working: true, input: false, unseen: false };
  const quiet = { working: false, input: false, unseen: false };
  assert.deepEqual(reportedState("idle", quiet), { state: "idle", source: "cue" });
  assert.deepEqual(reportedState("input", quiet), { state: "input", source: "cue" });
  assert.deepEqual(reportedState("working", busy), { state: "working", source: "cue" });
  assert.deepEqual(reportedState("working", quiet), { state: "unknown", source: "cue" });
  assert.deepEqual(reportedState(undefined, busy), { state: "working", source: "heuristic" });
  assert.deepEqual(reportedState(undefined, quiet), { state: "idle", source: "heuristic" });
});

test("controlSendPlan refuses unsafe control sends and allows explicit mobile sends during work", () => {
  assert.equal(controlSendPlan(undefined, false, "").allowed, false);
  assert.equal(controlSendPlan("input", false, "").allowed, false);
  assert.equal(controlSendPlan("working", true, "").allowed, false);
  assert.equal(controlSendPlan("idle", true, "half typed").allowed, false);
  assert.equal(controlSendPlan("idle", true, "").allowed, true);
  assert.equal(controlSendPlan("working", false, "draft").allowed, true);
  assert.equal(controlSendPlan("input", true, "", true).allowed, true);
  assert.equal(controlSendPlan("working", true, "", false, true).allowed, true);
  assert.equal(controlSendPlan("idle", true, "half typed", false, true).allowed, true);
  assert.equal(controlSendPlan("input", true, "", false, true).allowed, false);
});

test("promptKind separates permission prompts from questions on verified harnesses only", () => {
  const permission = fakeTerminal(["Do you want to proceed?", "Esc to cancel · Tab to amend", ""]);
  const picker = fakeTerminal(["1. Red", "2. Blue", "Enter to select · ↑/↓ to navigate · Esc to cancel"]);
  assert.equal(promptKind("claude-code", permission), "permission");
  assert.equal(promptKind("claude-code", picker), "question");
  assert.equal(promptKind("copilot-cli", fakeTerminal(["Do you want to run ls?", "❯ 1. Yes", "  2. Yes, and don't ask again", "  3. No, and tell Copilot", "enter to select · esc to cancel"])), "permission");
  assert.equal(promptKind("copilot-cli", picker), "question");
  assert.equal(promptKind("gsd", picker), "unknown");
});

// Minimal xterm buffer: only what the signal reader touches.
function fakeTerminal(rows: string[]): Terminal {
  return {
    rows: rows.length,
    buffer: { active: { baseY: 0, getLine: (index: number) => rows[index] === undefined ? undefined : { translateToString: () => rows[index] } } },
  } as unknown as Terminal;
}

const idle = { working: false, input: false, unseen: false };
const busy = { working: true, input: false, unseen: false };

test("Copilot cues come from the last two rows only", () => {
  assert.equal(readHarnessSignal("copilot-cli", fakeTerminal(["Working (3s) esc to cancel", "", "x"]), ""), undefined);
  assert.equal(readHarnessSignal("copilot-cli", fakeTerminal(["", "Working (3s) esc to cancel", "x"]), ""), "working");
  assert.equal(readHarnessSignal("copilot-cli", fakeTerminal(["", "x", "? help"]), ""), "idle");
  assert.equal(readHarnessSignal("copilot-cli", fakeTerminal(["", "navigate · enter to select · esc to cancel", "? help"]), ""), "input");
  // The ask_user form uses a different footer than the selection prompt.
  assert.equal(readHarnessSignal("copilot-cli", fakeTerminal(["", "↑/↓ select · enter accept · tab next · ctrl+d decline · esc cancel", ""]), ""), "input");
});

test("Claude cues come from the title, with the confirm footer meaning input", () => {
  const plain = fakeTerminal(["", ""]);
  assert.equal(readHarnessSignal("claude-code", plain, "\u2810 Claude Code"), "working");
  assert.equal(readHarnessSignal("claude-code", plain, "\u25D1 Claude Code"), "working");
  assert.equal(readHarnessSignal("claude-code", plain, "\u2733 Claude Code"), "idle");
  assert.equal(readHarnessSignal("claude-code", fakeTerminal(["Enter to confirm · Esc to cancel", ""]), "\u2733 Claude Code"), "input");
  assert.equal(readHarnessSignal("claude-code", fakeTerminal(["Do you want to proceed?", "Esc to cancel · Tab to amend", "", ""]), "\u2733 Claude Code"), "input");
  assert.equal(readHarnessSignal("claude-code", fakeTerminal(["Enter to select · ↑/↓ to navigate · Esc to cancel", "", ""]), "\u2733 Claude Code"), "input");
  assert.equal(readHarnessSignal("claude-code", fakeTerminal(["ctrl+g to edit in Notepad · ~\\.claude\\plans\\x.md  ⎿  /plan to preview"]), "\u2733 Claude Code"), "input");
  assert.equal(readHarnessSignal("unknown-tool", plain, "\u2810"), undefined);
});

test("harness cue wins over output timing", () => {
  const now = 100_000;
  assert.equal(nextActivity(idle, "idle", now - 10, now, false).working, false);
  assert.equal(nextActivity(idle, "working", now - 1000, now, false).working, true);
});

test("a working cue with no recent output is stale", () => {
  const now = 100_000;
  assert.equal(nextActivity(busy, "working", now - staleWorkingMs, now, false).working, true);
  assert.equal(nextActivity(busy, "working", now - staleWorkingMs - 1, now, false).working, false);
  assert.equal(nextActivity(idle, "working", undefined, now, false).working, false);
});

test("cue tools ignore output timing when the cue is missing", () => {
  const now = 100_000;
  assert.equal(nextActivity(idle, undefined, now - 10, now, false, true).working, false);
  assert.equal(nextActivity(idle, undefined, now - 10, now, false, false).working, true);
});

test("activity readers ignore idle redraws and trust their working state", () => {
  const now = 100_000;
  assert.equal(nextActivity(idle, undefined, now - 10, now, false, false, true).working, false);
  assert.equal(nextActivity(idle, "working", undefined, now, false, false, true).working, true);
});

test("without a cue, recent output means working", () => {
  const now = 100_000;
  assert.equal(nextActivity(idle, undefined, now - workingWindowMs + 1, now, false).working, true);
  assert.equal(nextActivity(idle, undefined, now - workingWindowMs, now, false).working, false);
  assert.equal(nextActivity(idle, undefined, undefined, now, false).working, false);
});

test("finishing or prompting while unseen glows until seen", () => {
  const now = 100_000;
  assert.equal(nextActivity(busy, "idle", now, now, false).unseen, true);
  assert.equal(nextActivity(idle, "input", now, now, false).unseen, true);
  assert.equal(nextActivity(busy, "idle", now, now, true).unseen, false);
  assert.equal(nextActivity({ ...idle, unseen: true }, "idle", now, now, true).unseen, false);
  assert.equal(nextActivity({ ...idle, unseen: true }, "idle", now, now, false).unseen, true);
});

test("alerts fire once on edges", () => {
  assert.equal(attentionEdge(idle, { working: false, input: true }, 0), "input");
  assert.equal(attentionEdge({ ...idle, input: true }, { working: false, input: true }, 0), undefined);
  assert.equal(attentionEdge(busy, { working: false, input: false }, 5000), "done");
  assert.equal(attentionEdge(busy, { working: false, input: false }, 1000), undefined);
});

test("injected text joins the draft", () => {
  assert.equal(planSend("idle", true, "existing prompt"), "insert");
  assert.equal(planSend("idle", true, ""), "submit");
  assert.equal(planSend("working", true, ""), "submit");
  assert.equal(planSend(undefined, true, ""), "submit");
  assert.equal(planSend(undefined, true, "draft"), "insert");
  assert.equal(planSend(undefined, false, ""), "insert");
  assert.equal(planSend("input", true, ""), "blocked");
  assert.equal(composeInjection("", "/compact", true), "/compact");
  assert.equal(composeInjection("existing prompt", "/compact", false), "existing prompt /compact");
  assert.equal(composeInjection("existing prompt", "/model gpt-6-luna", true), "existing prompt /model gpt-6-luna");
  assert.equal(composeInjection("existing prompt ", "/compact", false), "existing prompt /compact");
  assert.equal(composeInjection("fix it", "add tests", true), ". add tests");
  assert.equal(composeInjection("", "review", false), "review. ");
});
