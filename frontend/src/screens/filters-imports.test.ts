// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  importPathTo,
  productionModulesUnder,
} from "../../scripts/lib/import-reach";

// The shell asks Filters and views questions — which page an address opens,
// whether it heads itself — without drawing any of it. `filtersaddress.ts` and
// `savedviews.queries.ts` are the light halves that answer; the filter pages,
// the builder and the library are the heavy ones, loaded in their own chunk.
//
// The split only holds TRANSITIVELY, so this walks the import graph and fails
// on what is reachable, naming the path.
//
//  1. No production module under `src/app/**` reaches a filter UI module.
//  2. Neither light module reaches one, or every reader of it is coupled to
//     the pages through it.

const screensDir = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(screensDir, "..");

const NOUNS = "(filter|listlibrary|savedviews|viewactions)";
const STARTS_WITH_NOUN = new RegExp(`^${NOUNS}`);
const NAMES_A_NOUN = new RegExp(`(^|[./_-])${NOUNS}`);
const pages = productionModulesUnder(screensDir)
  .filter((file) => file.endsWith(".tsx"))
  .map((file) => relative(screensDir, file));

/**
 * The filter UI modules, DERIVED from the names that spell them rather than
 * listed: every page whose path starts with one of the destination's nouns,
 * whatever follows it and at any depth.
 */
const targets = new Set(
  pages
    .filter((name) => STARTS_WITH_NOUN.test(name))
    .map((name) => join(screensDir, name)),
);

/** Pages naming a noun past the start of their name, owned by another screen. */
const ELSEWHERE = new Set(["reporting.filters.tsx"]);

const lightModules = ["filtersaddress.ts", "savedviews.queries.ts"].map(
  (name) => join(screensDir, name),
);

/** The import path from `entry` to a filter UI module, or null. */
function pathToAny(entry: string, reached: ReadonlySet<string>) {
  const trail = importPathTo(entry, reached);
  return trail === null ? null : trail.map((file) => relative(srcRoot, file));
}

function violations(entries: readonly string[]): string[] {
  return entries.flatMap((entry) => {
    const trail = pathToAny(entry, targets);
    return trail === null ? [] : [trail.join(" -> ")];
  });
}

describe("the filters split holds transitively", () => {
  it("no shell module reaches a filter page", () => {
    expect(violations(productionModulesUnder(join(srcRoot, "app")))).toEqual(
      [],
    );
  });

  it("neither light module reaches a filter page", () => {
    expect(violations(lightModules)).toEqual([]);
  });

  // A name the prefix misses, such as `list-filter.tsx`, would leave its page
  // unguarded and the gate green, so every page naming a noun is accounted for.
  it("guards every page named for the destination, bar those it names", () => {
    const named = pages.filter((name) => NAMES_A_NOUN.test(name));
    expect(
      named.filter(
        (name) => !targets.has(join(screensDir, name)) && !ELSEWHERE.has(name),
      ),
    ).toEqual([]);
    expect(named).toEqual(expect.arrayContaining([...ELSEWHERE]));
  });

  // A gate that read a smaller tree would still pass, so it has to see what
  // is there: the pages it guards, and an edge between them.
  it("finds the pages it guards, and a path that does exist", () => {
    const named = [...targets].map((file) => relative(screensDir, file));
    expect(named).toEqual(
      expect.arrayContaining(["filters.tsx", "filterbuilder.tsx"]),
    );
    const trail = pathToAny(
      join(screensDir, "filters.tsx"),
      new Set([join(screensDir, "filterbuilder.tsx")]),
    );
    expect(trail?.[0]).toBe("screens/filters.tsx");
    expect(trail?.at(-1)).toBe("screens/filterbuilder.tsx");
  });
});
