// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir, treeOf } from "../tree";
import { featureFolders, rule } from "./feature-boundaries.census";

const planted = (...paths: string[]) =>
  treeOf(
    paths.map((path) => ({ path: join(srcDir, path), area: "src", text: "" })),
  ).files;

const plant = (files: Record<string, string>) =>
  Object.entries(files).map(([path, text]) => ({
    path: join(srcDir, path),
    area: "src" as Area,
    text,
  }));

describe("the feature-boundaries census", () => {
  it("names each feature folder once", () => {
    expect(
      featureFolders(
        planted("features/deals/a.tsx", "features/deals/b/c.ts", "lib/x.ts"),
      ),
    ).toEqual(["features/deals"]);
  });

  it("fails an import the tree cannot resolve and the disk can", () => {
    const tree = treeOf(
      plant({
        "features/deals/a.tsx":
          'import { LeadBand } from "../leads/leadband";\n',
      }),
    );
    expect(rule.floors?.(rule.corpus(tree), tree)).toContain(
      "src/features/deals/a.tsx imports src/features/leads/leadband, which is on disk and not in the tree",
    );
  });

  it("fails a stylesheet the tree misses and the disk holds", () => {
    const tree = treeOf(
      plant({ "features/deals/a.tsx": 'import "../../app.css";\n' }),
    );
    expect(rule.floors?.(rule.corpus(tree), tree)).toContain(
      "src/features/deals/a.tsx imports src/app.css, which is on disk and not in the tree",
    );
  });
});

describe("the feature-boundaries rule", () => {
  it("reports a planted feature importing another through its own across", () => {
    const findings = plantedFindings(
      rule,
      plant({
        "features/deals/a.tsx": 'import { b } from "../leads/b";\n',
        "features/leads/b.tsx": "export const b = 1;\n",
      }),
    );
    expect(findings).toContainEqual(
      expect.stringContaining(
        "src/features/deals/a.tsx imports src/features/leads/b.tsx",
      ),
    );
  });

  it("reads a mocked path however vi is spelled", () => {
    for (const mock of ["vi.mock", "v\\u0069.mock"]) {
      const findings = plantedFindings(
        rule,
        plant({
          "features/deals/a.test.ts": `${mock}("../leads/b");\n`,
          "features/leads/b.ts": "export const b = 1;\n",
        }),
      );
      expect(findings, mock).toContainEqual(
        expect.stringContaining(
          "src/features/deals/a.test.ts imports src/features/leads/b.ts",
        ),
      );
    }
  });
});
