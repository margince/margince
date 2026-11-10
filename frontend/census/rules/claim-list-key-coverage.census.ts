// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The list a claim invalidates is a list something reads. `useClaimRecord`
// invalidates the list its record is filed under. A key that matches no cache
// raises no error, and the list goes on showing the record unowned.
//
// The keys come from recordlistkeys.ts, so a new claimable kind is covered the
// day it is added.

import { join } from "node:path";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

export const keysModule = join(srcDir, "lib", "recordlistkeys.ts");

/** The list keys `RECORD_LIST_KEY` declares, or null when it is not a literal. */
export function declaredListKeys(source: string): string[] | null {
  const table =
    /const RECORD_LIST_KEY: Record<ListedRecordType, string> = \{([^}]*)\}/s.exec(
      source,
    );
  return table ? [...table[1].matchAll(/"([^"]+)"/g)].map((m) => m[1]) : null;
}

/** Each declared key no file in the corpus spells as a string. */
export function unreadKeys(
  keys: readonly string[],
  corpus: readonly CensusFile[],
): string[] {
  return keys
    .filter((key) => !corpus.some((file) => file.text.includes(`"${key}"`)))
    .map((key) => `"${key}" is invalidated by a claim and queried nowhere`);
}

export const rule: CensusRule = {
  name: "claim-list-key-coverage",
  // Every module under src, since a rail, a hook or a prefetch can read a list
  // key as well as a screen.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.area === "src" &&
        /\.tsx?$/.test(file.path) &&
        !file.path.endsWith(".d.ts") &&
        file.path !== keysModule,
    ),
  floors: (_corpus, tree) => {
    const keys = declaredListKeys(tree.at(keysModule)?.text ?? "");
    if (keys === null) {
      return [
        "recordlistkeys.ts no longer declares RECORD_LIST_KEY as an object literal; this census reads it textually",
      ];
    }
    return keys.length >= 4 ? [] : ["fewer than 4 list keys are declared"];
  },
  across: (corpus, tree) =>
    unreadKeys(declaredListKeys(tree.at(keysModule)?.text ?? "") ?? [], corpus),
  advice:
    "A claim invalidates a list key nothing queries, so the list goes on " +
    "showing the old owner. Make RECORD_LIST_KEY in lib/recordlistkeys.ts " +
    "name the key the list's query uses.",
};
