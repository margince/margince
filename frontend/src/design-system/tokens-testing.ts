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
    .replace(/\.[0-9]+/g, withoutTrailingZeros);
}

// The zeros come off one at a time rather than through `(\.[0-9]*?)0+`, where
// the lazy run and the greedy one compete for the same digits — which is what
// made that pattern cost the square of the decimal's length. The decimal point
// stays: `1.0` normalizes to `1.`, and it is the two sides agreeing that
// matters here, not which of them is prettier.
function withoutTrailingZeros(decimals: string): string {
  let end = decimals.length;
  while (end > 1 && decimals.charAt(end - 1) === "0") {
    end -= 1;
  }
  return decimals.slice(0, end);
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
  // Split, rather than scanned with `/(--[\w-]+)\s*:([^;]+);/g`. That pattern
  // was flagged for super-linear runtime and the width of its value class is
  // why: `[^;]+` can run to the end of the block from every start position the
  // engine tries, so the cost grows with the square of the block. The grammar
  // is the same one stated without backtracking — a declaration ends at `;`
  // and divides at its FIRST `:`, so a value may hold a colon (`url(https://…)`)
  // and still arrive whole.
  //
  // One deliberate difference from the pattern: a LAST declaration with no `;`
  // is read rather than dropped, which is what CSS means by it. The sheet ends
  // every declaration today — both spellings parse its `:root` and dark blocks
  // to the same 173 and 63 properties — so this only decides what happens to a
  // token somebody adds without the semicolon, and dropping it silently was
  // the worse of the two answers.
  for (const declaration of match[1].split(";")) {
    const divide = declaration.indexOf(":");
    if (divide < 0) {
      continue; // The block's trailing whitespace, and anything not a declaration.
    }
    const name = declaration.slice(0, divide).trim();
    if (!name.startsWith("--")) {
      continue; // A property this sheet does not own is not a token.
    }
    props[name] = declaration.slice(divide + 1).trim();
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

// ONE PALETTE OF COLOUR MATHS, FOR EVERY GATE THAT MEASURES THE SHEET.
//
// These lived inside tokens.test.ts, where they were reachable by exactly the
// assertions in that file. The state family's own gate is a sibling file now
// (state-tokens.test.ts) and it measures the same tints against the same
// grounds, so a second copy of the sRGB/Oklab conversions would be two answers
// to one question — and the copy that drifts is always the one whose failures
// nobody has seen. They are here, exported once, and each test file states only
// what it asserts.

// The :root palette, which both themes are read as a delta from.
const light = parseBlock(tokenDecls, ":root");

// The states, read off the sheet rather than listed beside it: a state is a
// base that carries an opaque Surface, and every sweep over the family iterates
// THIS. A list maintained by hand is the failure mode these gates exist to
// avoid — it goes on reporting PASS over a family it stopped covering.
export const states = Object.keys(light)
  .map((name) => /^--([a-z]+)Surface$/.exec(name)?.[1])
  .filter((name): name is string => name !== undefined);

// Relative luminance, WCAG 2.x §relativeluminancedef. Hex only, which is
// what every rung on this ladder is — an alpha colour has no luminance of
// its own, and none of these is one.
export function luminance(hex: string): number {
  const h = hex.trim().replace("#", "");
  const full =
    h.length === 3
      ? h
          .split("")
          .map((d) => d + d)
          .join("")
      : h;
  if (full.length !== 6) {
    throw new Error(`${hex} is not a 3- or 6-digit hex`);
  }
  const [r, g, b] = [0, 2, 4].map((i) => {
    const c = Number.parseInt(full.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

// A token's channels, whether it is written as a hex or as an rgba(). The
// alpha comes back too, because a tint's alpha is the whole reason the
// composite tests below exist.
function channels(value: string): [number, number, number, number] {
  const v = value.trim();
  // An authored lch() is converted rather than skipped: a colour a sweep
  // cannot read is a colour no sweep has measured, and a neutral the whole
  // product reads is the worst one to leave out.
  if (v.startsWith("lch(")) {
    return channels(lchToHex(v));
  }
  if (v.startsWith("#")) {
    const h = v.slice(1);
    const full =
      h.length === 3
        ? h
            .split("")
            .map((d) => d + d)
            .join("")
        : h.slice(0, 6);
    const [r, g, b] = [0, 2, 4].map((i) =>
      Number.parseInt(full.slice(i, i + 2), 16),
    );
    return [r, g, b, 1];
  }
  const inside = v.match(/rgba?\(([^)]+)\)/);
  if (!inside) {
    throw new Error(`${value} is neither a hex nor an rgb()`);
  }
  const parts = inside[1]
    .replace(/\//g, ",")
    .split(",")
    .map((n) => Number.parseFloat(n.trim()));
  return [parts[0], parts[1], parts[2], parts[3] ?? 1];
}

// Source-over, the compositing every alpha tint in this file goes through
// when a browser paints it on a ground.
export function composite(fg: string, bg: string): string {
  const [r, g, b, a] = channels(fg);
  const [br, bg_, bb] = channels(bg);
  const mix = [
    r * a + br * (1 - a),
    g * a + bg_ * (1 - a),
    b * a + bb * (1 - a),
  ].map((n) => Math.round(n));
  return `rgb(${mix.join(", ")})`;
}

export function contrastOf(fg: string, bg: string): number {
  // Text is opaque in every role measured here; a ground never is not.
  const lf = luminanceOf(fg);
  const lb = luminanceOf(bg);
  const [hi, lo] = lf > lb ? [lf, lb] : [lb, lf];
  return (hi + 0.05) / (lo + 0.05);
}

export function luminanceOf(value: string): number {
  const [lr, lg, lb] = linearOf(value);
  return 0.2126 * lr + 0.7152 * lg + 0.0722 * lb;
}

// Every value the sweeps below measure, as a hex or an rgba(). Most of the
// palette already is one; the status family is not — --success is a
// color-mix() of a lab() hue and the theme's own ink, and --successBg is a
// mix of that. A sweep that cannot read those forms stops measuring the
// family it was written for and still reports PASS, which is the one way a
// gate must not break. So the declared value is resolved here, following
// exactly the forms tokens.css writes and THROWING on anything else.
function topLevelArgs(inside: string): string[] {
  const args: string[] = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < inside.length; i += 1) {
    if (inside[i] === "(") depth += 1;
    if (inside[i] === ")") depth -= 1;
    if (inside[i] === "," && depth === 0) {
      args.push(inside.slice(start, i).trim());
      start = i + 1;
    }
  }
  args.push(inside.slice(start).trim());
  return args;
}

// CSS lab() is CIELAB on the D50 white point (CSS Color 4 §7), so sRGB is
// three conversions away and none is optional: Lab to XYZ under D50, the
// Bradford adaptation to D65, then D65 XYZ to linear sRGB. Skipping the
// adaptation moves every hue here by more than the headroom being
// measured.
function labToLinear(L: number, a: number, b: number): number[] {
  const fy = (L + 16) / 116;
  const fx = fy + a / 500;
  const fz = fy - b / 200;
  const epsilon = 216 / 24389;
  const kappa = 24389 / 27;
  const x = fx ** 3 > epsilon ? fx ** 3 : (116 * fx - 16) / kappa;
  const y = L > kappa * epsilon ? fy ** 3 : L / kappa;
  const z = fz ** 3 > epsilon ? fz ** 3 : (116 * fz - 16) / kappa;
  const d50 = [0.9642956764295677, 1, 0.8251046025104602];
  const [xd, yd, zd] = [x * d50[0], y * d50[1], z * d50[2]];
  const toD65 = [
    [0.955473452704218, -0.02309853687426142, 0.0632593086610217],
    [-0.02836970696320814, 1.0099954580058226, 0.021041398966943],
    [0.0123140016883199, -0.02050769643347791, 1.3303659366080753],
  ];
  const toLinear = [
    [3.2409699419045226, -1.537383177570094, -0.4986107602930034],
    [-0.9692436362808796, 1.8759675015077202, 0.04155505740717559],
    [0.05563007969699366, -0.20397695888897652, 1.0569715142428786],
  ];
  const xyz = toD65.map((row) => row[0] * xd + row[1] * yd + row[2] * zd);
  return toLinear.map((row) =>
    row.reduce((sum, coef, i) => sum + coef * xyz[i], 0),
  );
}

// Oklab, the space tokens.css interpolates the status family in (CSS Color
// 4 §10). A mix there is a straight interpolation of these three
// coordinates, which is why the tokens are written in it: one ink share
// means the same thing to the eye at every hue in the family.
export function linearToOklab(rgb: number[]): number[] {
  const l =
    0.4122214708 * rgb[0] + 0.5363325363 * rgb[1] + 0.0514459929 * rgb[2];
  const m =
    0.2119034982 * rgb[0] + 0.6806995451 * rgb[1] + 0.1073969566 * rgb[2];
  const s =
    0.0883024619 * rgb[0] + 0.2817188376 * rgb[1] + 0.6299787005 * rgb[2];
  const [l_, m_, s_] = [l, m, s].map((v) => Math.cbrt(v));
  return [
    0.2104542553 * l_ + 0.793617785 * m_ - 0.0040720468 * s_,
    1.9779984951 * l_ - 2.428592205 * m_ + 0.4505937099 * s_,
    0.0259040371 * l_ + 0.7827717662 * m_ - 0.808675766 * s_,
  ];
}

function oklabToLinear(lab: number[]): number[] {
  const l_ = lab[0] + 0.3963377774 * lab[1] + 0.2158037573 * lab[2];
  const m_ = lab[0] - 0.1055613458 * lab[1] - 0.0638541728 * lab[2];
  const s_ = lab[0] - 0.0894841775 * lab[1] - 1.291485548 * lab[2];
  const [l, m, s] = [l_ ** 3, m_ ** 3, s_ ** 3];
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
}

// Linear-light channels to the hex a browser would paint, gamut-clipped per
// channel the way a browser clips an out-of-gamut lab().
function hexOf(linear: number[]): string {
  const bytes = linear.map((channel) => {
    const clipped = Math.min(Math.max(channel, 0), 1);
    const encoded =
      clipped <= 0.0031308
        ? clipped * 12.92
        : 1.055 * clipped ** (1 / 2.4) - 0.055;
    return Math.round(encoded * 255)
      .toString(16)
      .padStart(2, "0");
  });
  return `#${bytes.join("")}`;
}

export function linearOf(value: string): number[] {
  const [r, g, b] = channels(value);
  return [r, g, b].map((n) => {
    const c = n / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
}

// A colour and the share of the mix it takes, split off one argument. It
// throws rather than defaulting: a spelling it cannot read would otherwise
// be measured as the literal text it failed to parse.
function shareOf(arg: string, whole: string): [string, number] {
  const share = arg.match(/^(.*)\s(\d+(?:\.\d+)?)%$/);
  if (!share) {
    throw new Error(`${whole}: "${arg}" states no percentage`);
  }
  return [share[1], Number.parseFloat(share[2]) / 100];
}

export function resolve(
  value: string,
  pal: Record<string, string>,
  seen: ReadonlySet<string>,
): string {
  const v = value.trim();
  const named = v.match(/^var\((--[\w-]+)\)$/);
  if (named) {
    if (seen.has(named[1])) {
      throw new Error(`${named[1]} resolves to itself`);
    }
    if (!pal[named[1]]) {
      throw new Error(`${value} names undeclared ${named[1]}`);
    }
    return resolve(pal[named[1]], pal, new Set([...seen, named[1]]));
  }
  const lab = v.match(/^lab\(\s*([\d.]+)%\s+(-?[\d.]+)\s+(-?[\d.]+)\s*\)$/);
  if (lab) {
    const [L, a, b] = lab.slice(1, 4).map(Number.parseFloat);
    return hexOf(labToLinear(L, a, b));
  }
  const mix = v.match(/^color-mix\(\s*in (oklab|srgb)\s*,([\s\S]*)\)$/);
  if (!mix) return v;
  const args = topLevelArgs(mix[2]);
  if (args.length !== 2) {
    throw new Error(`${value} is not a two-colour mix`);
  }
  // color-mix(in srgb, C P%, transparent) is C at alpha P/100, how a tint
  // derives from its base; over an opaque ground instead, sRGB
  // interpolation is exactly that tint composited on the ground.
  if (mix[1] === "srgb") {
    const [colour, alpha] = shareOf(args[0], value);
    const [r, g, b] = channels(resolve(colour, pal, seen));
    const tint = `rgba(${r}, ${g}, ${b}, ${alpha})`;
    return args[1] === "transparent"
      ? tint
      : composite(tint, resolve(args[1], pal, seen));
  }
  const [inkName, part] = shareOf(args[1], value);
  const hue = linearToOklab(linearOf(resolve(args[0], pal, seen)));
  const ink = linearToOklab(linearOf(resolve(inkName, pal, seen)));
  return hexOf(
    oklabToLinear(hue.map((n, i) => n * (1 - part) + ink[i] * part)),
  );
}

// One theme's palette with every colour resolved. Non-colour tokens — the
// space scale, the fonts, a shadow — fall through untouched: they are not
// one of the forms above and nothing here measures them.
function resolved(pal: Record<string, string>): Record<string, string> {
  return Object.fromEntries(
    Object.entries(pal).map(([name, value]) => [
      name,
      resolve(value, pal, new Set([name])),
    ]),
  );
}

// The two palettes every contrast assertion below reads.
export const themes = {
  light: resolved(light),
  dark: resolved({
    ...light,
    ...parseBlock(tokenDecls, '[data-theme="dark"]'),
  }),
} as const;

// The same two AS DECLARED, for the assertions about how a value is
// WRITTEN rather than what it comes out at — a derivation resolves to the
// colour it derives, so a resolved palette cannot tell the two apart.
export const blocks: Record<string, Record<string, string>> = {
  light,
  dark: { ...light, ...parseBlock(tokenDecls, '[data-theme="dark"]') },
};
