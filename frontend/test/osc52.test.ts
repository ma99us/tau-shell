import assert from "node:assert/strict";
import test from "node:test";
import { parseOsc52 } from "../src/osc52.ts";

const b64 = (text: string) => Buffer.from(text, "utf8").toString("base64");

test("decodes text written to the clipboard, including unicode", () => {
  assert.equal(parseOsc52(`c;${b64("hello")}`), "hello");
  assert.equal(parseOsc52(`;${b64("héllo ✓")}`), "héllo ✓");
});

test("ignores read queries, empty and invalid payloads", () => {
  assert.equal(parseOsc52("c;?"), undefined);
  assert.equal(parseOsc52("c;"), undefined);
  assert.equal(parseOsc52("c;***"), undefined);
  assert.equal(parseOsc52("nodelimiter"), undefined);
});
