// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The import graph of the parsed tree, for rules that ask what one module
// reaches through its imports.

import { basename } from "node:path";
import { moduleSpecifiers, resolveRelative } from "../scripts/lib/source-tree";
import type { CensusFile, CensusTree } from "./tree";

// Tests, stories and the kits only they import ship in no chunk.
const NOT_SHIPPED = /\.(test|stories)\.tsx?$|\.(testkit|storykit)\.tsx?$/;

/** Every shipped TypeScript module of the tree under `src/<dir>/`. */
export const productionModules = (
  files: readonly CensusFile[],
  dirs: readonly string[],
): CensusFile[] =>
  files.filter(
    (file) =>
      file.area === "src" &&
      dirs.some((dir) => file.rel.startsWith(`src/${dir}/`)) &&
      /\.tsx?$/.test(file.path) &&
      !NOT_SHIPPED.test(basename(file.path)),
  );

// Every walk crosses the same shared modules, so each file's edges resolve once.
const edgesByTree = new WeakMap<CensusTree, Map<string, string[]>>();

function edgesOf(tree: CensusTree, path: string): string[] {
  let edges = edgesByTree.get(tree);
  if (!edges) {
    edges = new Map();
    edgesByTree.set(tree, edges);
  }
  const known = edges.get(path);
  if (known) return known;
  const source = tree.at(path)?.source;
  const inTree = (candidate: string) => tree.at(candidate) !== undefined;
  const found = source
    ? moduleSpecifiers(source, "all").flatMap((specifier) => {
        const next = resolveRelative(path, specifier, inTree);
        return next === null ? [] : [next];
      })
    : [];
  edges.set(path, found);
  return found;
}

/**
 * The shortest import path from the nearest of `entries` to any of `targets`,
 * as paths from frontend/, or null. Type-only imports count: they couple the
 * two halves, and a value import across them would pass review.
 */
export function importTrail(
  tree: CensusTree,
  entries: readonly string[],
  targets: ReadonlySet<string>,
): string[] | null {
  const rel = (path: string) => tree.at(path)?.rel ?? path;
  const hit = entries.find((entry) => targets.has(entry));
  if (hit !== undefined) return [rel(hit)];
  const seen = new Set(entries);
  const queue: string[][] = entries.map((entry) => [entry]);
  for (let trail = queue.shift(); trail; trail = queue.shift()) {
    for (const next of edgesOf(tree, trail[trail.length - 1] ?? "")) {
      if (seen.has(next)) continue;
      if (targets.has(next)) return [...trail, next].map(rel);
      seen.add(next);
      queue.push([...trail, next]);
    }
  }
  return null;
}

/** One trail per entry that reaches a target. A clean graph costs one walk. */
export function trailsFrom(
  tree: CensusTree,
  entries: readonly string[],
  targets: ReadonlySet<string>,
): string[][] {
  if (importTrail(tree, entries, targets) === null) return [];
  return entries.flatMap((entry) => {
    const trail = importTrail(tree, [entry], targets);
    return trail === null ? [] : [trail];
  });
}
