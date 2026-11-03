import { filterConversations, conversationDays } from "../src/conversations.ts";
import type { Conversation } from "../src/bridge.ts";
import assert from "node:assert/strict";
import test from "node:test";

const make = (over: Partial<Conversation>): Conversation => ({ harnessSessionId: "id", title: "", model: "", firstDay: "", lastDay: "", tokens: 0, usd: 0, unpriced: false, current: false, unavailable: "", ...over });

test("filterConversations matches title, id and model case-insensitively", () => {
  const list = [make({ harnessSessionId: "aaa", title: "Fix Login" }), make({ harnessSessionId: "bbb", model: "opus" })];
  assert.equal(filterConversations(list, "login").length, 1);
  assert.equal(filterConversations(list, "BBB").length, 1);
  assert.equal(filterConversations(list, "opus")[0].harnessSessionId, "bbb");
  assert.equal(filterConversations(list, "  ").length, 2);
});

test("conversationDays describes the range", () => {
  assert.equal(conversationDays({ firstDay: "", lastDay: "" }), "no recorded usage");
  assert.equal(conversationDays({ firstDay: "2025-01-01", lastDay: "2025-01-01" }), "2025-01-01");
  assert.equal(conversationDays({ firstDay: "2025-01-01", lastDay: "2025-01-03" }), "2025-01-01 to 2025-01-03");
});
