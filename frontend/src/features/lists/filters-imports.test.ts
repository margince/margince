// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { basename, dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  importPathTo,
  productionModulesUnder,
} from "../../../scripts/lib/import-reach";
import { SCREEN_ROOTS } from "../../testing/screenfiles";

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

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(here, "../..");

const NOUNS = "(filter|listlibrary|savedviews|viewactions)";
const STARTS_WITH_NOUN = new RegExp(`^${NOUNS}`);
const NAMES_A_NOUN = new RegExp(`(^|[./_-])${NOUNS}`);
const pages = SCREEN_ROOTS.flatMap((root) =>
  productionModulesUnder(join(srcRoot, root)),
)
  .filter((file) => file.endsWith(".tsx"))
  .map((file) => relative(srcRoot, file));

/**
 * The filter UI modules, DERIVED from the names that spell them rather than
 * listed. A page is one when its file name, in any screen folder, starts with
 * one of the destination's nouns.
 */
const targets = new Set(
  pages
    .filter((name) => STARTS_WITH_NOUN.test(basename(name)))
    .map((name) => join(srcRoot, name)),
);

/** Pages naming a noun past the start of their name, owned by another screen. */
const ELSEWHERE = new Set([
  "features/ai/aicalls-filters.tsx",
  "features/analytics/reporting.filters.tsx",
]);

const lightModules = ["filtersaddress.ts", "savedviews.queries.ts"].map(
  (name) => join(srcRoot, "lib", name),
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
    // lib/ is the shell's own infrastructure, loaded with it.
    const shell = ["app", "lib"].flatMap((dir) =>
      productionModulesUnder(join(srcRoot, dir)),
    );
    expect(violations(shell)).toEqual([]);
  });

  it("neither light module reaches a filter page", () => {
    expect(violations(lightModules)).toEqual([]);
  });

  // A name the prefix misses, such as `list-filter.tsx`, would leave its page
  // unguarded and the gate green, so every page naming a noun is accounted for.
  it("guards every page named for the destination, bar those it names", () => {
    const named = pages.filter((name) => NAMES_A_NOUN.test(basename(name)));
    expect(
      named.filter(
        (name) => !targets.has(join(srcRoot, name)) && !ELSEWHERE.has(name),
      ),
    ).toEqual([]);
    expect(named).toEqual(expect.arrayContaining([...ELSEWHERE]));
  });

  // A gate that read a smaller tree would still pass, so it has to see what
  // is there: the pages it guards, and an edge between them.
  it("finds the pages it guards, and a path that does exist", () => {
    const named = [...targets].map((file) => relative(srcRoot, file));
    expect(named).toEqual(
      expect.arrayContaining([
        "features/lists/filters.tsx",
        "features/lists/filterbuilder.tsx",
      ]),
    );
    const trail = pathToAny(
      join(here, "filters.tsx"),
      new Set([join(here, "filterbuilder.tsx")]),
    );
    expect(trail?.[0]).toBe("features/lists/filters.tsx");
    expect(trail?.at(-1)).toBe("features/lists/filterbuilder.tsx");
  });
});
