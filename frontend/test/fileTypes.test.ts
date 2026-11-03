import assert from "node:assert/strict";
import test from "node:test";
import { fileIconFor } from "../src/fileTypes.ts";

test("file icons follow the extension, with a generic fallback", () => {
  assert.equal(fileIconFor("README.md").icon, "markdown");
  assert.equal(fileIconFor("Main.GO").icon, "code");
  assert.equal(fileIconFor("config.yaml").icon, "data");
  assert.equal(fileIconFor("logo.PNG").icon, "image");
  assert.equal(fileIconFor("notes.xyz").icon, "doc");
  assert.equal(fileIconFor("Makefile").icon, "gear");
});
