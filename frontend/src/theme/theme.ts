// Theme access for JavaScript (xterm options, SVG fills, saved tag colors).
// No color or size literals live here: every value is read from the CSS tokens in
// theme/tokens.css, so the token file stays the single source of truth.
// See docs/v2/25-ui-design-system.md.

const swatchNames = ["Red", "Orange", "Yellow", "Green", "Teal", "Blue", "Purple", "Pink"] as const;

// Resolved at call time; tokens are available once the stylesheet has loaded.
export function token(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(`--${name}`).trim();
}

export const theme = {
  fontMono: () => token("font-terminal"),
  terminal: () => ({
    background: token("terminal-bg"),
    foreground: token("terminal-fg"),
    cursor: token("terminal-cursor"),
    fontSize: Number.parseInt(token("terminal-font-size"), 10),
  }),
  // Brand marks keep the vendor colors; used as CSS values (SVG fill style).
  harnessMark: {
    "claude-code": "var(--brand-claude)",
    "copilot-cli": "var(--brand-copilot)",
    codex: "var(--brand-codex)",
    opencode: "var(--brand-opencode)",
    gsd: "var(--brand-gsd)",
  } as const,
  fallbackSessionColor: () => token("tag-fallback"),
  sessionTextOn: (light: boolean) => token(light ? "tag-text-on-light" : "tag-text-on-dark"),
  // Session tag colors offered in the context menu. The hex is what gets saved in the session YAML.
  sessionSwatches: () => swatchNames.map((name) => ({ name, value: token(`tag-${name.toLowerCase()}`) })),
};
