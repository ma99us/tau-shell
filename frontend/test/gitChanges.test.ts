import assert from "node:assert/strict";
import test from "node:test";
import { buildChangeTree, changeLabel, countChanges } from "../src/gitChanges.ts";

test("builds a folders-first tree of changed files", () => {
  const tree = buildChangeTree([
    { path: "z.txt", code: "M", staged: false },
    { path: "src/b.go", code: "A", staged: true },
    { path: "src/a/deep.go", code: "U", staged: false },
    { path: "src/a.go", code: "D", staged: false },
  ]);
  assert.deepEqual(tree.map((node) => node.name), ["src", "z.txt"]);
  const src = tree[0];
  assert.deepEqual(src.children.map((node) => `${node.name}${node.dir ? "/" : ""}`), ["a/", "a.go", "b.go"]);
  assert.equal(src.children[0].children[0].path, "src/a/deep.go");
  assert.equal(countChanges(tree), 4);
});

test("labels change codes and staging", () => {
  assert.equal(changeLabel({ code: "M", staged: false }), "Modified");
  assert.equal(changeLabel({ code: "A", staged: true }), "Added (staged)");
  assert.equal(changeLabel({ code: "!" }), "Conflict");
});

test("an empty change list gives an empty tree", () => {
  assert.deepEqual(buildChangeTree([]), []);
});
