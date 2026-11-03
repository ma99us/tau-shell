import assert from "node:assert/strict";
import test from "node:test";
import type { Session } from "../src/bridge.ts";
import { flattenWorkspaces, groupSessions, readCollapsed, summarize, summaryText, writeCollapsed } from "../src/workspaces.ts";

const make = (id: string, over: Partial<Session> = {}): Session => ({ id, name: id, tool: "claude-code", workingDir: "C:/x", status: "stopped", runningHere: false, hasResumeId: false, autoResume: false, ...over } as Session);
const groups = {
  a1: { key: "a", name: "A", path: "C:/a", repo: true },
  b1: { key: "b", name: "B", path: "C:/b", repo: false },
  a2: { key: "a", name: "A", path: "C:/a", repo: true },
};

test("groups follow first appearance and keep the shared order inside a group", () => {
  const result = groupSessions([make("a1"), make("b1"), make("a2"), make("new")], groups);
  assert.deepEqual(result.map((w) => [w.group.key, w.sessions.map((s) => s.id)]), [["a", ["a1", "a2"]], ["b", ["b1"]], ["ungrouped", ["new"]]]);
  assert.deepEqual(flattenWorkspaces(result).map((s) => s.id), ["a1", "a2", "b1", "new"]);
});

test("summary follows the Resume All rule and counts attention only for running sessions", () => {
  const list = [make("r", { runningHere: true }), make("ok", { hasResumeId: true }), make("warn", { hasResumeId: true, resumeNote: "gone" }), make("none")];
  const summary = summarize(list, (s) => s.id === "r");
  assert.deepEqual([summary.running, summary.needsAttention, summary.resumable.map((s) => s.id), summary.stoppable.length], [1, 1, ["ok"], 1]);
  assert.equal(summaryText(summary), "4 sessions, 1 running, 1 needs attention");
});

test("collapsed state round-trips and ignores corrupt data", () => {
  let saved = "";
  writeCollapsed({ setItem: (_k, v) => { saved = v; } }, new Set(["a"]));
  assert.deepEqual([...readCollapsed({ getItem: () => saved })], ["a"]);
  assert.equal(readCollapsed({ getItem: () => "{oops" }).size, 0);
  assert.equal(readCollapsed({ getItem: () => '{"a":1}' }).size, 0);
});
