// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A key is live only when code the shipped bundle runs names it.
// An unrendered export, a story, a test or an unimported module vouches for
// nothing. A template vouches only for what its types can produce.

import { join, relative } from "node:path";
import {
  bundleEntries,
  reachedModulesIn,
} from "../../scripts/lib/bundle-reach";
import { type KeyUse, liveKeyUse, usesKey } from "../../scripts/lib/key-reach";
import { en } from "../../src/i18n/en";
import { LOCALES } from "../../src/i18n/locale";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  srcDir,
} from "../tree";
import { srcModules } from "./caption-tables.census";

/**
 * Keys no live code renders. A key is rendered when it is named literally or a
 * template's types can produce it. It is also rendered as an arm of a plural
 * pair whose base is.
 */
export function orphanKeys(keys: readonly string[], use: KeyUse): string[] {
  const catalog = new Set(keys);
  // Only the two arms of a full pair: half a pair is the orphan this finds.
  const underPluralBase = (key: string): boolean => {
    const arm = ["_one", "_other"].find((suffix) => key.endsWith(suffix));
    if (arm === undefined) return false;
    const base = key.slice(0, -arm.length);
    return (
      catalog.has(`${base}_one`) &&
      catalog.has(`${base}_other`) &&
      usesKey(use, base)
    );
  };
  return keys.filter((key) => !usesKey(use, key) && !underPluralBase(key));
}

// The floors and the findings read one pass over the same corpus.
const used = new WeakMap<readonly CensusFile[], KeyUse>();

function keyUseOf(corpus: readonly CensusFile[], tree: CensusTree): KeyUse {
  const known = used.get(corpus);
  if (known) return known;
  const catalogs = new Set(
    LOCALES.map((locale) => join(srcDir, "i18n", `${locale}.ts`)),
  );
  const entries = bundleEntries();
  const loaded = reachedModulesIn(
    new Map(corpus.map((file) => [file.path, file.text])),
    { catalogs },
    entries,
  );
  const use = liveKeyUse(tree.checked(), loaded, entries, catalogs);
  used.set(corpus, use);
  return use;
}

export const rule: CensusRule = {
  name: "orphan-keys",
  corpus: srcModules,
  // A pass that skipped a loaded module would call dead a key it renders.
  floors: (corpus, tree) => {
    const use = keyUseOf(corpus, tree);
    return [
      ...use.missing.map(
        (path) =>
          `${relative(srcDir, path)} is loaded but absent from the checked program, so its keys were not read`,
      ),
      ...(use.literals.size > 1000
        ? []
        : [`the pass read ${use.literals.size} literals, not over 1000`]),
    ];
  },
  across: (corpus, tree) => orphanKeys(Object.keys(en), keyUseOf(corpus, tree)),
  advice:
    "These catalog keys are translated three times and rendered nowhere the " +
    "shipped bundle loads. Delete them from en, de and vi, or render them.",
};
