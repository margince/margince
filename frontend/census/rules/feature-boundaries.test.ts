// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import type { ImportGraph } from "../../scripts/feature-boundaries";
import { plantedFindings, srcDir, treeOf } from "../tree";
import {
  boundariesRule,
  featureFolders,
  graphDrift,
} from "./feature-boundaries.census";

const planted = (...paths: string[]) =>
  treeOf(
    paths.map((path) => ({ path: join(srcDir, path), area: "src", text: "" })),
  ).files;

describe("the feature-boundaries census", () => {
  it("names each feature folder once", () => {
    expect(
      featureFolders(
        planted("features/deals/a.tsx", "features/deals/b/c.ts", "lib/x.ts"),
      ),
    ).toEqual(["features/deals"]);
  });

  it("reports a walked graph that differs from the tree", () => {
    const graph: ImportGraph = new Map([["features/leads/b.tsx", []]]);
    expect(graphDrift(planted("features/deals/a.tsx"), graph)).toEqual([
      "src/features/deals/a.tsx is in the tree and not in the import graph",
      "src/features/leads/b.tsx is in the import graph and not in the tree",
      "src/features/deals was not read",
    ]);
  });
});

describe("the feature-boundaries rule", () => {
  it("reports a planted feature importing another through its own across", () => {
    const graph: ImportGraph = new Map([
      ["features/deals/a.tsx", ["features/leads/b.tsx"]],
      ["features/leads/b.tsx", []],
    ]);
    const findings = plantedFindings(
      boundariesRule(() => graph),
      [
        { path: join(srcDir, "features/deals/a.tsx"), area: "src", text: "" },
        { path: join(srcDir, "features/leads/b.tsx"), area: "src", text: "" },
      ],
    );
    expect(findings).toContainEqual(
      expect.stringContaining(
        "src/features/deals/a.tsx imports src/features/leads/b.tsx",
      ),
    );
  });
});
