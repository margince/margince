// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { srcDir, treeOf } from "../tree";
import { rule } from "./minor-sum.census";

const planted = (text: string) => {
  const tree = treeOf([
    { path: join(srcDir, "lib", "probe.ts"), area: "src", text },
  ]);
  const [file] = tree.files;
  if (file === undefined || !("visit" in rule))
    throw new Error("no planted file");
  return rule.visit(file, tree);
};

describe("the minor-sum rule", () => {
  it("fails a plain sum over a minor amount, however the operand is wrapped", () => {
    expect(
      planted(
        [
          "const a = row.won_minor + row.lost_minor;",
          "const b = items.reduce((sum, x) => sum + (x.amount_minor ?? 0), 0);",
          "total += rawMinor;",
          "const c = base + (entry.amountMinor as number);",
        ].join("\n"),
      ),
    ).toEqual([
      "src/lib/probe.ts:1: adds minor amounts with a plain +",
      "src/lib/probe.ts:2: adds minor amounts with a plain +",
      "src/lib/probe.ts:3: adds minor amounts with a plain +",
      "src/lib/probe.ts:4: adds minor amounts with a plain +",
    ]);
  });

  it("spares the helpers, a difference, a count and a mention in text", () => {
    expect(
      planted(
        [
          "const a = addMinor(row.won_minor, row.lost_minor);",
          "const gap = call.amount_minor - evidence;",
          "const n = row.count + 1;",
          "// row.won_minor + row.lost_minor is what this replaced",
          'const label = "won_minor + lost_minor";',
        ].join("\n"),
      ),
    ).toEqual([]);
  });
});
