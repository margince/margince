// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { treeOf } from "../tree";
import { layerFindings, rule } from "./ext-imports.census";

// A layer with a package.json declaring what a unit may import. No unit in the
// tree need ship one, so the test makes its own.
const layer = mkdtempSync(join(tmpdir(), "ext-layer-"));
writeFileSync(join(layer, "package.json"), "{}\n");
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
  it("reports a planted escape into the core through its own layer", () => {
    const tree = treeOf([
      unit("planted.ts", 'import { x } from "../../../frontend/src/lib/x";\n'),
      unit("deep.ts", 'import { y } from "@margince/frontend/src/lib/y";\n'),
    ]);
    const { violations } = layerFindings(rule.corpus(tree), [layer]);
    expect(violations).toHaveLength(2);
    expect(violations[0]).toContain("leaves the unit's own frontend/");
    expect(violations[1]).toContain("is not a published subpath");
  });

  it("states no floor when no unit ships a screen", () => {
    const tree = treeOf([]);
    expect(rule.floors?.(rule.corpus(tree), tree)).toEqual([]);
  });
});
