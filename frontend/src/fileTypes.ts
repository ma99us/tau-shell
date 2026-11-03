import type { IconName } from "./icons";

// Maps a file name to an icon and a muted accent colour.
const byExtension: Record<string, [IconName, string]> = {
  md: ["markdown", "#7aa2f7"], markdown: ["markdown", "#7aa2f7"],
  ts: ["code", "#4aa3df"], tsx: ["code", "#4aa3df"], js: ["code", "#e5c07b"], jsx: ["code", "#e5c07b"], mjs: ["code", "#e5c07b"],
  go: ["code", "#56b6c2"], py: ["code", "#61afef"], rs: ["code", "#d19a66"], java: ["code", "#e06c75"], kt: ["code", "#c678dd"],
  cs: ["code", "#98c379"], c: ["code", "#61afef"], h: ["code", "#61afef"], cpp: ["code", "#61afef"], css: ["code", "#c678dd"], html: ["code", "#e06c75"],
  json: ["data", "#e5c07b"], yaml: ["data", "#e06c75"], yml: ["data", "#e06c75"], toml: ["data", "#d19a66"], xml: ["data", "#d19a66"],
  ps1: ["shell", "#4aa3df"], psm1: ["shell", "#4aa3df"], sh: ["shell", "#98c379"], bat: ["shell", "#98c379"], cmd: ["shell", "#98c379"],
  png: ["image", "#c678dd"], jpg: ["image", "#c678dd"], jpeg: ["image", "#c678dd"], gif: ["image", "#c678dd"], svg: ["image", "#c678dd"], ico: ["image", "#c678dd"], webp: ["image", "#c678dd"],
  zip: ["archive", "#d19a66"], gz: ["archive", "#d19a66"], tgz: ["archive", "#d19a66"], "7z": ["archive", "#d19a66"],
  lock: ["lock", "#7f848e"], env: ["lock", "#7f848e"],
};

export function fileIconFor(name: string): { icon: IconName; color: string } {
  const lower = name.toLowerCase();
  if (lower === ".gitignore" || lower === ".gitattributes" || lower === "go.sum") return { icon: "gear", color: "#7f848e" };
  if (lower === "dockerfile" || lower === "makefile") return { icon: "gear", color: "#61afef" };
  const dot = lower.lastIndexOf(".");
  const match = dot >= 0 ? byExtension[lower.slice(dot + 1)] : undefined;
  return match ? { icon: match[0], color: match[1] } : { icon: "doc", color: "#9aa0a6" };
}
