// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir, treeOf } from "../tree";
import { rule } from "./surface-stays-off-the-screen-registry.census";

const surface = join(srcDir, "surface", "index.ts");
const seam = join(srcDir, "lib", "extensionscreen.ts");
const registry = join(srcDir, "composition", "extscreens.gen.ts");

const plantedTree = (
  seamText: string,
): { path: string; area: Area; text: string }[] => [
  {
    path: surface,
    area: "src",
    text: `export * from "../lib/extensionscreen";\n`,
  },
  { path: seam, area: "src", text: seamText },
  {
    path: registry,
    area: "src",
    text: "export const screens = [];\n",
  },
];

describe("the surface-stays-off-the-screen-registry rule", () => {
  it("reports a surface whose imports reach the screen registry", () => {
    const findings = plantedFindings(
      rule,
      plantedTree(`export * from "../composition/extscreens.gen";\n`),
    );
    expect(findings).toEqual([
      "src/surface/index.ts loads src/composition/extscreens.gen.ts",
    ]);
  });

  it("passes a surface that stops at the extension seam", () => {
    const findings = plantedFindings(
      rule,
      plantedTree("export const seam = 1;\n"),
    );
    expect(findings).toEqual([]);
  });

  it("states a floor when the surface does not reach the unit route spelling", () => {
    const tree = treeOf([
      { path: surface, area: "src", text: "export const alone = 1;\n" },
    ]);
    expect(rule.floors?.(rule.corpus(tree), tree)).toEqual([
      "the surface does not reach src/lib/extensionscreen.ts",
    ]);
  });
});
