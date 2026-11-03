import assert from "node:assert/strict";
import test from "node:test";
import { highlightToHtml, languageFor, languageForFence, maxHighlightChars } from "../src/highlight.ts";
import { parseMarkdown } from "../src/markdown.ts";

test("Markdown fences carry their language label, and the Markdown viewer owns .md files", () => {
  const blocks = parseMarkdown("```ts\nconst a = 1;\n```\n\n```\nplain\n```");
  assert.deepEqual(blocks, [{ kind: "code", text: "const a = 1;", lang: "ts" }, { kind: "code", text: "plain" }]);
  assert.equal(languageForFence("ts"), "typescript");
  assert.equal(languageForFence("powershell"), "powershell");
  assert.equal(languageForFence("nonsense"), null);
  assert.equal(languageForFence(""), null);
});

test("language comes from the extension or well-known file names", () => {
  assert.equal(languageFor("src/App.TSX"), "typescript");
  assert.equal(languageFor("a\\b\\run.ps1"), "powershell");
  assert.equal(languageFor("Dockerfile"), "bash");
  assert.equal(languageFor("notes.unknownext"), null);
  assert.equal(languageFor("LICENSE"), null);
});

test("highlighting marks tokens and escapes markup from the file", () => {
  const html = highlightToHtml("x.ts", 'const a = "<script>alert(1)</script>"; // hi')!;
  assert.match(html, /hljs-keyword/);
  assert.ok(!html.includes("<script>"));
  assert.match(html, /&lt;script&gt;/);
});

test("unknown types and very large files stay plain", () => {
  assert.equal(highlightToHtml("a.txt", "hello"), null);
  assert.equal(highlightToHtml("a.json", "x".repeat(maxHighlightChars + 1)), null);
});
