import hljs from "highlight.js/lib/core";
import bash from "highlight.js/lib/languages/bash";
import cpp from "highlight.js/lib/languages/cpp";
import csharp from "highlight.js/lib/languages/csharp";
import css from "highlight.js/lib/languages/css";
import diff from "highlight.js/lib/languages/diff";
import go from "highlight.js/lib/languages/go";
import ini from "highlight.js/lib/languages/ini";
import java from "highlight.js/lib/languages/java";
import javascript from "highlight.js/lib/languages/javascript";
import json from "highlight.js/lib/languages/json";
import kotlin from "highlight.js/lib/languages/kotlin";
import markdown from "highlight.js/lib/languages/markdown";
import powershell from "highlight.js/lib/languages/powershell";
import python from "highlight.js/lib/languages/python";
import rust from "highlight.js/lib/languages/rust";
import sql from "highlight.js/lib/languages/sql";
import typescript from "highlight.js/lib/languages/typescript";
import xml from "highlight.js/lib/languages/xml";
import yaml from "highlight.js/lib/languages/yaml";

const languages = { bash, cpp, csharp, css, diff, go, ini, java, javascript, json, kotlin, markdown, powershell, python, rust, sql, typescript, xml, yaml };
for (const [name, definition] of Object.entries(languages)) hljs.registerLanguage(name, definition);

const byExtension: Record<string, string> = {
  js: "javascript", jsx: "javascript", mjs: "javascript", cjs: "javascript",
  ts: "typescript", tsx: "typescript", json: "json", jsonc: "json",
  yaml: "yaml", yml: "yaml", toml: "ini", ini: "ini", cfg: "ini", conf: "ini", properties: "ini",
  go: "go", py: "python", rs: "rust", cs: "csharp", java: "java", kt: "kotlin", kts: "kotlin",
  c: "cpp", h: "cpp", cpp: "cpp", cc: "cpp", hpp: "cpp",
  ps1: "powershell", psm1: "powershell", psd1: "powershell", sh: "bash", bash: "bash", zsh: "bash",
  sql: "sql", css: "css", html: "xml", htm: "xml", xml: "xml", csproj: "xml", svg: "xml", xaml: "xml",
  md: "markdown", markdown: "markdown", diff: "diff", patch: "diff",
};

// Past this size highlighting is skipped so a large file cannot freeze the UI.
export const maxHighlightChars = 200_000;

export function languageFor(path: string): string | null {
  const name = path.split(/[\\/]/).pop()?.toLowerCase() ?? "";
  if (name === "dockerfile") return "bash";
  const dot = name.lastIndexOf(".");
  return dot < 0 ? null : byExtension[name.slice(dot + 1)] ?? null;
}

// Maps a Markdown fence label such as "ts", "powershell" or "json" to a registered language.
export function languageForFence(info: string): string | null {
  const label = info.trim().split(/\s+/)[0]?.toLowerCase() ?? "";
  if (!label) return null;
  return byExtension[label] ?? (hljs.getLanguage(label) ? label : null);
}

// Returns highlight.js markup (it escapes the source text) or null when the
// text should be shown as plain.
export function highlightCode(language: string | null, text: string): string | null {
  if (!language || text.length > maxHighlightChars) return null;
  try {
    return hljs.highlight(text, { language, ignoreIllegals: true }).value;
  } catch {
    return null;
  }
}

export const highlightToHtml = (path: string, text: string): string | null => highlightCode(languageFor(path), text);
