import assert from "node:assert/strict";
import test from "node:test";
import { chooseIde, readIdePick, writeIdePick, type IdeOption } from "../src/idePick.ts";

const option = (id: string): IdeOption => ({ id, name: id, icon: "", matched: false });
const options = [option("idea"), option("vscode"), option("cursor")];

test("IDE pick prefers the session choice, then the global one, then the first match", () => {
  assert.equal(chooseIde(options, "cursor", "vscode")?.id, "cursor");
  assert.equal(chooseIde(options, "gone", "vscode")?.id, "vscode");
  assert.equal(chooseIde(options, null, "gone")?.id, "idea");
  assert.equal(chooseIde([], "cursor", "vscode"), undefined);
});

test("IDE pick is remembered per session and globally", () => {
  const data = new Map<string, string>();
  const store = { getItem: (key: string) => data.get(key) ?? null, setItem: (key: string, value: string) => { data.set(key, value); } };
  writeIdePick(store, "a", "cursor");
  assert.equal(readIdePick(store, "a", options)?.id, "cursor");
  assert.equal(readIdePick(store, "b", options)?.id, "cursor");
  writeIdePick(store, "b", "vscode");
  assert.equal(readIdePick(store, "a", options)?.id, "cursor");
});
