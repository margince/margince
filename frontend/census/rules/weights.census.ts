// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Three weights, and a drawn file for every one of them. A weight the app does
// not bundle is synthesized: the engine smears or thins a face nobody drew.
// Both sides are read off the tree: the font faces app.css imports, and what
// the stylesheets ask for. Weight tokens resolve through tokens.css, and a raw
// 400 or 700 is still measured.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { type CensusFile, type CensusRule, frontendRoot } from "../tree";

export const SHIPPED: readonly number[] = [400, 500, 700];
export const TOKENS = "src/design-system/tokens.css";
const read = (file: string) => readFileSync(join(frontendRoot, file), "utf8");

export const FONT_SHEET = "src/app.css";
const FACE_IMPORT = /@import\s+["']@fontsource\/([\w-]+)\/([1-9]00)\.css["']/g;
const FACE_FAMILY = /font-family:\s*["']([^"']+)["']/;
const WEIGHT_DECL = /font-weight\s*:([^;}]*)/g;
const TYPE_VALUE = /(?:^|[;{\s])(?:font|--font[\w-]*)\s*:([^;}]*)/g;
// The whole value, not its first word. An inline weight is a var() string,
// and a capture that stopped at `var` would resolve to nothing and pass.
const INLINE_WEIGHT = /fontWeight\s*[:=]\s*(["'][^"']*["']|[\w-]+)/g;
const WEIGHT_TOKEN = /(--fontWeight[A-Za-z0-9]*)\s*:\s*([^;}]+);/g;

/**
 * Every --fontWeight* token and the number it declares, read off tokens.css,
 * so a new or retuned role token moves this census with it.
 */
export function weightTokens(css: string): Map<string, number> {
  const declarations = css.replace(/\/\*[\s\S]*?\*\//g, "");
  const tokens = new Map<string, number>();
  for (const [, name, value] of declarations.matchAll(WEIGHT_TOKEN)) {
    tokens.set(name, Number(value.trim()));
  }
  if (tokens.size === 0) {
    throw new Error(`${TOKENS} declares no --fontWeight* token`);
  }
  return tokens;
}

/** The family a family token resolves to: the quoted name in its value. */
export function declaredFamily(token: string): string {
  const css = read(TOKENS);
  const declared = new RegExp(`--${token}\\s*:\\s*"([^"]+)"`).exec(css);
  if (!declared) throw new Error(`tokens.css declares no --${token}`);
  return declared[1];
}

/**
 * Every family app.css bundles, with the weights it imports for each. The
 * family's name is read from the face sheet the import loads. A package named
 * apart from its family then counts under the family tokens.css names.
 */
export function requested(): Map<string, number[]> {
  const families = new Map<string, number[]>();
  for (const [, pkg, weight] of read(FONT_SHEET).matchAll(FACE_IMPORT)) {
    const face = read(
      join("node_modules", "@fontsource", pkg, `${weight}.css`),
    );
    const family = FACE_FAMILY.exec(face)?.[1];
    if (!family)
      throw new Error(`@fontsource/${pkg}/${weight}.css names no family`);
    families.set(family, [...(families.get(family) ?? []), Number(weight)]);
  }
  if (families.size === 0)
    throw new Error(`${FONT_SHEET} imports no font face`);
  for (const weights of families.values()) weights.sort((a, b) => a - b);
  return families;
}

const NAMED: Readonly<Record<string, number>> = { normal: 400, bold: 700 };

type Ask = Readonly<{ weight: number; viaToken: boolean }>;

/**
 * The weight a value asks for, or nothing when it asks for none. A `font`
 * shorthand and a `--font*` token carry it first, as a weight token or a
 * number. An unknown token resolves to nothing, and the floors catch that.
 */
function weightIn(
  value: string,
  tokens: ReadonlyMap<string, number>,
): Ask | undefined {
  const first =
    value
      .trim()
      .replace(/^["']|["']$/g, "")
      .split(/[\s/]+/)[0] ?? "";
  const reference = /^var\(\s*(--[\w-]+)/.exec(first);
  if (reference) {
    const weight = tokens.get(reference[1]);
    return weight === undefined ? undefined : { weight, viaToken: true };
  }
  const named = NAMED[first.toLowerCase()];
  if (named !== undefined) return { weight: named, viaToken: false };
  if (!/^[1-9]00$/.test(first)) return undefined;
  return { weight: Number(first), viaToken: false };
}

type Use = Readonly<{ weight: number; viaToken: boolean; where: string }>;

/** Every weight one source asks for, with the line that asks for it. */
export function usesIn(
  file: string,
  source: string,
  tokens: ReadonlyMap<string, number>,
): Use[] {
  const css = file.endsWith(".css");
  // Blanked rather than cut, so a line number still counts to the same line.
  const text = css
    ? source.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, " "))
    : source;
  const patterns = css ? [WEIGHT_DECL, TYPE_VALUE] : [INLINE_WEIGHT];
  return patterns.flatMap((pattern) =>
    [...text.matchAll(pattern)].flatMap((match) => {
      const ask = weightIn(match[1], tokens);
      if (ask === undefined) return [];
      const line = text.slice(0, match.index ?? 0).split("\n").length;
      return [{ ...ask, where: `${file}:${line}` }];
    }),
  );
}

const judged = (file: CensusFile) =>
  file.area === "src" &&
  /\.(css|tsx)$/.test(file.rel) &&
  !/\.test\.tsx?$/.test(file.rel);

function uses(corpus: readonly CensusFile[]): Use[] {
  const sheet = corpus.find((file) => file.rel === TOKENS);
  const tokens = sheet ? weightTokens(sheet.text) : new Map<string, number>();
  return corpus.flatMap((file) => usesIn(file.rel, file.text, tokens));
}

export const rule: CensusRule = {
  name: "weights",
  corpus: (tree) => tree.files.filter(judged),
  // A scanner or a resolver that read nothing reports a clean tree. The three
  // weight tokens spell a number, and the type scale asks through them.
  floors: (corpus) => {
    const found = uses(corpus);
    return [
      ...(corpus.some((file) => file.rel === TOKENS)
        ? []
        : [`${TOKENS} was not read`]),
      ...(found.length >= SHIPPED.length
        ? []
        : ["fewer weights were read than the request loads"]),
      ...(found.filter(({ viaToken }) => viaToken).length > SHIPPED.length
        ? []
        : ["no weight resolved through a token"]),
    ];
  },
  across: (corpus) =>
    uses(corpus)
      .filter(({ weight }) => !SHIPPED.includes(weight))
      .map(({ weight, where }) => `${where} asks for ${weight}`),
  advice:
    "Ask for a weight through --fontWeightRegular, Medium or Bold. app.css " +
    "bundles 400, 500 and 700 only; any other weight is synthesized.",
};
