// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// Fitness function for a screen that decides for itself what a currency is.
//
// Two things about a currency are facts, not choices: how many minor units it
// has, and how it is written. `format/minorunits.ts` owns the first (and a Go
// gate holds the two halves in step); `Intl.NumberFormat` owns the second, and
// `format/format.ts` is where this application asks it. A screen that carries
// its own map of codes is answering one of those questions a second time.
//
// It is not a tidiness argument. `personstrip.tsx` carried
// `{EUR:"€", USD:"$", GBP:"£", default: CODE+" "}` beside a hard-coded /1000
// tier, so every reader was told "k" — including the German one, whose
// conventions abbreviate at the million and put the symbol after the figure.
// A three-entry table is also a table that is wrong about the fourth currency
// the first time somebody sells in one. And the same shape carried the
// hundredfold scale error that #2317 had to sweep out of four places.
//
// The subject is DERIVED: any switch on, or object literal keyed by, two or
// more ISO-4217 codes, found by parsing everything under src/ outside
// `format/`. Two is the threshold because one code in a switch is a special
// case somebody made a decision about — a currency this product treats
// differently — while two is a table.
//
// WHAT THIS DOES NOT CATCH, deliberately: a table built from an array of
// codes, or one whose keys are computed. Nothing here writes one, and a rule
// that had to follow a computed key would need the type checker.
//
// `mcp-apps/bridge.ts` needs no exemption and is worth saying so: it formats
// money for a document served to a third-party host and CANNOT import the
// formatters, but it imports `minorUnitDigits` and asks `Intl` for the rest —
// the sanctioned way to be a second caller without being a second answer.

const formatDir = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(formatDir, "..");

// A sample of ISO-4217 wide enough to catch a real table without matching every
// three-letter string. It does not need to be exhaustive: a table of currencies
// this product would plausibly carry cannot avoid all of these, and the gate's
// job is to catch the table, not to validate the codes in it.
const CURRENCY_CODES = new Set([
  "EUR",
  "USD",
  "GBP",
  "JPY",
  "VND",
  "CHF",
  "SEK",
  "NOK",
  "DKK",
  "PLN",
  "CZK",
  "HUF",
  "RON",
  "BGN",
  "TRY",
  "CAD",
  "AUD",
  "NZD",
  "CNY",
  "INR",
  "KRW",
  "SGD",
  "HKD",
  "BRL",
  "MXN",
  "ZAR",
  "AED",
  "SAR",
  "ILS",
  "THB",
  "IDR",
  "MYR",
  "PHP",
  "ISK",
  "CLP",
  "COP",
  "KWD",
  "BHD",
  "OMR",
  "JOD",
  "TND",
  "IQD",
  "IRR",
  "MGA",
]);

/** `<kind> <file>:<line>` for every per-currency table in one file. */
function tablesIn(fileName: string, text: string): string[] {
  const source = ts.createSourceFile(
    fileName,
    text,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const found: string[] = [];
  const at = (node: ts.Node) =>
    `${fileName}:${source.getLineAndCharacterOfPosition(node.getStart()).line + 1}`;
  const visit = (node: ts.Node) => {
    if (ts.isSwitchStatement(node)) {
      const codes = node.caseBlock.clauses.filter(
        (clause) =>
          ts.isCaseClause(clause) &&
          ts.isStringLiteral(clause.expression) &&
          CURRENCY_CODES.has(clause.expression.text),
      );
      if (codes.length >= 2) {
        found.push(`switch ${at(node)}`);
      }
    }
    if (ts.isObjectLiteralExpression(node)) {
      const codes = node.properties.filter((property) => {
        const name = property.name;
        return (
          name !== undefined &&
          (ts.isStringLiteral(name) || ts.isIdentifier(name)) &&
          CURRENCY_CODES.has(name.text)
        );
      });
      if (codes.length >= 2) {
        found.push(`record ${at(node)}`);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    return entry.isDirectory()
      ? sourceFiles(path)
      : /\.tsx?$/.test(entry.name)
        ? [path]
        : [];
  });
}

function tables(): string[] {
  const files = sourceFiles(srcRoot)
    .map((file) => [relative(srcRoot, file), file] as const)
    // `format/` is where the answers live, and the generated contract types
    // name currencies because the CONTRACT does.
    .filter(
      ([name]) => !name.startsWith("format/") && name !== "api/schema.d.ts",
    );
  // A sweep that found no files and a tree with no tables look identical.
  expect(files.length).toBeGreaterThan(0);
  return files
    .flatMap(([name, file]) => tablesIn(name, readFileSync(file, "utf8")))
    .sort();
}

const PARSE_MS = 60_000;

describe("a currency", () => {
  it(
    "is described in exactly one place",
    () => {
      const found = tables();
      expect(found, `\n${found.join("\n")}\n`).toEqual([]);
    },
    PARSE_MS,
  );

  const planted: ReadonlyArray<readonly [string, string, number]> = [
    [
      "the symbol table personstrip really carried",
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
    // What must stay invisible.
    [
      "ONE code, which is a decision about one currency rather than a table",
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
    });
  }
});
