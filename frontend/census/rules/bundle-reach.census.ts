// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The modules the shipped bundle loads, read from the census tree. A census
// asking whether anything the user runs touches a file reads this set. An empty
// set would report a clean tree from nothing.

import {
  buildScriptEntries,
  documentEntries,
  type Module,
  reachedModulesIn,
  surfaceEntries,
} from "../../scripts/lib/bundle-reach";
import { MODULE_FILE } from "../../scripts/lib/source-tree";
import type { CensusRule, CensusTree } from "../tree";

const reachedByTree = new WeakMap<CensusTree, ReadonlyMap<string, Module>>();

/** The modules the shipped bundle loads out of the tree's modules under src/. */
export function reachedInTree(tree: CensusTree): ReadonlyMap<string, Module> {
  const known = reachedByTree.get(tree);
  if (known) return known;
  const reached = reachedModulesIn(
    new Map(
      tree.files
        .filter((file) => file.area === "src" && MODULE_FILE.test(file.path))
        .map((file): [string, string] => [file.path, file.text]),
    ),
  );
  reachedByTree.set(tree, reached);
  return reached;
}

export const rule: CensusRule = {
  name: "bundle-reach",
  corpus: (tree) => tree.files.filter((file) => file.area === "src"),
  // The floors sit well below the real counts. They catch a walk that stopped
  // walking, not how many modules the app has.
  floors: (_corpus, tree) => [
    ...(documentEntries().length > 1 ? [] : ["fewer than 2 document entries"]),
    ...(surfaceEntries().length > 0 ? [] : ["no package.json surface entry"]),
    ...(buildScriptEntries().length > 0
      ? []
      : ["no module named by the PWA build script"]),
    ...(reachedInTree(tree).size > 200
      ? []
      : ["the bundle reaches 200 modules or fewer"]),
  ],
  // Every entry names a file the tree holds, so none was misread.
  across: (_corpus, tree) =>
    [...documentEntries(), ...surfaceEntries(), ...buildScriptEntries()]
      .filter((entry) => !tree.at(entry))
      .map((entry) => `${entry} is a bundle entry the tree does not hold`),
  advice:
    "A bundle entry names a file that is not there. Fix the index.html, the " +
    "package.json export or the PWA build script that names it.",
};
