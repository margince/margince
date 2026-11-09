// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { extensionPackageLayers } from "../../scripts/lib/source-tree";
import { extensionsDir, plantedFindings, treeOf } from "../tree";
import { layerFindings, rule } from "./ext-imports.census";

// A real layer, so its package.json declares what a unit may import.
const [layer] = extensionPackageLayers(extensionsDir);
const unit = (name: string, text: string) => ({
  path: join(layer, name),
  area: "extensions" as const,
  text,
});

describe("the ext-imports census", () => {
  it("counts the files it read in each layer", () => {
    const tree = treeOf([unit("planted.ts", "export const a = 1;\n")]);
    const { files, violations } = layerFindings(rule.corpus(tree), [layer]);
    expect(files.get(layer)).toBe(1);
    expect(violations).toEqual([]);
  });

  it("leaves a file outside every layer unread", () => {
    const tree = treeOf([unit("planted.ts", "export const a = 1;\n")]);
    const { files } = layerFindings(rule.corpus(tree), [join(layer, "other")]);
    expect(files.size).toBe(0);
  });
});

describe("the ext-imports rule", () => {
  it("reports a planted escape into the core through its own across", () => {
    const findings = plantedFindings(rule, [
      unit("planted.ts", 'import { x } from "../../../frontend/src/lib/x";\n'),
      unit("deep.ts", 'import { y } from "@margince/frontend/src/lib/y";\n'),
    ]);
    expect(findings).toHaveLength(2);
    expect(findings[0]).toContain("leaves the unit's own frontend/");
    expect(findings[1]).toContain("is not a published subpath");
  });

  it("states a floor for a layer it read nothing in", () => {
    const tree = treeOf([]);
    expect(rule.floors?.(rule.corpus(tree), tree)).toContainEqual(
      expect.stringContaining("was found and no file in it was read"),
    );
  });
});
