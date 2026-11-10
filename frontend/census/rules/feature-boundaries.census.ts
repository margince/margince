// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Each feature folder stays its own: only the shell imports a feature, and a
// shared file is not one feature's code. The rules and their ratchet live in
// scripts/feature-boundaries.ts; this runs them over the tree.
//
// `readGraph` walks src/ itself and takes no file list. The floors hold its
// walk to the tree's own files, so a walk that stops short fails here.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  type ImportGraph,
  judge,
  LEDGER,
  ownedFindings,
  ownerOf,
  parseLedger,
  reachFindings,
  readGraph,
  SRC,
} from "../../scripts/feature-boundaries";
import { MODULE_FILE } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule, CensusTree } from "../tree";
import { reachedInTree } from "./bundle-reach.census";

const fromSrc = (file: CensusFile) => file.rel.replace(/^src\//, "");

const graphs = new WeakMap<CensusTree, ImportGraph>();
const walkedGraph = (tree: CensusTree): ImportGraph => {
  const known = graphs.get(tree);
  if (known) return known;
  const graph = readGraph();
  graphs.set(tree, graph);
  return graph;
};

/** The feature folders the tree holds, as `features/<name>`. */
export function featureFolders(corpus: readonly CensusFile[]): string[] {
  const folders = corpus
    .map((file) => /^features\/([^/]+)\//.exec(fromSrc(file))?.[1])
    .filter((name) => name !== undefined)
    .map((name) => `features/${name}`);
  return [...new Set(folders)].sort();
}

/** Each way the walked graph differs from the tree's own files. */
export function graphDrift(
  corpus: readonly CensusFile[],
  graph: ImportGraph,
): string[] {
  const read = new Set(corpus.map(fromSrc));
  const seen = new Set([...graph.keys()].map(ownerOf));
  return [
    ...[...read]
      .filter((path) => !graph.has(path))
      .map((path) => `src/${path} is in the tree and not in the import graph`),
    ...[...graph.keys()]
      .filter((path) => !read.has(path))
      .map((path) => `src/${path} is in the import graph and not in the tree`),
    ...featureFolders(corpus)
      .filter((feature) => !seen.has(feature))
      .map((feature) => `src/${feature} was not read`),
  ];
}

/** The rule over an import graph; the tree's rule reads it with `readGraph`. */
export function boundariesRule(
  graphOf: (tree: CensusTree) => ImportGraph,
): CensusRule {
  return {
    name: "feature-boundaries",
    corpus: (tree) =>
      tree.files.filter(
        (file) =>
          file.area === "src" &&
          (MODULE_FILE.test(file.path) || file.path.endsWith(".css")),
      ),
    floors: (corpus, tree) => [
      ...(featureFolders(corpus).length > 10
        ? []
        : ["10 feature folders or fewer were read"]),
      ...graphDrift(corpus, graphOf(tree)),
    ],
    // Rule 2 counts only importers that ship. A stylesheet ships with the
    // module importing it.
    across: (_corpus, tree) => {
      const graph = graphOf(tree);
      const reached = reachedInTree(tree);
      const ships = (path: string) =>
        path.endsWith(".css") || reached.has(join(SRC, path));
      return judge(
        [...reachFindings(graph), ...ownedFindings(graph, ships)],
        parseLedger(readFileSync(LEDGER, "utf8")),
      );
    },
    advice:
      "A feature imports another feature or the shell, or a shared file " +
      "serves one feature alone. Each line says where the file goes; move it " +
      "with node scripts/move-modules.ts. Never add a line to " +
      "scripts/feature-boundaries-waivers.txt.",
  };
}

export const rule = boundariesRule(walkedGraph);
