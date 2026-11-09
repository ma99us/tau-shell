import { test } from "node:test";
import assert from "node:assert/strict";
import { APP_SCOPE, addToast, clearToasts, dismissToast, toastsFor, type Toast } from "../src/toasts.ts";

const toast = (id: number, scope: string, tone: "info" | "error", message = "m"): Toast => ({ id, scope, tone, message });

test("a scope keeps one toast per tone and a newer one replaces the older", () => {
  let list: Toast[] = [];
  list = addToast(list, toast(1, "a", "error", "first"));
  list = addToast(list, toast(2, "a", "info"));
  list = addToast(list, toast(3, "a", "error", "second"));
  assert.deepEqual(toastsFor(list, "a").map((t) => t.id), [2, 3]);
});

test("session toasts stay out of the app scope and other sessions", () => {
  const list = [toast(1, APP_SCOPE, "info"), toast(2, "a", "error"), toast(3, "b", "error")];
  assert.deepEqual(toastsFor(list, APP_SCOPE).map((t) => t.id), [1]);
  assert.deepEqual(toastsFor(list, "a").map((t) => t.id), [2]);
});

test("dismiss and clear remove only what they name", () => {
  const list = [toast(1, "a", "error"), toast(2, "a", "info"), toast(3, "b", "error")];
  assert.deepEqual(dismissToast(list, 2).map((t) => t.id), [1, 3]);
  assert.deepEqual(clearToasts(list, "a", "error").map((t) => t.id), [2, 3]);
  assert.deepEqual(clearToasts(list, "a").map((t) => t.id), [3]);
});