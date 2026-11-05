import assert from "node:assert/strict";
import test from "node:test";
import { isExpensiveModel } from "../src/modelTier.ts";

test("top-tier models are flagged expensive", () => {
  for (const id of ["claude-opus-4-1", "Claude Opus 4.8", "opus", "gpt-5-pro", "o1", "o3-pro", "gemini-2.5-pro", "grok-4-max"]) assert.ok(isExpensiveModel(id), id);
});

test("everyday and small models are not", () => {
  for (const id of ["claude-sonnet-4-5", "claude-haiku-4-5", "gpt-5", "gpt-5-mini", "o3-mini", "gemini-flash", "", "sonnet"]) assert.ok(!isExpensiveModel(id), id);
});
