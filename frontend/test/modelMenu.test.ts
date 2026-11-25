import assert from "node:assert/strict";
import test from "node:test";
import type { ModelMenu, ModelOption } from "../src/bridge.ts";
import { fallbackMenu, rankingStatus, scoreBadge, splitModels } from "../src/modelMenu.ts";

const option = (id: string, ranked: boolean, score = 0): ModelOption => ({ id, label: id, ranked, score, cost: 0, tokens: 0 });


test("splits the top ranked models from the rest", () => {
  const options = [option("a", true, 60), option("b", true, 50), option("c", true, 40), option("x", false)];
  const { top, other } = splitModels(options, 2);
  assert.deepEqual(top.map((o) => o.id), ["a", "b"]);
  assert.deepEqual(other.map((o) => o.id), ["c", "x"]);
});

test("caps the long tail", () => {
  const options = Array.from({ length: 50 }, (_, i) => option(`m${i}`, false));
  const { other, hidden } = splitModels(options);
  assert.equal(other.length, 30);
  assert.equal(hidden, 20);
});

test("marks rough estimates", () => {
  assert.equal(scoreBadge({ ...option("a", true, 61.234), confidence: "Rough estimate" }), "~61.2");
  assert.equal(scoreBadge({ ...option("a", true, 61.234), confidence: "Confident score" }), "61.2");
});

test("describes ranking availability without errors", () => {
  assert.match(rankingStatus(fallbackMenu(["a"])), /unavailable/);
  const menu: ModelMenu = { options: [], probing: false, ranking: { source: "cache", asOf: "2026-11-20", stale: true, error: "timeout" } };
  assert.equal(rankingStatus(menu), "MEI data as of 2026-11-20, stale, refresh failed");
  assert.equal(rankingStatus({ ...menu, ranking: { source: "snapshot", stale: false } }), "built-in snapshot");
});
