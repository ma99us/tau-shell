import assert from "node:assert/strict";
import test from "node:test";
import { progressText } from "../src/updateProgress.ts";

test("download progress shows a percentage when the size is known", () => {
  assert.equal(progressText({ phase: "downloading", done: 50, total: 200 }), "Downloading 25%");
  assert.equal(progressText({ phase: "downloading", done: 500, total: 200 }), "Downloading 100%");
});

test("unknown size and other phases use plain labels", () => {
  assert.equal(progressText({ phase: "downloading", done: 5, total: -1 }), "Downloading...");
  assert.equal(progressText({ phase: "verifying", done: 0, total: 0 }), "Verifying checksum...");
  assert.equal(progressText({ phase: "restarting", done: 0, total: 0 }), "Restarting TauShell...");
});
