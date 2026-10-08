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

// "scanned" is a text scan for a string after `from` or `import`: a superset
// of "all" at a hundredth of a parse, so it can clear a graph but not convict.
type Edges = "all" | "values" | "scanned";

const GAP = String.raw`(?:\s|/\*[\s\S]*?\*/|//[^\n]*\n)*`;
const SPECIFIER_SCAN = new RegExp(
  String.raw`\b(?:from|import)${GAP}\(?${GAP}["'\`]([^"'\`\n]+)["'\`]`,
  "g",
);

// Every walk crosses the same shared subgraph — the design system, the api
// client, i18n — so a module is parsed once for the run, not once per entry
// that reaches it. The tree does not change while a suite runs.
const resolvedImports: Record<Edges, Map<string, string[]>> = {
  all: new Map(),
  values: new Map(),
  scanned: new Map(),
};

// An MDX page's imports are ESM lines outside its code fences; the prose and
// samples around them would parse into specifiers nothing loads.
const MDX_FENCE = /^```[\s\S]*?^```/gm;
const MDX_IMPORT = /^import\s[^;]*?["'][^"'\n]+["'];?/gm;

function importText(file: string): string {
  const text = readFileSync(file, "utf8");
  if (!isDocsPage(file)) {
    return text;
  }
  return (text.replace(MDX_FENCE, "").match(MDX_IMPORT) ?? []).join("\n");
}

function sourceOf(file: string): ts.SourceFile {
  return isDocsPage(file)
    ? parseSource(file, importText(file))
    : sourceFileAt(file);
}

function specifiersOf(file: string, edges: Edges): string[] {
  return edges === "scanned"
    ? [...importText(file).matchAll(SPECIFIER_SCAN)].map((match) => match[1])
    : moduleSpecifiers(sourceOf(file), edges);
}

export function edgesOf(file: string, edges: Edges): string[] {
  const known = resolvedImports[edges].get(file);
  if (known) {
    return known;
  }
  const found = specifiersOf(file, edges)
    .map((specifier) => resolveRelative(file, specifier))
    .filter((next): next is string => next !== null);
  resolvedImports[edges].set(file, found);
  return found;
}

/**
 * The shortest import path from `entry` (or the nearest of several) to any of
 * `targets`, as absolute paths from the entry on, or null when none is
 * reachable. A path rather than a boolean, because the offending edge is
 * usually several hops in and invisible from the entry.
 */
export function importPathTo(
  entry: string | readonly string[],
  targets: ReadonlySet<string> | ((file: string) => boolean),
  // Type-only edges count by default: a type import naming the heavy half
  // couples the two, and a later value import across it would pass review.
  edges: Edges = "all",
): string[] | null {
  const isTarget =
    typeof targets === "function"
      ? targets
      : (file: string) => targets.has(file);
  const starts = typeof entry === "string" ? [entry] : entry;
  const hit = starts.find(isTarget);
  if (hit !== undefined) {
    return [hit];
  }
  const seen = new Set(starts);
  const queue: string[][] = starts.map((start) => [start]);
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
  const loads =
    importText(file).includes("vitest") &&
    moduleSpecifiers(sourceOf(file), "values").some((specifier) =>
      TEST_RUNNER.test(specifier),
    );
  runnerModules.set(file, loads);
  return loads;
}

/** Each entry's value-import path to a module that loads the test runner. */
export function testRunnerReach(entries: readonly string[]): string[][] {
  // The scan costs a fraction of a parse, so only an entry it flags pays for
  // the exact walk, which may still clear it of a type-only path.
  return offenders(entries, "scanned")
    .map((entry) => importPathTo(entry, loadsTestRunner, "values"))
    .filter((path): path is string[] => path !== null);
}

// One walk per offender from every remaining entry, not one per entry: each
// clean entry's walk would cross the whole graph again.
function offenders(entries: readonly string[], edges: Edges): string[] {
  const found: string[] = [];
  let remaining = [...entries];
  let path = importPathTo(remaining, loadsTestRunner, edges);
  while (path !== null) {
    const offender = path[0];
    found.push(offender);
    remaining = remaining.filter((entry) => entry !== offender);
    path = importPathTo(remaining, loadsTestRunner, edges);
  }
  return entries.filter((entry) => found.includes(entry));
}
