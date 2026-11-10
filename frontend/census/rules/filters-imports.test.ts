// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { importTrail } from "../imports";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { rule } from "./filters-imports.census";

const at = (rel: string, text = "") => ({
  path: join(srcDir, rel),
  area: "src" as const,
  text,
});

describe("the filters-imports rule", () => {
  it("names the path from the shell to a filter page, through a type import", () => {
    const findings = plantedFindings(rule, [
      at("app/shell.tsx", 'import "../lib/helper";'),
      at(
        "lib/helper.ts",
        'import type { X } from "../features/lists/filters";',
      ),
      at("features/lists/filters.tsx"),
      at("features/lists/listpage.tsx"),
    ]);
    expect(findings).toEqual([
      "reaches a filter page: src/app/shell.tsx -> src/lib/helper.ts -> src/features/lists/filters.tsx",
      "reaches a filter page: src/lib/helper.ts -> src/features/lists/filters.tsx",
      "src/features/ai/aicalls-filters.tsx: listed in ELSEWHERE but no such page names a noun",
      "src/features/analytics/reporting.filters.tsx: listed in ELSEWHERE but no such page names a noun",
    ]);
  });

  it("fails a page that names a noun past the start of its name", () => {
    const findings = plantedFindings(rule, [
      at("features/lists/list-filter.tsx"),
      at("features/ai/aicalls-filters.tsx"),
      at("features/analytics/reporting.filters.tsx"),
    ]);
    expect(findings).toEqual([
      "src/features/lists/list-filter.tsx: names a filter noun but is not guarded",
    ]);
  });

  it("finds the shortest trail and none where no edge leads", () => {
    const tree = treeOf([
      at("a.ts", 'import "./b"; import "./c";'),
      at("b.ts", 'import "./c";'),
      at("c.ts"),
      at("d.ts"),
    ]);
    const c = new Set([join(srcDir, "c.ts")]);
    expect(importTrail(tree, [join(srcDir, "a.ts")], c)).toEqual([
      "src/a.ts",
      "src/c.ts",
    ]);
    expect(importTrail(tree, [join(srcDir, "d.ts")], c)).toBeNull();
  });
});
