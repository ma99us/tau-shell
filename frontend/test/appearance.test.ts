import assert from "node:assert/strict";
import test from "node:test";
import { clampFontScale, parseAppearance, readCachedAppearance, resolveTheme } from "../src/theme/appearanceModel.ts";

test("appearance parsing keeps good values and defaults bad ones", () => {
  assert.deepEqual(parseAppearance({ theme: "light", fontScale: 1.2, terminalFontSize: 16 }), { theme: "light", fontScale: 1.2, terminalFontSize: 16 });
  assert.deepEqual(parseAppearance({ theme: "sepia", fontScale: "big", terminalFontSize: NaN }), { theme: "dark", fontScale: 1, terminalFontSize: 14 });
  assert.deepEqual(parseAppearance(null), { theme: "dark", fontScale: 1, terminalFontSize: 14 });
});

test("sizes are clamped to the supported range", () => {
  assert.equal(parseAppearance({ fontScale: 5 }).fontScale, 1.6);
  assert.equal(parseAppearance({ fontScale: 0.1 }).fontScale, 0.8);
  assert.equal(parseAppearance({ terminalFontSize: 100 }).terminalFontSize, 28);
  assert.equal(clampFontScale(1.1 + 0.1), 1.2);
});

test("system follows the OS and explicit choices ignore it", () => {
  assert.equal(resolveTheme("system", true), "dark");
  assert.equal(resolveTheme("system", false), "light");
  assert.equal(resolveTheme("light", true), "light");
  assert.equal(resolveTheme("dark", false), "dark");
});

test("a damaged cache falls back to the defaults", () => {
  assert.equal(readCachedAppearance({ getItem: () => "{not json" }).theme, "dark");
  assert.equal(readCachedAppearance({ getItem: () => JSON.stringify({ theme: "light" }) }).theme, "light");
});
