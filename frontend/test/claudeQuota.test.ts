import { test } from "node:test";
import assert from "node:assert/strict";
import { claudeQuotaView, formatDollars, parseBudgetInput } from "../src/claudeQuota.ts";
import type { ClaudeQuota } from "../src/claudeQuota.ts";
import { claudeBudgetKey, readClaudeBudget } from "../src/storage.ts";

const quota: ClaudeQuota = { host: "proxy", spend: 660, budget: 3000, budgetSource: "member", daysLeft: 22, projected: 2400, canProject: true, teamSpend: 700, teamBudget: 5000 };

test("a manual budget replaces the proxy budget and drives percent and warning", () => {
  const proxy = claudeQuotaView(quota, null);
  assert.deepEqual([proxy.budget, Math.round(proxy.percent), proxy.overridden, proxy.overBudget], [3000, 22, false, false]);
  const manual = claudeQuotaView(quota, 2000);
  assert.deepEqual([manual.budget, Math.round(manual.percent), manual.overridden, manual.overBudget], [2000, 33, true, true]);
});

test("no budget never warns or divides by zero", () => {
  const view = claudeQuotaView({ ...quota, budget: 0, budgetSource: "none" }, null);
  assert.deepEqual([view.percent, view.overBudget], [0, false]);
});

test("budget input accepts dollars and commas, anything else clears", () => {
  assert.equal(parseBudgetInput("$2,000"), 2000);
  assert.equal(parseBudgetInput(" 1500.456 "), 1500.46);
  assert.equal(parseBudgetInput(""), null);
  assert.equal(parseBudgetInput("abc"), null);
  assert.equal(parseBudgetInput("-5"), null);
  assert.equal(formatDollars(2476.4), "$2,476");
});

test("stored budget ignores junk", () => {
  const storage = (value: string | null) => ({ getItem: (key: string) => (key === claudeBudgetKey ? value : null) });
  assert.equal(readClaudeBudget(storage("2000")), 2000);
  assert.equal(readClaudeBudget(storage(null)), null);
  assert.equal(readClaudeBudget(storage("\"x\"")), null);
  assert.equal(readClaudeBudget(storage("{bad")), null);
  assert.equal(readClaudeBudget(storage("-1")), null);
});
