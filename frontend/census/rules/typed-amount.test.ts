// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { coercedScalingsIn, rule } from "./typed-amount.census";

describe("a typed amount scaled by coercion", () => {
  const planted: ReadonlyArray<readonly [string, string, number]> = [
    ["Number in place", "toMinorUnits(Number(amount), currency);", 1],
    ["parseFloat in place", "toMinorUnits(parseFloat(amount), c);", 1],
    ["Number.parseFloat", "toMinorUnits(Number.parseFloat(a), c);", 1],
    ["a unary plus", "toMinorUnits(+amount, currency);", 1],
    [
      "a const that holds the coercion",
      "const parsed = Number(input.amount); f(toMinorUnits(parsed, c));",
      1,
    ],
    ["a number already", "toMinorUnits(value, currency);", 0],
    [
      "a comma stripped beside the scale",
      'const d = minorUnitDigits(c); const s = text.replace(/[\\s,]/g, "");',
      1,
    ],
    [
      "a comma swapped beside the scale",
      'toMajorUnits(m, c); const s = text.replace(",", ".");',
      1,
    ],
    ["a comma replaced away from money", 'text.replace(",", ".");', 0],
    ["the adopted reader", "typedMinorUnits(text, currency, locale);", 0],
  ];
  for (const [name, source, expected] of planted) {
    it(`sees ${name}`, () => {
      expect(coercedScalingsIn("planted.ts", source)).toHaveLength(expected);
    });
  }
});

describe("the typed-amount rule", () => {
  it("reports a screen's coercion and passes the owner's", () => {
    const text = "toMinorUnits(Number(amount), currency);\n";
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "x.ts"), area: "src", text },
        { path: join(srcDir, "format", "minorunits.ts"), area: "src", text },
        { path: join(srcDir, "features", "x.test.ts"), area: "src", text },
      ]),
    ).toEqual(["src/features/x.ts:1"]);
  });
});
