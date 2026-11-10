// Pure appearance settings logic (no DOM): choices, limits, parsing and the theme
// resolution. The DOM side lives in appearance.ts.

export type ThemeChoice = "dark" | "light" | "system";
export type ResolvedTheme = "dark" | "light";
export type Appearance = { theme: ThemeChoice; fontScale: number; terminalFontSize: number };

export const themeChoices: { id: ThemeChoice; label: string }[] = [
  { id: "dark", label: "Dark" },
  { id: "light", label: "Light" },
  { id: "system", label: "System" },
];

export const fontScale = { min: 0.8, max: 1.6, step: 0.1, default: 1 } as const;
export const terminalFont = { min: 9, max: 28, step: 1, default: 14 } as const;
export const defaultAppearance: Appearance = { theme: "dark", fontScale: fontScale.default, terminalFontSize: terminalFont.default };
export const appearanceKey = "taushell.appearance.v1";

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

export function clampFontScale(value: number): number {
  return Math.round(clamp(value, fontScale.min, fontScale.max) * 100) / 100;
}

export function clampTerminalFont(value: number): number {
  return Math.round(clamp(value, terminalFont.min, terminalFont.max));
}

// Accepts anything (saved JSON, bridge data) and returns safe values; unknown or out-of-range fields use the defaults.
export function parseAppearance(raw: unknown): Appearance {
  const value = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
  const theme = value.theme === "light" || value.theme === "system" || value.theme === "dark" ? value.theme : defaultAppearance.theme;
  const scale = typeof value.fontScale === "number" && Number.isFinite(value.fontScale) ? clampFontScale(value.fontScale) : defaultAppearance.fontScale;
  const terminal = typeof value.terminalFontSize === "number" && Number.isFinite(value.terminalFontSize) ? clampTerminalFont(value.terminalFontSize) : defaultAppearance.terminalFontSize;
  return { theme, fontScale: scale, terminalFontSize: terminal };
}

export function resolveTheme(choice: ThemeChoice, systemPrefersDark: boolean): ResolvedTheme {
  if (choice === "system") return systemPrefersDark ? "dark" : "light";
  return choice;
}

export function readCachedAppearance(storage: Pick<Storage, "getItem">): Appearance {
  try {
    return parseAppearance(JSON.parse(storage.getItem(appearanceKey) ?? "null"));
  } catch {
    return defaultAppearance;
  }
}

export const percent = (scale: number) => `${Math.round(scale * 100)}%`;
