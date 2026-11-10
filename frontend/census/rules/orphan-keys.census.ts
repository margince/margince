// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A key is live only when a module the shipped bundle loads names it. A story,
// a test, a fixture or a module nothing imports vouches for nothing.

import { join } from "node:path";
import type { Module } from "../../scripts/lib/bundle-reach";
import { en } from "../../src/i18n/en";
import { LOCALES } from "../../src/i18n/locale";
import { type CensusRule, srcDir } from "../tree";
import { shippedModules, srcModules } from "./caption-tables.census";

/**
 * Keys no loaded module renders, literally, under a template stem, or as the
 * arm of a plural pair whose base it names.
 */
export function orphanKeys(
  keys: readonly string[],
  modules: Iterable<Module>,
): string[] {
  const loaded = [...modules];
  const literals = new Set(loaded.flatMap((module) => module.literals));
  const stems = [...new Set(loaded.flatMap((module) => module.stems))];
  const catalog = new Set(keys);
  // Only the two arms of a full pair: half a pair is the orphan this finds.
  const underPluralBase = (key: string): boolean => {
    const arm = ["_one", "_other"].find((suffix) => key.endsWith(suffix));
    if (arm === undefined) return false;
    const base = key.slice(0, -arm.length);
    return (
      catalog.has(`${base}_one`) &&
      catalog.has(`${base}_other`) &&
      literals.has(base)
    );
  };
  return keys.filter(
    (key) =>
      !literals.has(key) &&
      !underPluralBase(key) &&
      !stems.some((stem) => key.startsWith(stem)),
  );
}

export const rule: CensusRule = {
  name: "orphan-keys",
  corpus: srcModules,
  across: (corpus) => {
    const catalogs = new Set(
      LOCALES.map((locale) => join(srcDir, "i18n", `${locale}.ts`)),
    );
    return orphanKeys(
      Object.keys(en),
      shippedModules(corpus, catalogs).values(),
    );
  },
  advice:
    "These catalog keys are translated three times and rendered nowhere the " +
    "shipped bundle loads. Delete them from en, de and vi, or render them.",
};
