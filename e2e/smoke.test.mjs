import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { bridgeMethods, lifecycleFailures, versionFailures } from "./smoke.mjs";

test("bridgeMethods lists the methods of the WailsApp type only", () => {
  const source = [
    "type Other = {",
    "  NotMe(): void;",
    "};",
    "type WailsApp = {",
    "  GetAppVersion(): Promise<string>;",
    "  ListSessions(filter: string): Promise<Session[]>;",
    "};",
    "const x = { Ignored() {} };",
  ].join("\n");
  assert.deepEqual(bridgeMethods(source), ["GetAppVersion", "ListSessions"]);
});

test("bridgeMethods finds the real bridge contract", () => {
  const methods = bridgeMethods(readFileSync(new URL("../frontend/src/bridge.ts", import.meta.url), "utf8"));
  assert.ok(methods.includes("GetAppVersion"));
  assert.ok(methods.length > 50);
});

test("version checks require the bridge and visible Home version to match", () => {
  assert.deepEqual(versionFailures({ version: "1.0.1", displayedVersion: "v1.0.1" }, "1.0.1"), []);
  assert.deepEqual(versionFailures({ version: "dev", displayedVersion: "dev build" }, "dev"), []);
  assert.equal(versionFailures({ version: "1.0.0", displayedVersion: "v1.0.0" }, "1.0.1").length, 2);
  assert.equal(versionFailures({ version: "1.0.1" }, "1.0.1").length, 1);
});

test("lifecycle checks require auto-restore, one confirmation, stopped session, and clean exit report", () => {
  const prepare = { phase: "prepare", home: true, seedStarted: true, seedStopped: true, hasResumeId: true, confirmationCount: 0, missing: [] };
  const restore = { phase: "restore", home: true, autoRestored: true, hasResumeId: true, confirmationCount: 1, shutdownFinished: true, allStopped: true, missing: [] };
  assert.deepEqual(lifecycleFailures(prepare, restore), []);
  assert.ok(lifecycleFailures(prepare, { ...restore, confirmationCount: 2 }).some((failure) => failure.includes("exactly one")));
  assert.ok(lifecycleFailures(prepare, { ...restore, shutdownFinished: false }).some((failure) => failure.includes("did not finish")));
  assert.ok(lifecycleFailures(prepare, { ...restore, autoRestored: false }).some((failure) => failure.includes("auto-restore")));
});
