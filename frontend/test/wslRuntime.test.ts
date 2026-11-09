import assert from "node:assert/strict";
import test from "node:test";
import { defaultDistro, isWindowsMount, wslChoices, wslHarnessSupported } from "../src/wslRuntime.ts";

const status = (installed: boolean, names: [string, boolean][]) => ({
  installed, distros: names.map(([name, isDefault]) => ({ name, default: isDefault, running: false, version: 2, state: "Stopped" })),
  checks: [], warnings: [],
});

test("wsl choices need an installed WSL with distros", () => {
  assert.deepEqual(wslChoices(null), []);
  assert.deepEqual(wslChoices(status(false, [["Ubuntu", true]])), []);
  assert.equal(wslChoices(status(true, [["Ubuntu", true]])).length, 1);
});

test("default distro prefers the WSL default", () => {
  assert.equal(defaultDistro(status(true, [["A", false], ["B", true]]).distros), "B");
  assert.equal(defaultDistro(status(true, [["A", false]]).distros), "A");
  assert.equal(defaultDistro([]), "");
});

test("windows mounts are detected and codex is gated", () => {
  assert.equal(isWindowsMount("/mnt/c/work"), true);
  assert.equal(isWindowsMount("/mnt/c"), true);
  assert.equal(isWindowsMount("/mnt/cache"), false);
  assert.equal(isWindowsMount("/home/me/x"), false);
  assert.equal(wslHarnessSupported("codex"), false);
  assert.equal(wslHarnessSupported("claude-code"), true);
});
