import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { contrast, parseHex } from "../src/sessionColor.ts";

function tokens(file: string): Map<string, string> {
  const text = readFileSync(new URL(`../src/theme/${file}`, import.meta.url), "utf8");
  return new Map([...text.matchAll(/^\s*(--[\w-]+):\s*([^;]+);/gm)].map((match) => [match[1], match[2].trim()]));
}

const rgb = (map: Map<string, string>, name: string) => {
  let value = map.get(name);
  while (value?.startsWith("var(")) value = map.get(value.slice(4, -1));
  value = value?.replace(/^#([0-9a-f])([0-9a-f])([0-9a-f])$/i, "#$1$1$2$2$3$3");
  const color = parseHex(value?.slice(0, 7) ?? "");
  assert.ok(color, `${name} must be an opaque #rrggbb color (got ${value})`);
  return color;
};

function check(map: Map<string, string>, pairs: [string, string, number][]) {
  const failures = pairs.flatMap(([fg, bg, min]) => {
    const ratio = contrast(rgb(map, fg), rgb(map, bg));
    return ratio >= min ? [] : [`${fg} on ${bg}: ${ratio.toFixed(2)} < ${min}`];
  });
  assert.deepEqual(failures, []);
}

const grounds = ["--bg-app", "--bg-panel", "--bg-card", "--bg-raised", "--bg-hover", "--bg-control"];

test("light theme text meets WCAG AA on every surface", () => {
  const light = tokens("light.css");
  const texts = ["--text-1", "--text-title", ...Array.from({ length: 10 }, (_, index) => `--text-${index + 2}`), "--text-11"];
  check(light, texts.flatMap((text) => grounds.map((ground): [string, string, number] => [text, ground, 4.5])));
});

test("light theme accents, danger and terminal colors are readable", () => {
  const light = tokens("light.css");
  const pairs: [string, string, number][] = [
    ["--accent", "--bg-panel", 4.5],
    ["--accent-eyebrow", "--bg-panel", 4.5],
    ["--on-accent", "--accent", 4.5],
    ["--on-action", "--action", 4.5],
    ["--white", "--danger-solid", 4.5],
    ["--terminal-fg", "--terminal-bg", 7],
    ["--terminal-cursor", "--terminal-bg", 3],
    ["--status-ok", "--bg-panel", 3],
    ["--status-warn", "--bg-panel", 3],
    ["--status-input", "--bg-panel", 3],
    ["--status-failed", "--bg-panel", 3],
    ["--border-muted", "--bg-panel", 2],
  ];
  for (const index of [1, 2, 3, 4]) {
    pairs.push([`--accent-text-${index}`, `--accent-bg-${index}`, 4.5], [`--danger-text-${index}`, `--danger-bg-${index}`, 4.5]);
  }
  for (const name of ["black", "red", "green", "yellow", "blue", "magenta", "cyan", "white", "bright-black", "bright-red", "bright-green", "bright-yellow", "bright-blue", "bright-magenta", "bright-cyan", "bright-white"]) {
    pairs.push([`--ansi-${name}`, "--terminal-bg", 3]);
  }
  check(light, pairs);
});

test("dark theme keeps its text and terminal readable", () => {
  const dark = tokens("dark.css");
  check(dark, [
    ["--text-1", "--bg-panel", 7],
    ["--text-6", "--bg-panel", 4.5],
    ["--terminal-fg", "--terminal-bg", 7],
  ]);
});

test("dark and light define the same tokens", () => {
  assert.deepEqual([...tokens("light.css").keys()].sort(), [...tokens("dark.css").keys()].sort());
});
