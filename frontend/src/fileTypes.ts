import type { IconName } from "./icons";

// Maps a file name to an icon and a muted accent colour.
const byExtension: Record<string, [IconName, string]> = {
  md: ["markdown", "var(--file-blue)"], markdown: ["markdown", "var(--file-blue)"],
  ts: ["code", "var(--file-cyan)"], tsx: ["code", "var(--file-cyan)"], js: ["code", "var(--file-yellow)"], jsx: ["code", "var(--file-yellow)"], mjs: ["code", "var(--file-yellow)"],
  go: ["code", "var(--file-teal)"], py: ["code", "var(--file-sky)"], rs: ["code", "var(--file-orange)"], java: ["code", "var(--file-red)"], kt: ["code", "var(--file-purple)"],
  cs: ["code", "var(--file-green)"], c: ["code", "var(--file-sky)"], h: ["code", "var(--file-sky)"], cpp: ["code", "var(--file-sky)"], css: ["code", "var(--file-purple)"], html: ["code", "var(--file-red)"],
  json: ["data", "var(--file-yellow)"], yaml: ["data", "var(--file-red)"], yml: ["data", "var(--file-red)"], toml: ["data", "var(--file-orange)"], xml: ["data", "var(--file-orange)"],
  ps1: ["shell", "var(--file-cyan)"], psm1: ["shell", "var(--file-cyan)"], sh: ["shell", "var(--file-green)"], bat: ["shell", "var(--file-green)"], cmd: ["shell", "var(--file-green)"],
  png: ["image", "var(--file-purple)"], jpg: ["image", "var(--file-purple)"], jpeg: ["image", "var(--file-purple)"], gif: ["image", "var(--file-purple)"], svg: ["image", "var(--file-purple)"], ico: ["image", "var(--file-purple)"], webp: ["image", "var(--file-purple)"],
  zip: ["archive", "var(--file-orange)"], gz: ["archive", "var(--file-orange)"], tgz: ["archive", "var(--file-orange)"], "7z": ["archive", "var(--file-orange)"],
  lock: ["lock", "var(--file-muted)"], env: ["lock", "var(--file-muted)"],
};

export function fileIconFor(name: string): { icon: IconName; color: string } {
  const lower = name.toLowerCase();
  if (lower === ".gitignore" || lower === ".gitattributes" || lower === "go.sum") return { icon: "gear", color: "var(--file-muted)" };
  if (lower === "dockerfile" || lower === "makefile") return { icon: "gear", color: "var(--file-sky)" };
  const dot = lower.lastIndexOf(".");
  const match = dot >= 0 ? byExtension[lower.slice(dot + 1)] : undefined;
  return match ? { icon: match[0], color: match[1] } : { icon: "doc", color: "var(--file-default)" };
}
