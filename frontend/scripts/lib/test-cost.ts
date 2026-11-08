// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a test file is expected to cost, read from its source so that no list
// of timings has to be kept. Three kinds of file differ widely. One renders a
// screen, one loads the TypeScript compiler to parse the tree, and one also
// type-checks a whole program over it.

import { readFileSync } from "node:fs";
import { edgesOf } from "./import-reach.ts";

export type TestKind = "render" | "parse" | "program";

// Milliseconds on the CI runner: a fixed part for loading what the kind loads,
// and a part per kilobyte of test source. Fitted to six runs of the full suite.
// The parse kind was fitted again once those files ran unisolated, from two
// runs scaled to the render fit. A stale fit makes the split less even and
// never drops or repeats a file.
const COST_MS: Readonly<
  Record<TestKind, { fixed: number; perKilobyte: number }>
> = {
  render: { fixed: 700, perKilobyte: 110 },
  parse: { fixed: 1950, perKilobyte: 570 },
  program: { fixed: 0, perKilobyte: 2300 },
};

// A value import: `import type` is erased and loads nothing.
const LOADS_COMPILER =
  /^\s*import\s+(?!type\b)[^;]*?\bfrom\s*["']typescript["']/m;
const BUILDS_PROGRAM = /\b(?:programOver|checkedProgram|createProgram)\s*\(/;
const isSweep = (path: string) => path.endsWith(".sweep.ts");

// The modules each module is imported by, over everything `entries` reach,
// and which of them import the compiler themselves.
function importGraph(entries: readonly string[]) {
  const importers = new Map<string, string[]>();
  const loaders: string[] = [];
  const seen = new Set(entries);
  const queue = [...entries];
  for (let file = queue.pop(); file !== undefined; file = queue.pop()) {
    if (LOADS_COMPILER.test(readFileSync(file, "utf8"))) loaders.push(file);
    for (const next of edgesOf(file, "scanned")) {
      const known = importers.get(next);
      if (known) known.push(file);
      else importers.set(next, [file]);
      if (!seen.has(next)) {
        seen.add(next);
        queue.push(next);
      }
    }
  }
  return { importers, loaders };
}

// Walked backward from the modules that import the compiler, so a cycle in
// the forward walk cannot hide a path to it.
function compilerReach(entries: readonly string[]): Set<string> {
  const { importers, loaders } = importGraph(entries);
  const reach = new Set(loaders);
  for (let file = loaders.pop(); file !== undefined; file = loaders.pop()) {
    for (const importer of importers.get(file) ?? []) {
      if (!reach.has(importer)) {
        reach.add(importer);
        loaders.push(importer);
      }
    }
  }
  return reach;
}

/** Each test file's kind and its expected cost, keyed by absolute path. */
export function testCosts(
  files: readonly string[],
): Map<string, { kind: TestKind; cost: number }> {
  const reach = compilerReach(files);
  return new Map(
    files.map((file) => {
      // A test's `.sweep.ts` is its own code, moved out to run in a worker.
      const source = [file, ...edgesOf(file, "scanned").filter(isSweep)]
        .map((path) => readFileSync(path, "utf8"))
        .join("\n");
      const kind: TestKind = !reach.has(file)
        ? "render"
        : BUILDS_PROGRAM.test(source)
          ? "program"
          : "parse";
      const { fixed, perKilobyte } = COST_MS[kind];
      return [
        file,
        { kind, cost: fixed + (perKilobyte * source.length) / 1000 },
      ];
    }),
  );
}
