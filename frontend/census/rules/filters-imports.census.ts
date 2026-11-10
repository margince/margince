// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The shell asks Filters and views questions (which page an address opens,
// whether it heads itself) without drawing any of it. `filtersaddress.ts` and
// `savedviews.queries.ts` are the light halves that answer; the filter pages,
// the builder and the library load in their own chunk.
//
// The split holds only transitively, so this walks the import graph and fails
// on what is reachable, naming the path.

import { basename, join } from "node:path";
import { SCREEN_ROOTS } from "../../src/testing/screenfiles";
import { importTrail, productionModules, trailsFrom } from "../imports";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

const NOUNS = "(filter|listlibrary|savedviews|viewactions)";
const STARTS_WITH_NOUN = new RegExp(`^${NOUNS}`);
const NAMES_A_NOUN = new RegExp(`(^|[./_-])${NOUNS}`);

/** Pages naming a noun past the start of their name, owned by another screen. */
const ELSEWHERE = new Set([
  "src/features/ai/aicalls-filters.tsx",
  "src/features/analytics/reporting.filters.tsx",
]);

const LIGHT_MODULES = ["filtersaddress.ts", "savedviews.queries.ts"].map(
  (name) => join(srcDir, "lib", name),
);

const pagesOf = (files: readonly CensusFile[]) =>
  productionModules(files, SCREEN_ROOTS).filter((file) =>
    file.path.endsWith(".tsx"),
  );

// The filter UI modules, derived from the names that spell them.
const targetsOf = (pages: readonly CensusFile[]) =>
  new Set(
    pages
      .filter((file) => STARTS_WITH_NOUN.test(basename(file.path)))
      .map((file) => file.path),
  );

const listsDir = join(srcDir, "features", "lists");

export const rule: CensusRule = {
  name: "filters-imports",
  corpus: (tree) =>
    productionModules(tree.files, ["app", "lib", ...SCREEN_ROOTS]),
  // A gate that read a smaller tree would still pass, so it has to see the
  // pages it guards and an edge between them.
  floors: (corpus, tree) => {
    const targets = [...targetsOf(pagesOf(corpus))];
    const trail = importTrail(
      tree,
      [join(listsDir, "filters.tsx")],
      new Set([join(listsDir, "filterbuilder.tsx")]),
    );
    return [
      ...["filters.tsx", "filterbuilder.tsx"]
        .filter((name) => !targets.includes(join(listsDir, name)))
        .map((name) => `src/features/lists/${name} is not a guarded page`),
      ...(trail === null
        ? ["no import path from filters.tsx to filterbuilder.tsx was found"]
        : []),
    ];
  },
  across: (corpus, tree) => {
    const pages = pagesOf(corpus);
    const targets = targetsOf(pages);
    const shell = productionModules(corpus, ["app", "lib"]).map(
      (file) => file.path,
    );
    const reach = trailsFrom(tree, [...shell, ...LIGHT_MODULES], targets).map(
      (trail) => `reaches a filter page: ${trail.join(" -> ")}`,
    );
    // A name the prefix misses, such as `list-filter.tsx`, leaves its page unguarded.
    const named = pages.filter((file) =>
      NAMES_A_NOUN.test(basename(file.path)),
    );
    const unguarded = named
      .filter((file) => !targets.has(file.path) && !ELSEWHERE.has(file.rel))
      .map((file) => `${file.rel}: names a filter noun but is not guarded`);
    const stale = [...ELSEWHERE]
      .filter((rel) => !named.some((file) => file.rel === rel))
      .map(
        (rel) => `${rel}: listed in ELSEWHERE but no such page names a noun`,
      );
    return [...reach, ...unguarded, ...stale];
  },
  advice:
    "The shell (src/app, src/lib) and the light modules filtersaddress.ts and " +
    "savedviews.queries.ts must not reach a filter page, or the pages load " +
    "with the shell. Cut the edge the path names.",
};
