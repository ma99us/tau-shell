import assert from "node:assert/strict";
import test from "node:test";
import { formatUSD } from "../src/usage.ts";
import type { UsageTotals } from "../src/usage.ts";

const totals = (usd: number, unpriced = false) => ({ tokens: 1, usd, unpriced }) as unknown as UsageTotals;

test("prices round up to whole dollars", () => {
  assert.equal(formatUSD(totals(6.36)), "~$7");
  assert.equal(formatUSD(totals(1234.5)), "~$1,235");
  assert.equal(formatUSD(totals(0.2)), "~$1");
  assert.equal(formatUSD(totals(12)), "~$12");
  assert.equal(formatUSD(totals(0)), "~$0");
  assert.equal(formatUSD(totals(12.4, true)), "~$13+");
  assert.equal(formatUSD(totals(0, true)), "$?");
});
