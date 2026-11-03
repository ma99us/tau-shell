import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { clearQueue, enqueuePrompt, getQueue, graceMs, queueDecision, queuedTotal, resumeQueue, runQueues, type QueueHooks, type QueueSession, type SessionQueue } from "../src/promptQueue.ts";
import type { HarnessSignal } from "../src/attention.ts";
import { setHarnesses, type HarnessInfo } from "../src/harnesses.ts";

setHarnesses(JSON.parse(readFileSync(new URL("./fixtures/harnesses.json", import.meta.url), "utf8")) as HarnessInfo[]);

const queue = (over: Partial<SessionQueue> = {}): SessionQueue => ({ items: [{ id: 1, text: "next" }], sentAt: 0, sawBusy: false, paused: "", ...over });

test("a queued prompt is sent only on a confirmed idle harness with an empty input line", () => {
  assert.equal(queueDecision(queue(), "claude-code", true, "idle", "", 0).action, "send");
  for (const signal of ["working", "input", undefined] as HarnessSignal[]) {
    assert.equal(queueDecision(queue(), "claude-code", true, signal, "", 0).action, "wait", String(signal));
  }
  assert.equal(queueDecision(queue(), "claude-code", true, "idle", "half typed", 0).action, "wait");
  assert.equal(queueDecision(queue({ items: [] }), "claude-code", true, "idle", "", 0).action, "wait");
});

test("a stopped session or a harness without cues pauses the queue instead of sending", () => {
  assert.equal(queueDecision(queue(), "claude-code", false, "idle", "", 0).action, "pause");
  assert.equal(queueDecision(queue(), "codex", true, "idle", "", 0).action, "pause");
  assert.equal(queueDecision(queue({ paused: "x" }), "claude-code", true, "idle", "", 0).action, "wait");
});

test("after a send the next prompt waits for the previous turn to show as busy, or a grace period", () => {
  const sent = queue({ sentAt: 1000 });
  assert.equal(queueDecision(sent, "claude-code", true, "idle", "", 1500).action, "wait");
  assert.equal(queueDecision(sent, "claude-code", true, "idle", "", 4000).action, "wait");
  assert.equal(queueDecision(sent, "claude-code", true, "idle", "", 1000 + graceMs).action, "send");
  assert.equal(queueDecision({ ...sent, sawBusy: true }, "claude-code", true, "idle", "", 4000).action, "send");
});

test("runQueues sends one prompt per finished turn, in order, and records each send", () => {
  const session: QueueSession = { id: "q1", tool: "claude-code", runningHere: true };
  let signal: HarnessSignal = "idle";
  const typed: string[] = [];
  let recorded = 0;
  const hooks: QueueHooks = {
    signalOf: () => signal,
    draftOf: () => "",
    send: (_s, text) => { typed.push(text); return ""; },
    sent: () => { recorded += 1; },
  };
  enqueuePrompt("q1", "first");
  enqueuePrompt("q1", "second");
  assert.equal(queuedTotal(), 2);
  runQueues([session], hooks, 10000);
  assert.deepEqual(typed, ["first"]);
  runQueues([session], hooks, 13000); // idle again but the first turn never showed as busy
  assert.deepEqual(typed, ["first"]);
  signal = "working";
  runQueues([session], hooks, 14000);
  signal = "idle";
  runQueues([session], hooks, 17000);
  assert.deepEqual(typed, ["first", "second"]);
  assert.equal(recorded, 2);
  assert.equal(getQueue("q1").items.length, 0);
});

test("a stopped session pauses its queue and keeps the items until resumed", () => {
  const session: QueueSession = { id: "q2", tool: "claude-code", runningHere: false };
  const hooks: QueueHooks = { signalOf: () => "idle", draftOf: () => "", send: () => { throw new Error("must not send"); }, sent: () => {} };
  enqueuePrompt("q2", "keep me");
  runQueues([session], hooks, 1000);
  assert.equal(getQueue("q2").paused, "the session is not running");
  assert.equal(getQueue("q2").items.length, 1);
  resumeQueue("q2");
  assert.equal(getQueue("q2").paused, "");
  clearQueue("q2");
  assert.equal(getQueue("q2").items.length, 0);
});

test("a refused send leaves the item waiting", () => {
  const session: QueueSession = { id: "q3", tool: "copilot-cli", runningHere: true };
  const hooks: QueueHooks = { signalOf: () => "idle", draftOf: () => "", send: () => "the harness is working", sent: () => { throw new Error("not sent"); } };
  enqueuePrompt("q3", "retry");
  runQueues([session], hooks, 1000);
  assert.equal(getQueue("q3").items.length, 1);
  clearQueue("q3");
});
