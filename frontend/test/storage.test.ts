import { beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import { readRecentFolders, readTabs, recentFoldersKey, savedTabsKey, writeStored } from "../src/storage.ts";

let data: Map<string, string>;
let failWrites = false;
let reports: string[];
const report = (source: string) => { reports.push(source); };

beforeEach(() => {
  data = new Map();
  failWrites = false;
  reports = [];
  (globalThis as { localStorage?: unknown }).localStorage = {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => {
      if (failWrites) throw new Error("QuotaExceededError");
      data.set(key, value);
    },
  };
});

test("a fresh or cleared WebView profile yields empty defaults without reports", () => {
  assert.deepEqual(readTabs(report), []);
  assert.deepEqual(readRecentFolders(report), []);
  assert.deepEqual(reports, []);
});

test("corrupt JSON falls back to defaults and is reported", () => {
  data.set(savedTabsKey, "{not json");
  data.set(recentFoldersKey, "][");
  assert.deepEqual(readTabs(report), []);
  assert.deepEqual(readRecentFolders(report), []);
  assert.deepEqual(reports, ["storage.tabs", "storage.recent-folders"]);
});

test("wrong shapes are filtered, normalized and de-duplicated", () => {
  data.set(savedTabsKey, JSON.stringify([{ id: "a", name: "A", extra: 1 }, null, 7, { id: "a", name: "dup" }, { id: "b" }, { id: "c", name: "C" }]));
  data.set(recentFoldersKey, JSON.stringify(["C:\\one", "", 3, "C:\\two", "d", "e", "f", "g"]));
  assert.deepEqual(readTabs(report), [{ id: "a", name: "A" }, { id: "c", name: "C" }]);
  assert.deepEqual(readRecentFolders(report), ["C:\\one", "C:\\two", "d", "e", "f"]);
  assert.deepEqual(readTabs(() => assert.fail("no report expected")).length, 2);
});

test("a non-array value is ignored", () => {
  data.set(savedTabsKey, JSON.stringify({ id: "a", name: "A" }));
  assert.deepEqual(readTabs(report), []);
});

test("written state round-trips and a failing write is reported, not thrown", () => {
  writeStored(savedTabsKey, [{ id: "a", name: "A" }], report);
  assert.deepEqual(readTabs(report), [{ id: "a", name: "A" }]);
  failWrites = true;
  writeStored(savedTabsKey, [], report);
  assert.deepEqual(reports, [`storage.write.${savedTabsKey}`]);
});
