import { test } from "node:test";
import assert from "node:assert/strict";
import { parseInline, parseMarkdown } from "../src/markdown.ts";

test("markdown becomes plain nodes: no HTML is passed through", () => {
  const blocks = parseMarkdown("# Title\n\nSome **bold** and `code` text <script>alert(1)</script>\n\n- one\n- two\n\n```\nlet x = 1;\n```\n---\n> quoted");
  assert.deepEqual(blocks.map((block) => block.kind), ["heading", "paragraph", "list", "code", "rule", "quote"]);
  const paragraph = blocks[1];
  assert.ok(paragraph.kind === "paragraph");
  assert.ok(paragraph.inline.some((node) => node.kind === "text" && node.text.includes("<script>")), "script stays literal text");
  const list = blocks[2];
  assert.ok(list.kind === "list" && list.items.length === 2 && !list.ordered);
  assert.deepEqual(blocks[3], { kind: "code", text: "let x = 1;" });
});

test("links stay as text with their target and images are never loaded", () => {
  assert.deepEqual(parseInline("[docs](https://example.com/a)"), [{ kind: "link", label: "docs", href: "https://example.com/a" }]);
  assert.deepEqual(parseInline("![logo](http://evil/x.png)"), [{ kind: "text", text: "[image: logo]" }]);
  assert.deepEqual(parseInline("javascript:alert(1)"), [{ kind: "text", text: "javascript:alert(1)" }]);
});

test("an unterminated code fence takes the rest of the file instead of hanging", () => {
  assert.deepEqual(parseMarkdown("```\na\nb"), [{ kind: "code", text: "a\nb" }]);
});
