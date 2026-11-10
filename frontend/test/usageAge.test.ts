import assert from "node:assert/strict";
import test from "node:test";
import { ageLines, formatAge, usageTooltip, type SessionUsage } from "../src/usage.ts";

const base: SessionUsage = {
  total: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, tokens: 10, usd: 12, unpriced: false },
  models: null, conversations: null,
  prices: { source: "test", updatedAt: "", stale: false, overrides: false },
};
const hour = 3_600_000;

test("ages read as days, hours or minutes", () => {
  assert.equal(formatAge(5 * 60_000), "5m");
  assert.equal(formatAge(3 * hour + 7 * 60_000), "3h 7m");
  assert.equal(formatAge(50 * hour), "2d 2h");
});

test("tooltip shows age, hourly average and per active day", () => {
  const now = Date.parse("2026-01-02T12:00:00Z");
  const lines = ageLines({ ...base, firstStarted: "2026-01-02T00:00:00Z", activeDays: 1 }, now);
  assert.match(lines[0], /^Age: 12h 0m \(first started /);
  assert.equal(lines[1], "Avg spend: ~$1.00/h over the session age");
  assert.equal(lines[2], "~$12.00 per day with usage (1 day)");
});

test("deduced starts are marked and tiny or unpriced sessions show no rate", () => {
  const now = Date.parse("2026-01-02T12:00:00Z");
  assert.match(ageLines({ ...base, firstStarted: "2026-01-02T00:00:00Z", firstStartedApprox: true }, now)[0], /^Age: ~12h.*from first logged usage day/);
  assert.equal(ageLines({ ...base, firstStarted: "2026-01-02T11:55:00Z", activeDays: 0 }, now).length, 1);
  assert.equal(ageLines({ ...base, total: { ...base.total, usd: 0 }, firstStarted: "2026-01-01T00:00:00Z" }, now).length, 1);
  assert.deepEqual(ageLines(base, now), []);
  assert.ok(usageTooltip(base, now).startsWith("Estimate from harness logs"));
});
