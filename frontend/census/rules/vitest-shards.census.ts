// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The cost model that splits the vitest shards reads each test file as a
// render, parse or program test. Over the tree's own test files it must find
// renders and parses; a model that read every file as one kind splits by
// chance. No fe-unit file builds a program: the typed censuses run in the
// census lane. So the planted cases alone prove the program kind.
//
// Every file the model reads as parse or program must also sit in the
// unisolated project that compilerTests lists for the vitest config.

import { type TestKind, testCosts } from "../../scripts/lib/test-cost";
import { compilerTests } from "../../scripts/vitest-shards";
import { type CensusFile, frontendRoot, type SettledRule } from "../tree";

const KINDS: readonly TestKind[] = ["render", "parse"];

type Costs = ReadonlyMap<string, Readonly<{ kind: TestKind }>>;

/** The kinds the cost model finds none of among these costs. */
export function missingKinds(costs: Costs): TestKind[] {
  const found = new Set([...costs.values()].map(({ kind }) => kind));
  return KINDS.filter((kind) => !found.has(kind));
}

/** The compiler-loading test files the unisolated project leaves out. */
export function outsideTheProject(
  corpus: readonly CensusFile[],
  costs: Costs,
  project: ReadonlySet<string>,
): string[] {
  return corpus
    .filter((file) => (costs.get(file.path)?.kind ?? "render") !== "render")
    .map((file) => file.rel)
    .filter((rel) => !project.has(rel));
}

export const rule: SettledRule = {
  name: "vitest-shards",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "scripts") &&
        /\.test\.tsx?$/.test(file.path),
    ),
  // compilerTests is async, so the rule settles after the synchronous ones.
  settle: async (corpus) => {
    const costs = testCosts(corpus.map((file) => file.path));
    const project = new Set(await compilerTests(frontendRoot));
    return [
      ...missingKinds(costs).map(
        (kind) => `the cost model reads no test file as a ${kind} test`,
      ),
      ...outsideTheProject(corpus, costs, project).map(
        (rel) =>
          `${rel} loads the compiler and is not in the unisolated project`,
      ),
    ];
  },
  advice:
    "The shard cost model in scripts/lib/test-cost.ts and compilerTests in " +
    "scripts/vitest-shards.ts must agree on which test files load the " +
    "compiler. Fix how they read a file's imports.",
};
