// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { mayHoldTable, rule, tablesIn } from "./currency-table-coverage.census";

describe("a currency", () => {
  const planted: ReadonlyArray<readonly [string, string, number]> = [
    [
      "a symbol table as a switch",
      'function s(c) { switch (c) { case "EUR": return "€"; case "USD": return "$"; case "GBP": return "£"; default: return c; } }',
      1,
    ],
    [
      "the same table as a record",
      'const SYMBOL = { EUR: "€", USD: "$", GBP: "£" };',
      1,
    ],
    [
      "a quoted-key record, which an identifier-only rule would miss",
      'const D = { "EUR": 2, "JPY": 0 };',
      1,
    ],
    [
      "a scale table, which is the same question asked the other way",
      "const DIGITS = { JPY: 0, VND: 0 };",
      1,
    ],
    [
      "an if-chain",
      'function s(c: string) { if (c === "EUR") return "€"; if (c === "USD") return "$"; return c; }',
      1,
    ],
    [
      "a table in a class method",
      'class F { symbol(c: string) { if (c === "EUR") return "€"; if (c === "USD") return "$"; return c; } }',
      1,
    ],
    [
      "a getter holding the same table",
      'class F { get digits() { return this.c === "JPY" ? 0 : this.c === "VND" ? 0 : 2; } }',
      1,
    ],
    [
      "the same chain as an arrow function",
      'const s = (c: string) => (c === "JPY" ? 0 : c === "VND" ? 0 : 2);',
      1,
    ],
    [
      "two unrelated one-code callbacks inside one function",
      'function outer() { a(() => { if (c === "JPY") return 0; }); b(() => { if (c === "VND") return 0; }); }',
      0,
    ],
    [
      "two unrelated one-code methods in one class",
      'class F { a(c: string) { if (c === "JPY") return 0; } b(c: string) { if (c === "VND") return 0; } }',
      0,
    ],
    [
      "one code compared, which is a decision about one currency",
      'function whole(c: string, a: number) { if (c === "JPY") { return a; } return a / 100; }',
      0,
    ],
    [
      "one code in an if, which is a decision about one currency rather than a table",
      'if (currency === "JPY") { return whole(amount); }',
      0,
    ],
    [
      "one case in a switch",
      'switch (c) { case "EUR": return a; default: return b; }',
      0,
    ],
    [
      "a switch on something that is not a currency",
      'switch (k) { case "GET": return a; case "PUT": return b; }',
      0,
    ],
    [
      "the adopted spelling",
      "return formatMoneyCompact(minor, currency, locale);",
      0,
    ],
  ];

  for (const [name, source, expected] of planted) {
    it(`sees ${name}`, () => {
      expect(tablesIn("planted.ts", source)).toHaveLength(expected);
      // The rule parses only what the prefilter admits.
      if (expected > 0) {
        expect(
          mayHoldTable(source),
          "the prefilter turns this table away",
        ).toBe(true);
      }
    });
  }
});

describe("the currency-table-coverage rule", () => {
  it("reports a screen's table and passes the owner's", () => {
    const table = "export const DIGITS = { JPY: 0, VND: 0 };\n";
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "x.ts"), area: "src", text: table },
        {
          path: join(srcDir, "format", "minorunits.ts"),
          area: "src",
          text: table,
        },
      ]),
    ).toEqual(["record src/features/x.ts:1"]);
  });
});
