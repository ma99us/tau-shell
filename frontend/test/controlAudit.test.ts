import { test } from "node:test";
import assert from "node:assert/strict";
import { addAuditEntry, auditCaller, keepEntries } from "../src/controlAuditLog.ts";

const entry = (pid: number) => ({ time: "2026-01-01T00:00:00Z", pid, image: "taushell.exe", op: "list", outcome: "ok" });

test("audit entries are newest first and bounded", () => {
  let entries = [] as ReturnType<typeof entry>[];
  for (let pid = 1; pid <= keepEntries + 5; pid++) entries = addAuditEntry(entries, entry(pid));
  assert.equal(entries.length, keepEntries);
  assert.equal(entries[0].pid, keepEntries + 5);
});

test("audit caller falls back when the image is unknown", () => {
  assert.equal(auditCaller({ ...entry(7), image: "" }), "unknown program (PID 7)");
  assert.equal(auditCaller(entry(7)), "taushell.exe (PID 7)");
});
