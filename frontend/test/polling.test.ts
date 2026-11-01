import { test } from "node:test";
import assert from "node:assert/strict";
import { clearPollFailure, shouldReportPollFailure } from "../src/polling.ts";

test("poll failures report once until a successful response clears the episode", () => {
  const failures = new Set<string>();
  assert.equal(shouldReportPollFailure(failures, "branch"), true);
  assert.equal(shouldReportPollFailure(failures, "branch"), false);
  clearPollFailure(failures, "branch");
  assert.equal(shouldReportPollFailure(failures, "branch"), true);
});