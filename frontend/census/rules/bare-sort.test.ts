// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { rule } from "./bare-sort.census";

const planted = (text: string) => {
  const tree = treeOf([
    { path: join(srcDir, "lib", "probe.ts"), area: "src", text },
  ]);
  const [file] = tree.files;
  if (file === undefined || !("visit" in rule))
    throw new Error("no planted file");
  return rule.visit(file, tree);
};

describe("the bare-sort rule", () => {
  it("fails a sort or toSorted called with no comparator, optional chain included", () => {
    expect(
      planted(
        [
          "ids.sort();",
          "names?.sort();",
          "const sorted = values.toSorted();",
        ].join("\n"),
      ),
    ).toEqual([
      "src/lib/probe.ts:1: sorts with no comparator",
      "src/lib/probe.ts:2: sorts with no comparator",
      "src/lib/probe.ts:3: sorts with no comparator",
    ]);
  });

  it("spares a sort with a comparator, and a mention in a comment or a string", () => {
    expect(
      planted(
        [
          "ids.sort(stable);",
          "counts.sort((a, b) => a - b);",
          "// ids.sort() is what this replaced",
          'const advice = "never write xs.sort()";',
        ].join("\n"),
      ),
    ).toEqual([]);
  });

  it("reads the census's own files beside the walked tree", () => {
    const corpus = rule.corpus(treeOf([]));
    expect(
      corpus.some((file) => file.rel === "census/rules/bare-sort.census.ts"),
    ).toBe(true);
  });
});

describe("the sort prefilter", () => {
  it("reads a bare sort however it is spelled", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "plant", "a.ts"),
          area: "src",
          text: "rows.sort();\n",
        },
      ]),
    ).toHaveLength(1);
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "plant", "b.ts"),
          area: "src",
          text: "rows.\\u0073ort();\n",
        },
      ]),
    ).toHaveLength(1);
  });
});
