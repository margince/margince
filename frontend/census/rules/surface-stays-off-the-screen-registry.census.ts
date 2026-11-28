// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A unit imports the published surface, and loading the screen registry loads
// every unit, so a surface that reached the registry would import itself.

import { join } from "node:path";
import { reachedModulesIn } from "../../scripts/lib/bundle-reach";
import { MODULE_FILE } from "../../scripts/lib/source-tree";
import { type CensusRule, type CensusTree, srcDir } from "../tree";

const surfaceEntry = join(srcDir, "surface", "index.ts");
const extensionRoute = join(srcDir, "lib", "extensionscreen.ts");
const screenRegistry = join(srcDir, "composition", "extscreens.gen.ts");

const reachedFromSurface = (tree: CensusTree) =>
  reachedModulesIn(
    new Map(
      tree.files
        .filter((file) => file.area === "src" && MODULE_FILE.test(file.path))
        .map((file): [string, string] => [file.path, file.text]),
    ),
    {},
    [surfaceEntry],
  );

export const rule: CensusRule = {
  name: "surface-stays-off-the-screen-registry",
  corpus: (tree) => tree.files.filter((file) => file.area === "src"),
  // The surface reaches the unit route's spelling today; a walk that misses it
  // stopped walking, and would report the registry as unreached from nothing.
  floors: (_corpus, tree) => [
    ...(tree.at(surfaceEntry)
      ? []
      : ["the tree holds no src/surface/index.ts"]),
    ...(reachedFromSurface(tree).has(extensionRoute)
      ? []
      : ["the surface does not reach src/lib/extensionscreen.ts"]),
  ],
  across: (_corpus, tree) =>
    reachedFromSurface(tree).has(screenRegistry)
      ? ["src/surface/index.ts loads src/composition/extscreens.gen.ts"]
      : [],
  advice:
    "The published surface reaches the units' screen registry, so a unit " +
    "importing the surface would import itself. Move the import that reaches " +
    "it behind a lazy load, or out of the surface.",
};
