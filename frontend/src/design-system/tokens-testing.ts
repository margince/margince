// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// The sheet every token gate reads, and the two ways of reading it.
//
// It lives here rather than in one of them because there are two now —
// tokens.test.ts pins the Ledger-Green palette, type-tokens.test.ts pins the
// type scale and the control geometry — and a second copy of `parseBlock` is a
// second answer to what a declaration IS. The narrower copy would read fewer
// tokens and report the same word, PASS.

const here = dirname(fileURLToPath(import.meta.url));

/** tokens.css as authored, for anything that reports a line a reader can open. */
export const tokensCss = readFileSync(join(here, "tokens.css"), "utf8");

/**
 * The same sheet with its comments removed, for anything that reads
 * DECLARATIONS. A declaration and a sentence about a declaration are not the
 * same thing, and that file explains most of its values in prose that names
 * them; left in, the prose parses as declarations of its own.
 */
export const tokenDecls = tokensCss.replace(/\/\*[\s\S]*?\*\//g, "");

/**
 * A value with its formatting taken out — case, whitespace and a leading zero
 * before a decimal point — so a comparison is about the value and not about how
 * the formatter last wrapped it.
 */
export function normalize(value: string): string {
  return value
    .toLowerCase()
    .replace(/\s+/g, "")
    .replace(/(^|[^0-9])0\./g, "$1.")
    .replace(/(\.[0-9]*?)0+([^0-9]|$)/g, "$1$2");
}

/** Every custom property one block declares, by name. */
export function parseBlock(
  css: string,
  selector: string,
): Record<string, string> {
  // Parentheses and the colon are escaped too, so a selector carrying a
  // functional pseudo-class (`:root:not([data-theme="light"])`) is matched as
  // the literal text it is rather than compiled into a capture group — which
  // matches nothing, and would report the block as missing.
  const match = css.match(
    new RegExp(`${selector.replace(/[[\]"=():]/g, "\\$&")}\\s*\\{([^}]*)\\}`),
  );
  if (!match) {
    throw new Error(`tokens.css has no ${selector} block`);
  }
  const props: Record<string, string> = {};
  for (const [, name, value] of match[1].matchAll(
    /(--[\w-]+)\s*:\s*([^;]+);/g,
  )) {
    props[name] = value.trim();
  }
  return props;
}

// ===== lch() → sRGB =====
//
// `--textSecondary` is authored in LCh, and every contrast sweep in
// tokens.test.ts measures sRGB. A colour the sweeps cannot parse is not a
// failure, it is a THROW or a silent skip, and either way the one neutral the
// product reads most would be the one nobody measured.
//
// The whole chain, CSS Color 4 §-by-§, because there is no colour dependency in
// package.json and adding one to convert four numbers is a package to keep
// current forever. Each step is the spec's own matrix, unrounded: LCh is polar
// Lab; CSS Lab is D50-referred; sRGB is D65-referred, so the chromatic
// adaptation between them is not optional — skipping it lands a neutral grey a
// visible step off where it was authored, in the direction the eye reads as a
// colour cast.

/** D50 white, from the spec's chromaticity. */
const D50: readonly number[] = [
  0.3457 / 0.3585,
  1,
  (1 - 0.3457 - 0.3585) / 0.3585,
];

/** Bradford-adapted D50 → D65 (CSS Color 4). */
const D50_TO_D65: readonly number[][] = [
  [0.9554734527042182, -0.023098536874261423, 0.0632593086610217],
  [-0.028369706963208136, 1.0099954580058226, 0.021041398966943008],
  [0.012314001688319899, -0.020507696433477912, 1.3303659366080753],
];

/** XYZ (D65) → linear-light sRGB (CSS Color 4). */
const XYZ_TO_RGB: readonly number[][] = [
  [3.2409699419045226, -1.537383177570094, -0.4986107602930034],
  [-0.9692436362808796, 1.8759675015077202, 0.04155505740717559],
  [0.05563007969699366, -0.20397695888897652, 1.0569715142428786],
];

const apply = (m: readonly number[][], v: readonly number[]) =>
  m.map((row) => row[0] * v[0] + row[1] * v[1] + row[2] * v[2]);

/** The sRGB transfer function, on one linear-light channel. */
function gamma(channel: number): number {
  const sign = channel < 0 ? -1 : 1;
  const c = Math.abs(channel);
  return c <= 0.0031308
    ? 12.92 * channel
    : sign * (1.055 * c ** (1 / 2.4) - 0.055);
}

/**
 * An `lch(L% C H)` value as a `#rrggbb`.
 *
 * Out-of-gamut components are CLAMPED rather than gamut-mapped. Every LCh
 * colour in tokens.css is a near-neutral at C ≈ 1, nowhere near an edge, so the
 * two agree here — and a clamp that silently rescued a saturated colour would
 * hide the fact that the sheet asked for one sRGB cannot show. `parseLch`
 * throws on anything it does not recognise for the same reason: a value this
 * returns a default for is a value no sweep has actually measured.
 */
export function lchToHex(value: string): string {
  const parts = /^lch\(\s*([\d.]+)%\s+([\d.]+)\s+([\d.]+)\s*\)$/.exec(
    value.trim(),
  );
  if (!parts) {
    throw new Error(`${value} is not an lch(L% C H) value`);
  }
  const [lightness, chroma, hue] = parts.slice(1).map(Number);
  const radians = (hue * Math.PI) / 180;
  const [a, b] = [chroma * Math.cos(radians), chroma * Math.sin(radians)];

  // Lab → XYZ (D50), CIE 15.3 with the spec's integer κ and ε.
  const kappa = 24389 / 27;
  const epsilon = 216 / 24389;
  const fy = (lightness + 16) / 116;
  const [fx, fz] = [fy + a / 500, fy - b / 200];
  const ratios = [
    fx ** 3 > epsilon ? fx ** 3 : (116 * fx - 16) / kappa,
    lightness > kappa * epsilon ? fy ** 3 : lightness / kappa,
    fz ** 3 > epsilon ? fz ** 3 : (116 * fz - 16) / kappa,
  ];
  const xyzD50 = ratios.map((ratio, i) => ratio * D50[i]);

  const rgb = apply(XYZ_TO_RGB, apply(D50_TO_D65, xyzD50))
    .map(gamma)
    .map((channel) => Math.round(Math.min(1, Math.max(0, channel)) * 255));
  return `#${rgb.map((n) => n.toString(16).padStart(2, "0")).join("")}`;
}
