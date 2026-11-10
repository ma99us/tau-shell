// Theme access for JavaScript (xterm options, SVG fills, saved tag colors).
// No color or size literals live here: every value is read from the CSS tokens in
// theme/tokens.css, so the token file stays the single source of truth.
// See docs/v2/25-ui-design-system.md.

const swatchNames = ["Red", "Orange", "Green", "Teal", "Blue", "Purple", "Pink"] as const;

// Resolved at call time; tokens are available once the stylesheet has loaded.
export function token(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(`--${name}`).trim();
}

export const theme = {
  fontMono: () => token("font-terminal"),
  // The xterm theme (background, foreground, selection and the 16 ANSI colors) plus size and contrast.
  terminal: () => ({
    theme: {
      background: token("terminal-bg"),
      foreground: token("terminal-fg"),
      cursor: token("terminal-cursor"),
      selectionBackground: token("terminal-selection"),
      black: token("ansi-black"),
      red: token("ansi-red"),
      green: token("ansi-green"),
      yellow: token("ansi-yellow"),
      blue: token("ansi-blue"),
      magenta: token("ansi-magenta"),
      cyan: token("ansi-cyan"),
      white: token("ansi-white"),
      brightBlack: token("ansi-bright-black"),
      brightRed: token("ansi-bright-red"),
      brightGreen: token("ansi-bright-green"),
      brightYellow: token("ansi-bright-yellow"),
      brightBlue: token("ansi-bright-blue"),
      brightMagenta: token("ansi-bright-magenta"),
      brightCyan: token("ansi-bright-cyan"),
      brightWhite: token("ansi-bright-white"),
    },
    fontSize: Number.parseInt(token("terminal-font-size"), 10),
    minimumContrastRatio: Number.parseFloat(token("terminal-min-contrast")) || 1,
  }),
  // Brand marks keep the vendor colors; used as CSS values (SVG fill style).
  // User harnesses use the color in their manifest instead.
  harnessMark: {
    "claude-code": "var(--brand-claude)",
    "copilot-cli": "var(--brand-copilot)",
    codex: "var(--brand-codex)",
    opencode: "var(--brand-opencode)",
    gsd: "var(--brand-gsd)",
  } as const,
  fallbackSessionColor: () => token("tag-fallback"),
  panelBackground: () => token("bg-panel"),
  warningColor: () => token("status-warn"),
  sessionTextOn: (light: boolean) => token(light ? "tag-text-on-light" : "tag-text-on-dark"),
  // Session tag colors offered in the context menu. The hex is what gets saved in the session YAML.
  sessionSwatches: () => swatchNames.map((name) => ({ name, value: token(`tag-${name.toLowerCase()}`) })),
};
