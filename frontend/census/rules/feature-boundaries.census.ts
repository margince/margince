// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Each feature folder stays its own: only the shell imports a feature, and a
// shared file is not one feature's code. The rules and their ratchet live in
// scripts/feature-boundaries.ts; this runs them over the tree.
//
// The graph is read from the tree. An import the tree cannot resolve and the
// disk can is an edge the graph lost, so the floors fail it.

import { readFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import {
  DISK,
  type GraphReader,
  type ImportGraph,
  judge,
  LEDGER,
  ownedFindings,
  parseLedger,
  reachFindings,
  readGraphWithMisses,
  SRC,
} from "../../scripts/feature-boundaries";
import { MODULE_FILE, moduleAt } from "../../scripts/lib/source-tree";
import { stable } from "../../src/format/collate";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  TREE_FILE,
} from "../tree";
import { reachedInTree } from "./bundle-reach.census";

const fromSrc = (file: CensusFile) => file.rel.replace(/^src\//, "");

const inGraph = (file: CensusFile) =>
  file.area === "src" &&
  (MODULE_FILE.test(file.path) || file.path.endsWith(".css"));

// The walk equals git under src/, so the tree answers there for each kind it
// walks. The disk answers only for an asset the walk does not read.
const treeReader = (tree: CensusTree): GraphReader => {
  const fromTree = (path: string) =>
    path.startsWith(`${SRC}${sep}`) && TREE_FILE.test(path);
  return {
    text: (path) => tree.at(path)?.text ?? DISK.text(path),
    source: (path) => tree.at(path)?.source ?? DISK.source(path),
    isModule: (path) =>
      fromTree(path)
        ? MODULE_FILE.test(path) && tree.at(path) !== undefined
        : DISK.isModule(path),
    isFile: (path) =>
      fromTree(path) ? tree.at(path) !== undefined : DISK.isFile(path),
  };
};

const reads = new WeakMap<CensusTree, ReturnType<typeof readGraphWithMisses>>();
const treeRead = (tree: CensusTree) => {
  const known = reads.get(tree);
  if (known) return known;
  const files = tree.files.filter(inGraph).map((file) => file.path);
  const read = readGraphWithMisses(SRC, files, treeReader(tree));
  reads.set(tree, read);
  return read;
};

const treeGraph = (tree: CensusTree): ImportGraph => treeRead(tree).graph;

/** Each import the tree resolves to nothing and the disk resolves. */
function lostEdges(tree: CensusTree): string[] {
  const fromRepo = (path: string) => relative(join(SRC, ".."), path);
  return treeRead(tree)
    .unresolved.filter(
      ({ path }) => moduleAt(path, DISK.isModule) !== null || DISK.isFile(path),
    )
    .map(
      ({ file, path }) =>
        `${fromRepo(file)} imports ${fromRepo(path)}, which is on disk and not in the tree`,
    );
}

/** The feature folders the tree holds, as `features/<name>`. */
export function featureFolders(corpus: readonly CensusFile[]): string[] {
  const folders = corpus
    .map((file) => /^features\/([^/]+)\//.exec(fromSrc(file))?.[1])
    .filter((name) => name !== undefined)
    .map((name) => `features/${name}`);
  return [...new Set(folders)].sort(stable);
}

export const rule: CensusRule = {
  name: "feature-boundaries",
  corpus: (tree) => tree.files.filter(inGraph),
  floors: (corpus, tree) => [
    ...(featureFolders(corpus).length > 10
      ? []
      : ["10 feature folders or fewer were read"]),
    ...lostEdges(tree),
  ],
  // Rule 2 counts only importers that ship. A stylesheet ships with the
  // module importing it.
  across: (_corpus, tree) => {
    const graph = treeGraph(tree);
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
