// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which modules an import walk reaches, and by which path: the question every
// split gate asks ("the shell must not reach the settings cards", "the light
// filters modules must not reach the pages"). A split only holds TRANSITIVELY
// — `light -> other -> heavy` pulls the heavy half in while every direct edge
// looks fine — so these gates walk the graph rather than grep one hop.
//
// Shared, so every split gate reads one corpus: a gate with a narrower idea of
// what ships reads a smaller tree and reports the same word, PASS.

import { readFileSync } from "node:fs";
import { basename } from "node:path";
import type ts from "typescript";
import {
  filesMatching,
  moduleSpecifiers,
  parseSource,
  resolveRelative,
  sourceFileAt,
} from "./source-tree";
import { isDocsPage } from "./story-files";

// Tests, stories and testkits ask for both halves on purpose, and ship in no
// chunk, so a walk from one proves nothing about the bundle.
const NOT_SHIPPED = /\.(test|stories)\.tsx?$|\.testkit\.tsx?$/;

/** Every shipped TypeScript module under `dir`, at any depth. */
export function productionModulesUnder(dir: string): string[] {
  return filesMatching(dir, /\.tsx?$/).filter(
    (file) => !NOT_SHIPPED.test(basename(file)),
  );
}

type Edges = "all" | "values";

// Every walk crosses the same shared subgraph — the design system, the api
// client, i18n — so a module is parsed once for the run, not once per entry
// that reaches it. The tree does not change while a suite runs.
const resolvedImports: Record<Edges, Map<string, string[]>> = {
  all: new Map(),
  values: new Map(),
};

// An MDX page's imports are ESM lines outside its code fences; the prose and
// samples around them would parse into specifiers nothing loads.
const MDX_FENCE = /^```[\s\S]*?^```/gm;
const MDX_IMPORT = /^import\s[^;]*?["'][^"'\n]+["'];?/gm;

function sourceOf(file: string): ts.SourceFile {
  if (!isDocsPage(file)) {
    return sourceFileAt(file);
  }
  const imports =
    readFileSync(file, "utf8").replace(MDX_FENCE, "").match(MDX_IMPORT) ?? [];
  return parseSource(file, imports.join("\n"));
}

function edgesOf(file: string, edges: Edges): string[] {
  const known = resolvedImports[edges].get(file);
  if (known) {
    return known;
  }
  const found = moduleSpecifiers(sourceOf(file), edges)
    .map((specifier) => resolveRelative(file, specifier))
    .filter((next): next is string => next !== null);
  resolvedImports[edges].set(file, found);
  return found;
}

/**
 * The shortest import path from `entry` to any of `targets`, as absolute
 * paths from the entry on, or null when none is reachable. A path rather than
 * a boolean, because the offending edge is usually several hops in and
 * invisible from the entry.
 */
export function importPathTo(
  entry: string,
  targets: ReadonlySet<string> | ((file: string) => boolean),
  // Type-only edges count by default: a type import naming the heavy half
  // couples the two, and a later value import across it would pass review.
  edges: Edges = "all",
): string[] | null {
  const isTarget =
    typeof targets === "function"
      ? targets
      : (file: string) => targets.has(file);
  if (isTarget(entry)) {
    return [entry];
  }
  const seen = new Set([entry]);
  const queue: string[][] = [[entry]];
  for (let trail = queue.shift(); trail; trail = queue.shift()) {
    for (const next of edgesOf(trail[trail.length - 1], edges)) {
      if (seen.has(next)) {
        continue;
      }
      if (isTarget(next)) {
        return [...trail, next];
      }
      seen.add(next);
      queue.push([...trail, next]);
    }
  }
  return null;
}

// `vitest`, `vitest/*`, `@vitest/*`, and a package's own `vitest` entry such as
// `@testing-library/jest-dom/vitest`, which imports vitest in turn.
const TEST_RUNNER = /^(?!\.)(?:[^/]+\/)*@?vitest(?:\/|$)/;

const runnerModules = new Map<string, boolean>();

export function loadsTestRunner(file: string): boolean {
  const known = runnerModules.get(file);
  if (known !== undefined) {
    return known;
  }
  const loads = moduleSpecifiers(sourceOf(file), "values").some((specifier) =>
    TEST_RUNNER.test(specifier),
  );
  runnerModules.set(file, loads);
  return loads;
}

/** Each entry's value-import path to a module that loads the test runner. */
export function testRunnerReach(entries: readonly string[]): string[][] {
  return entries
    .map((entry) => importPathTo(entry, loadsTestRunner, "values"))
    .filter((path): path is string[] => path !== null);
}
