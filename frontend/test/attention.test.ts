// Run with `npm test` (Node runs this TypeScript directly; no test framework).
import { test } from "node:test";
import assert from "node:assert/strict";
import type { Terminal } from "@xterm/xterm";
import { attentionEdge, composeInjection, nextActivity, readHarnessSignal, staleWorkingMs, workingWindowMs } from "../src/attention.ts";

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
});

test("Claude cues come from the title, with the confirm footer meaning input", () => {
  const plain = fakeTerminal(["", ""]);
  assert.equal(readHarnessSignal("claude-code", plain, "\u2810 Claude Code"), "working");
  assert.equal(readHarnessSignal("claude-code", plain, "\u25D1 Claude Code"), "working");
  assert.equal(readHarnessSignal("claude-code", plain, "\u2733 Claude Code"), "idle");
  assert.equal(readHarnessSignal("claude-code", fakeTerminal(["Enter to confirm · Esc to cancel", ""]), "\u2733 Claude Code"), "input");
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
  assert.equal(composeInjection("", "/compact", true), "/compact");
  assert.equal(composeInjection("fix it", "add tests", true), ". add tests");
  assert.equal(composeInjection("", "review", false), "review. ");
});
