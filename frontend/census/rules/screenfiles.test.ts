// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { missingRoots, rule, screenCorpus } from "./screenfiles.census";

const planted = (...names: string[]) =>
  names.map((name) => ({
    path: join(srcDir, name),
    area: "src" as const,
    text: "",
  }));

describe("the screenfiles census", () => {
  it("leaves out tests, testkits, stories and files outside the screen folders", () => {
    const tree = treeOf(
      planted(
        "features/a.tsx",
        "features/a.test.tsx",
        "features/a.stories.tsx",
        "components/b.testkit.tsx",
        "features/c.ts",
        "lib/d.tsx",
      ),
    );
    expect(screenCorpus(tree).map((file) => file.rel)).toEqual([
      "src/features/a.tsx",
    ]);
  });
});

describe("the screenfiles rule", () => {
  it("reports a screen folder that is gone", () => {
    expect(missingRoots(["features", "planted-gone"])).toEqual([
      "src/planted-gone does not exist",
    ]);
  });

  it("states its floors over a planted tree that lost the screens", () => {
    const tree = treeOf(planted("features/a.tsx"));
    expect(rule.floors?.(rule.corpus(tree), tree)).toEqual([
      "500 screen files or fewer were read",
      "no contacts.tsx was read",
      "no file under deal360/ was read",
    ]);
    expect(plantedFindings(rule, planted("features/a.tsx"))).toEqual([]);
  });
});
