// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The census runner. It walks and parses the tree once. Then it runs every rule
// in census/rules/ as its own named case, so a red rule names itself.
//
// One test file is one module graph, so every rule here reads one parse.
// `make fe-census` runs it; `-t <rule>` runs one rule.

import { readdirSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { beforeAll, describe, expect, it } from "vitest";
import { byCodeUnit } from "../src/lib/codeunitorder";
import {
  buildTree,
  type CensusRule,
  type CensusTree,
  gitListedFiles,
  repoRoot,
  ruleFindings,
  settles,
} from "./tree";

const rulesDir = join(dirname(fileURLToPath(import.meta.url)), "rules");
const ruleFiles = readdirSync(rulesDir)
  .filter((name) => name.endsWith(".census.ts"))
  .sort(byCodeUnit);

function isRule(value: unknown): value is CensusRule {
  if (typeof value !== "object" || value === null) return false;
  return (
    "name" in value &&
    typeof value.name === "string" &&
    "corpus" in value &&
    typeof value.corpus === "function" &&
    "advice" in value &&
    typeof value.advice === "string" &&
    ("visit" in value || "across" in value || "settle" in value)
  );
}

const rules: CensusRule[] = [];
for (const name of ruleFiles) {
  const loaded: unknown = (await import(join(rulesDir, name))).rule;
  if (!isRule(loaded)) {
    throw new Error(
      `census/rules/${name} exports no \`rule\` with a name, a corpus, advice and a visit or across`,
    );
  }
  rules.push(loaded);
}

// A settled rule's work starts with the tree and is awaited after every other
// rule. So it runs beside them rather than inside one test's budget.
const ordered = [
  ...rules.filter((rule) => !settles(rule)),
  ...rules.filter(settles),
];

describe("census", () => {
  let tree: CensusTree;
  const pending = new Map<CensusRule, Promise<readonly string[]>>();
  beforeAll(() => {
    tree = buildTree();
    // The shared typed program, built here so no one rule pays for it.
    tree.checked().getTypeChecker();
    for (const rule of rules.filter(settles)) {
      const work = rule.settle(rule.corpus(tree), tree);
      // Awaited in the rule's own case; this keeps an early failure from
      // reading as an unhandled rejection before then.
      work.catch(() => undefined);
      pending.set(rule, work);
    }
  });

  // A walk that skips a folder reads a smaller tree, and every rule passes.
  // So the walk must equal what git knows under the same roots.
  it("reads every file git knows under its roots", () => {
    const walked = tree.files.map((file) => relative(repoRoot, file.path));
    expect(walked.sort(byCodeUnit)).toEqual(gitListedFiles().sort(byCodeUnit));
  });

  it("runs one rule per *.census.ts file, each under its own name", () => {
    expect(rules).toHaveLength(ruleFiles.length);
    expect(rules.map((rule) => `${rule.name}.census.ts`)).toEqual(ruleFiles);
  });

  for (const rule of ordered) {
    it(rule.name, async () => {
      const corpus = rule.corpus(tree);
      expect(corpus.length, `${rule.name} read no file`).toBeGreaterThan(0);
      expect(rule.floors?.(corpus, tree) ?? []).toEqual([]);
      const found = await (pending.get(rule) ??
        ruleFindings(rule, corpus, tree));
      expect(found, rule.advice).toEqual([]);
    });
  }
});
