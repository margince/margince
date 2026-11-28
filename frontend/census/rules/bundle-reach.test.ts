// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  aliases,
  SRC_ROOT,
  surfaceEntries,
} from "../../scripts/lib/bundle-reach";
import { moduleAt } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { reachedInTree, rule } from "./bundle-reach.census";

describe("the bundle-reach census", () => {
  it("follows an entry's imports through the planted tree", () => {
    const [entry] = surfaceEntries();
    expect(entry).toBeDefined();
    const planted = join(srcDir, "surface", "planted.ts");
    const stray = join(srcDir, "surface", "stray.ts");
    const tree = treeOf([
      { path: entry, area: "src", text: `export * from "./planted";\n` },
      { path: planted, area: "src", text: "export const planted = 1;\n" },
      { path: stray, area: "src", text: "export const stray = 1;\n" },
    ]);
    const reached = reachedInTree(tree);
    expect(reached.has(planted)).toBe(true);
    expect(reached.has(stray)).toBe(false);
  });
});

describe("the bundle aliases", () => {
  it("map every specifier onto a module under src", () => {
    const mapped = [...aliases().values()];
    expect(mapped.length).toBeGreaterThan(0);
    for (const target of mapped) {
      // A type-only alias target is a declaration file no bundler loads.
      const file =
        moduleAt(target) ?? (existsSync(`${target}.d.ts`) ? target : null);
      expect(file?.startsWith(`${SRC_ROOT}/`)).toBe(true);
    }
  });
});

describe("the bundle-reach rule", () => {
  it("reports every entry a planted tree does not hold", () => {
    const findings = plantedFindings(rule, [
      { path: join(srcDir, "planted.ts"), area: "src", text: "" },
    ]);
    expect(findings).toContain(
      `${surfaceEntries()[0]} is a bundle entry the tree does not hold`,
    );
  });

  it("states a floor when the bundle reaches almost nothing", () => {
    const tree = treeOf([
      { path: join(srcDir, "planted.ts"), area: "src", text: "" },
    ]);
    expect(rule.floors?.(rule.corpus(tree), tree)).toContain(
      "the bundle reaches 200 modules or fewer",
    );
  });
});
