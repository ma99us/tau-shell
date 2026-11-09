import assert from "node:assert/strict";
import test from "node:test";
import { linkProblem, linkSegments, notesProblem, parseTags } from "../src/sessionNotes.ts";

test("linkSegments finds http(s) links and trims trailing punctuation", () => {
  const segments = linkSegments("See https://example.com/pr/12. Then (https://example.com/a?b=1) and ftp://x.test/y");
  assert.deepEqual(segments.filter((segment) => segment.url).map((segment) => segment.url), ["https://example.com/pr/12", "https://example.com/a?b=1"]);
  assert.equal(segments.map((segment) => segment.text).join(""), "See https://example.com/pr/12. Then (https://example.com/a?b=1) and ftp://x.test/y");
});

test("parseTags splits on spaces and commas and removes duplicates", () => {
  assert.deepEqual(parseTags(" review, VTA-123  vta-123 https://jira.example.com/VTA-123 "), ["review", "VTA-123", "https://jira.example.com/VTA-123"]);
  assert.deepEqual(parseTags(""), []);
});

test("linkProblem accepts only http(s) addresses", () => {
  assert.equal(linkProblem(""), "");
  assert.equal(linkProblem("https://github.com/o/r/pull/1"), "");
  for (const bad of ["javascript:alert(1)", "file:///C:/x", "example.com/x", "https://a b.com", "https://"]) assert.notEqual(linkProblem(bad), "", bad);
});

test("notesProblem enforces the host limits", () => {
  assert.equal(notesProblem("ok", ["a"]), "");
  assert.notEqual(notesProblem("x".repeat(501), []), "");
  assert.notEqual(notesProblem("", Array.from({ length: 9 }, (_, index) => `t${index}`)), "");
});
