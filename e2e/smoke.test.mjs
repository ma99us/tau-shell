import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { bridgeMethods, versionFailures } from "./smoke.mjs";

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
