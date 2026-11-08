// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Vitest's own `--shard` splits by a hash of the path, which evens out the file
// count and leaves the time to chance. The few files that type-check the whole
// tree cost minutes each, so this splits by expected cost instead.

import { globSync } from "node:fs";
import { join } from "node:path";
import type { TestSequencer, TestSpecification, Vitest } from "vitest/node";

/**
 * Splits `costs` into `count` slices, heaviest first, each onto the slice
 * with the least cost so far. Ties fall to the path and the lower slice, so
 * every process given the same files computes the same split.
 */
export function partition(
  costs: ReadonlyMap<string, number>,
  count: number,
): string[][] {
  if (!Number.isInteger(count) || count < 1) {
    throw new Error(
      `partition: slice count must be a positive integer, got ${count}`,
    );
  }
  const slices = Array.from({ length: count }, () => ({
    load: 0,
    files: [] as string[],
  }));
  const heaviestFirst = [...costs].sort(
    ([pathA, costA], [pathB, costB]) =>
      costB - costA || (pathA < pathB ? -1 : pathA > pathB ? 1 : 0),
  );
  for (const [file, cost] of heaviestFirst) {
    const lightest = slices.reduce((best, slice) =>
      slice.load < best.load ? slice : best,
    );
    lightest.load += cost;
    lightest.files.push(file);
  }
  return slices.map((slice) => slice.files);
}

// The cost model loads the TypeScript compiler, so it is imported only once a
// test run asks for it. A `vite build` reading this config never pays for it.
async function costsOf(
  files: readonly TestSpecification[],
): Promise<Map<string, number>> {
  const { testCosts } = await import("./lib/test-cost.ts");
  const paths = [...new Set(files.map((spec) => spec.moduleId))];
  return new Map([...testCosts(paths)].map(([path, { cost }]) => [path, cost]));
}

// Vitest's default include. A file this misses stays in the isolated project,
// so a gap here costs time and never drops a file.
const TEST_FILES = "**/*.{test,spec}.{ts,tsx,mts,cts,js,jsx,mjs,cjs}";

/**
 * The test files under `frontendRoot` that load the TypeScript compiler, as
 * paths relative to it: the parse and program kinds of the cost model.
 */
export async function compilerTests(frontendRoot: string): Promise<string[]> {
  const { testCosts } = await import("./lib/test-cost.ts");
  const files = globSync(TEST_FILES, {
    cwd: frontendRoot,
    exclude: (path) =>
      path === "node_modules" || path.endsWith("/node_modules"),
  });
  const costs = testCosts(files.map((file) => join(frontendRoot, file)));
  return files
    .filter((file) => costs.get(join(frontendRoot, file))?.kind !== "render")
    .sort();
}

export class CostSequencer implements TestSequencer {
  constructor(private readonly ctx: Vitest) {}

  async shard(files: TestSpecification[]): Promise<TestSpecification[]> {
    const { shard } = this.ctx.config;
    if (!shard) return files;
    const slices = partition(await costsOf(files), shard.count);
    const mine = new Set(slices[shard.index - 1]);
    return files.filter((spec) => mine.has(spec.moduleId));
  }

  // Heaviest first, so a file that takes minutes starts while the other
  // workers still have short files to run beside it.
  async sort(files: TestSpecification[]): Promise<TestSpecification[]> {
    const costs = await costsOf(files);
    return [...files].sort(
      (a, b) => (costs.get(b.moduleId) ?? 0) - (costs.get(a.moduleId) ?? 0),
    );
  }
}
