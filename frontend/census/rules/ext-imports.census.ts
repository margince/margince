// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A unit's screen reaches the core only through the published surface, and
// npm only through what its own package declares. The three rules and why a
// parser holds them are written in scripts/ext-imports.test.ts; this runs them
// over the extension files of the tree.

import { join, relative } from "node:path";
import {
  entriesOf,
  judge,
  layersThatReadNothing,
  publishedSubpaths,
  readJson,
  specifiersIn,
  surfacePkg,
  testFile,
} from "../../scripts/ext-imports";
import {
  extensionLayers,
  extensionPackageLayers,
  MODULE_FILE,
} from "../../scripts/lib/source-tree";
import {
  type CensusFile,
  type CensusRule,
  extensionsDir,
  repoRoot,
} from "../tree";

/** The innermost layer holding this file, so a nested layer judges its own. */
const layerOf = (layers: readonly string[], path: string) =>
  layers
    .filter((layer) => path.startsWith(`${layer}/`))
    .sort((a, b) => b.length - a.length)[0];

/** The files read in each layer, and what the three rules find in them. */
export function layerFindings(
  corpus: readonly CensusFile[],
  layers: readonly string[],
): { violations: string[]; files: Map<string, number> } {
  const published = publishedSubpaths(surfacePkg);
  const files = new Map<string, number>();
  const violations = corpus.flatMap((file) => {
    const layer = layerOf(layers, file.path);
    if (layer === undefined) return [];
    files.set(layer, (files.get(layer) ?? 0) + 1);
    const pkg = readJson(join(layer, "package.json"));
    // Shipped code may reach dependencies and peers; a test may also reach
    // devDependencies, so no test runner is pulled into the bundle.
    const shipped = [
      ...entriesOf(pkg.dependencies),
      ...entriesOf(pkg.peerDependencies),
    ];
    const declared = new Map(
      testFile.test(file.path)
        ? [...shipped, ...entriesOf(pkg.devDependencies)]
        : shipped,
    );
    return judge({
      file: file.path,
      shown: relative(repoRoot, file.path),
      layer,
      specifiers: specifiersIn(file.path, file.text),
      published,
      declared,
    });
  });
  return { violations, files };
}

export const rule: CensusRule = {
  name: "ext-imports",
  corpus: (tree) =>
    tree.files.filter(
      (file) => file.area === "extensions" && MODULE_FILE.test(file.path),
    ),
  // A census that judged nothing certifies nothing. Only a layer with a
  // package.json holds source, so each such layer must have had a file read.
  floors: (corpus) => {
    const packageLayers = extensionPackageLayers(extensionsDir);
    const { files } = layerFindings(corpus, extensionLayers(extensionsDir));
    return [
      ...(publishedSubpaths(surfacePkg).length > 0
        ? []
        : ["frontend/package.json publishes no exports"]),
      ...(packageLayers.length > 0
        ? []
        : ["no extension frontend layer holds a package.json"]),
      ...layersThatReadNothing(packageLayers, files).map(
        (layer) =>
          `${relative(repoRoot, layer)} was found and no file in it was read`,
      ),
    ];
  },
  across: (corpus) =>
    layerFindings(corpus, extensionLayers(extensionsDir)).violations,
  advice:
    "An extension imports past the published surface or past its own " +
    "package.json. Reach the core through @margince/frontend/<subpath>, and " +
    "declare every package the unit imports.",
};
