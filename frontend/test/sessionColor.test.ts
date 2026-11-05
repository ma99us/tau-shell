import assert from "node:assert/strict";
import test from "node:test";
import { contrast, inkOn, parseHex, sessionPalette, tintShares, warnOn } from "../src/sessionColor.ts";

const panel = "#11141a", light = "#ffffff", dark = "#101318";

test("every swatch-like color gets readable ink on every tinted surface", () => {
  for (const color of ["#ff0000", "#ffff00", "#ffffff", "#000000", "#00ffff", "#7a3cff", "#ff9500", "#33cc66"]) {
    const palette = sessionPalette(color, panel, light, dark)!;
    const ground = parseHex(panel)!, base = parseHex(color)!;
    for (const [share, ink] of [[tintShares.rest, palette.ink], [tintShares.hover, palette.inkHover], [tintShares.active, palette.inkActive], [tintShares.bar, palette.barInk]] as const) {
      const surface = base.map((v, i) => v * share + ground[i] * (1 - share)) as [number, number, number];
      assert.ok(contrast(surface, parseHex(ink)!) >= 4.5, `${color} at ${share}: ${ink}`);
    }
  }
});

test("ink flips to dark on light backgrounds", () => {
  assert.equal(inkOn([250, 250, 200], light, dark), dark);
  assert.equal(inkOn([20, 20, 40], light, dark), light);
});

test("an invalid color gives no palette", () => {
  assert.equal(sessionPalette("red", panel, light, dark), null);
});

test("the warning color stays readable on every tinted top bar", () => {
  const warn = "#e0b24d";
  for (const color of ["#ff0000", "#ffff00", "#ffffff", "#000000", "#00ffff", "#7a3cff", "#ff9500", "#33cc66", "#e0b24d"]) {
    const palette = sessionPalette(color, panel, light, dark, warn)!;
    const ground = parseHex(panel)!, base = parseHex(color)!;
    const bar = base.map((v, i) => v * tintShares.bar + ground[i] * (1 - tintShares.bar)) as [number, number, number];
    assert.ok(contrast(bar, parseHex(palette.barWarn)!) >= 4.5, `${color}: ${palette.barWarn}`);
  }
});

test("the warning color is left alone when it already reads well", () => {
  assert.equal(warnOn([17, 20, 26], "#e0b24d", "#ffffff"), "#e0b24d");
});
