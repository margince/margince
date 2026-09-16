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

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const SHIPPED: readonly number[] = [400, 500, 700];
const read = (file: string) => readFileSync(join(frontendRoot, file), "utf8");

const REQUEST = /href="(https:\/\/fonts\.googleapis\.com\/css2\?[^"]*)"/;
const FAMILY = /family=([^:&]+)(?::wght@([^&]*))?/g;
const WEIGHT_DECL = /font-weight\s*:([^;}]*)/g;
const TYPE_VALUE = /(?:^|[;{\s])(?:font|--font[\w-]*)\s*:([^;}]*)/g;
const INLINE_WEIGHT = /fontWeight\s*[:=]\s*["']?(\w+)/g;

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

/**
 * The weight a value asks for, or nothing when it asks for none. A `font`
 * shorthand and a `--font*` token carry it first, ahead of the size; `inherit`
 * asks for the parent's, which is never a fourth weight.
 */
function weightIn(value: string): number | undefined {
  const first = value.trim().split(/[\s/]+/)[0] ?? "";
  const named = NAMED[first.toLowerCase()];
  if (named !== undefined) return named;
  return /^[1-9]00$/.test(first) ? Number(first) : undefined;
}

type Use = Readonly<{ weight: number; where: string }>;

/** Every weight one source asks for, with the line that asks for it. */
function usesIn(file: string, source: string): Use[] {
  const css = file.endsWith(".css");
  // Blanked rather than cut, so a line number still counts to the same line.
  const text = css
    ? source.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, " "))
    : source;
  const patterns = css ? [WEIGHT_DECL, TYPE_VALUE] : [INLINE_WEIGHT];
  return patterns.flatMap((pattern) =>
    [...text.matchAll(pattern)].flatMap((match) => {
      const weight = weightIn(match[1]);
      if (weight === undefined) return [];
      const line = text.slice(0, match.index ?? 0).split("\n").length;
      return [{ weight, where: `${file}:${line}` }];
    }),
  );
}

describe("three weights, and every one of them drawn", () => {
  const request = requested();
  const files = filesMatching(join(frontendRoot, "src"), /\.(css|tsx)$/)
    .filter((file) => !/\.test\.tsx?$/.test(file))
    .map((file) => relative(frontendRoot, file));
  const uses = files.flatMap((file) => usesIn(file, read(file)));

  it("reads the tree it claims", () => {
    expect(files).toContain("src/design-system/tokens.css");
    // A scanner that read nothing reports a clean tree for free: the body and
    // heading tokens each spell a weight, so the census sees at least those.
    expect(uses.length).toBeGreaterThanOrEqual(SHIPPED.length);
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
