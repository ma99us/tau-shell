// Pure color math for session tags: tinted surfaces and the text color that stays
// readable on them, whatever color the user picks.

type Rgb = [number, number, number];

export function parseHex(hex: string): Rgb | null {
  const match = hex.trim().match(/^#([0-9a-f]{6})$/i);
  if (!match) return null;
  return [0, 2, 4].map((i) => parseInt(match[1].slice(i, i + 2), 16)) as Rgb;
}

export function toHex(rgb: Rgb): string {
  return `#${rgb.map((value) => Math.round(value).toString(16).padStart(2, "0")).join("")}`;
}

export function luminance(rgb: Rgb): number {
  const [r, g, b] = rgb.map((value) => value / 255).map((c) => c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrast(a: Rgb, b: Rgb): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// Same result as CSS color-mix(in srgb, color share%, base).
export function mix(color: Rgb, base: Rgb, share: number): Rgb {
  return color.map((value, index) => value * share + base[index] * (1 - share)) as Rgb;
}

// Whichever of the two ink colors has more contrast on the background; pure
// black or white is used when neither reaches WCAG AA (4.5), which one always can.
export function inkOn(background: Rgb, light: string, dark: string): string {
  const lightRgb = parseHex(light), darkRgb = parseHex(dark);
  if (!lightRgb || !darkRgb) return light;
  const [best, bestRgb] = contrast(background, lightRgb) >= contrast(background, darkRgb) ? [light, lightRgb] : [dark, darkRgb];
  if (contrast(background, bestRgb) >= 4.5) return best;
  return contrast(background, [255, 255, 255]) >= contrast(background, [0, 0, 0]) ? "#ffffff" : "#000000";
}

// The tint shares are published to CSS as --session-tint-* so the tab/row
// backgrounds and the contrast math above cannot disagree.
export const tintShares = { rest: 0.3, hover: 0.42, active: 0.52, bar: 0.5 } as const;

export const tintVars = {
  "--session-tint-rest": `${tintShares.rest * 100}%`,
  "--session-tint-hover": `${tintShares.hover * 100}%`,
  "--session-tint-active": `${tintShares.active * 100}%`,
} as const;

// The warning color adjusted until it reaches WCAG AA (4.5) on the background:
// kept as is when it already does, otherwise moved toward white or black (whichever
// reads better on that background), falling back to plain ink if nothing reaches it.
export function warnOn(background: Rgb, warn: string, fallbackInk: string): string {
  const base = parseHex(warn);
  if (!base) return fallbackInk;
  if (contrast(background, base) >= 4.5) return warn;
  const target: Rgb = contrast(background, [255, 255, 255]) >= contrast(background, [0, 0, 0]) ? [255, 255, 255] : [0, 0, 0];
  for (let share = 0.1; share <= 1.0001; share += 0.1) {
    const candidate = mix(target, base, share);
    if (contrast(background, candidate) >= 4.5) return toHex(candidate);
  }
  return fallbackInk;
}

export type SessionPalette = { color: string; ink: string; inkHover: string; inkActive: string; bar: string; barInk: string; barWarn: string };

export function sessionPalette(color: string, panel: string, light: string, dark: string, warn = ""): SessionPalette | null {
  const base = parseHex(color), ground = parseHex(panel);
  if (!base || !ground) return null;
  const on = (share: number) => inkOn(mix(base, ground, share), light, dark);
  const barInk = on(tintShares.bar);
  return {
    color,
    ink: on(tintShares.rest),
    inkHover: on(tintShares.hover),
    inkActive: on(tintShares.active),
    bar: toHex(mix(base, ground, tintShares.bar)),
    barInk,
    barWarn: warnOn(mix(base, ground, tintShares.bar), warn, barInk),
  };
}
