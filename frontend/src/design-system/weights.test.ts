// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { filesMatching } from "../../scripts/lib/source-tree";

// Three weights, and a drawn file for every one of them. A weight the request
// does not load is not refused, it is SYNTHESIZED: an engine smears or thins a
// face nobody drew and it looks almost right. Both sides are read off the tree
// — what index.html loads, what the stylesheets ask for — and neither may be
// empty: no request, a family with no weights, or an empty corpus fails here
// rather than passing for free.
//
// The tree no longer SPELLS a weight: the type scale reads --fontWeightRegular
// / Medium / Bold, and this census resolves them through tokens.css rather than
// carrying its own copy of what each one is worth. Derived, because a listed
// map is a second spelling of the tokens and would keep reporting PASS after
// one of them was retuned. A raw 400 or 700 anywhere else is still seen and
// still measured — resolving a token must not make the scanner blind to a
// number, which is the one failure with no assertion to notice.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const SHIPPED: readonly number[] = [400, 500, 700];
const TOKENS = "src/design-system/tokens.css";
const read = (file: string) => readFileSync(join(frontendRoot, file), "utf8");

const REQUEST = /href="(https:\/\/fonts\.googleapis\.com\/css2\?[^"]*)"/;
const FAMILY = /family=([^:&]+)(?::wght@([^&]*))?/g;
const WEIGHT_DECL = /font-weight\s*:([^;}]*)/g;
const TYPE_VALUE = /(?:^|[;{\s])(?:font|--font[\w-]*)\s*:([^;}]*)/g;
// The whole value, not its first word: an inline weight is now a var() string,
// and a capture that stopped at `var` would resolve to nothing and pass.
const INLINE_WEIGHT = /fontWeight\s*[:=]\s*(["'][^"']*["']|[\w-]+)/g;
const WEIGHT_TOKEN = /(--fontWeight[A-Za-z0-9]*)\s*:\s*([^;}]+);/g;

/**
 * Every --fontWeight* token and the number it declares, read off tokens.css.
 * Derived rather than listed, so a fourth role token cannot arrive unmeasured
 * and a retuned one moves this census with it.
 */
function weightTokens(): Map<string, number> {
  const declarations = read(TOKENS).replace(/\/\*[\s\S]*?\*\//g, "");
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
function declaredFamily(token: string): string {
  const css = read("src/design-system/tokens.css");
  const declared = new RegExp(`--${token}\\s*:\\s*"([^"]+)"`).exec(css);
  if (!declared) throw new Error(`tokens.css declares no --${token}`);
  return declared[1];
}

/** Every family index.html asks for, with the weights it asks for each in. */
function requested(): Map<string, number[]> {
  const href = REQUEST.exec(read("index.html"));
  if (!href) throw new Error("index.html requests no font stylesheet");
  const families = new Map<string, number[]>();
  for (const family of href[1].matchAll(FAMILY)) {
    const name = decodeURIComponent(family[1]).replaceAll("+", " ");
    const asked = (family[2] ?? "").split(";");
    const weights = asked.filter((w) => /^[1-9]00$/.test(w)).map(Number);
    if (weights.length === 0) throw new Error(`${name} loads no weight`);
    weights.sort((a, b) => a - b);
    families.set(name, weights);
  }
  if (families.size === 0) throw new Error("the request names no family");
  return families;
}

const NAMED: Readonly<Record<string, number>> = { normal: 400, bold: 700 };

type Ask = Readonly<{ weight: number; viaToken: boolean }>;

/**
 * The weight a value asks for, or nothing when it asks for none. A `font`
 * shorthand and a `--font*` token carry it first, ahead of the size, spelled
 * either as a weight token or as a number; `inherit` asks for the parent's,
 * which is never a fourth weight. An unknown token resolves to nothing, and
 * the corpus assertion below is what keeps that from passing for free.
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
function usesIn(
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

describe("three weights, and every one of them drawn", () => {
  const request = requested();
  const tokens = weightTokens();
  const files = filesMatching(join(frontendRoot, "src"), /\.(css|tsx)$/)
    .filter((file) => !/\.test\.tsx?$/.test(file))
    .map((file) => relative(frontendRoot, file));
  const uses = files.flatMap((file) => usesIn(file, read(file), tokens));

  it("reads the tree it claims", () => {
    expect(files).toContain(TOKENS);
    // A scanner that read nothing reports a clean tree for free: the three
    // weight tokens spell a number, so the census sees at least those.
    expect(uses.length).toBeGreaterThanOrEqual(SHIPPED.length);
    // And a RESOLVER that read nothing does the same, one step in: every size
    // in the type scale now asks through a token, so a resolution that quietly
    // returned nothing would report a tree with no weights in it and pass.
    const resolved = uses.filter(({ viaToken }) => viaToken);
    expect(
      resolved.length,
      "no weight resolved through a token",
    ).toBeGreaterThan(SHIPPED.length);
  });

  // A role token pointing at a face nobody loaded synthesizes just as surely as
  // a spelled 600 does, and it does it at every call site at once.
  it("declares no weight token the request does not load", () => {
    const rogue = [...tokens]
      .filter(([, weight]) => !SHIPPED.includes(weight))
      .map(([name, weight]) => `${name} is ${weight}`);
    expect(rogue, rogue.join("\n")).toEqual([]);
  });

  it("loads exactly 400, 500 and 700 for both text families", () => {
    for (const token of ["fontFamilyHeading", "fontFamilyBody"]) {
      const family = declaredFamily(token);
      expect(request.get(family), family).toEqual([...SHIPPED]);
    }
  });

  it("loads the code face in none but those weights", () => {
    const mono = declaredFamily("fontFamilyMono");
    const loaded = request.get(mono);
    expect(loaded, `${mono} is not requested`).toBeDefined();
    expect(loaded?.filter((weight) => !SHIPPED.includes(weight))).toEqual([]);
  });

  it("asks for no weight the request does not load", () => {
    const synthesized = uses
      .filter(({ weight }) => !SHIPPED.includes(weight))
      .map(({ weight, where }) => `${where} asks for ${weight}`);
    expect(synthesized, synthesized.join("\n")).toEqual([]);
  });
});
