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
// THREE SHAPES, because a table can be written three ways and the third is the
// one somebody reaches for after the first two are gated: a `switch`, an object
// literal, and an if/else chain of `=== "EUR"` comparisons. The third arm is
// here because it is the most natural way to rewrite the `symbolFor` this
// change deleted, and a gate blind to it would have said PASS over the same
// table in a different coat.
//
// WHAT THIS DOES NOT CATCH, deliberately: a table built from an ARRAY of codes,
// and a vocabulary of OFFERED currencies. `deals.tsx` and `companyactions.tsx`
// each spell `["EUR", "USD", "GBP", "CHF"]` for the currency picker — that is a
// list of what this deployment sells in, a product decision, not a claim about
// what a currency IS, and folding it in here would gate a preference with a
// rule about facts. (It is a genuine third copy of one list and deserves its
// own answer; it is not this one.) Nor does this follow a computed key, which
// would need the type checker.
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

// ONE predicate for "this node owns a scope", used as the scan ROOT and as the
// traversal BOUNDARY. Two lists would drift, and the drift is silent in a
// specific direction: a kind that is a boundary but not a root skips a table,
// and a kind that is a root but not a boundary invents one out of two unrelated
// decisions. Methods, accessors and constructors are function scopes too — a
// class holding a currency table in a method was neither.
type FunctionScope =
  | ts.FunctionDeclaration
  | ts.ArrowFunction
  | ts.FunctionExpression
  | ts.MethodDeclaration
  | ts.ConstructorDeclaration
  | ts.GetAccessorDeclaration
  | ts.SetAccessorDeclaration;

function isFunctionScope(node: ts.Node): node is FunctionScope {
  return (
    ts.isFunctionDeclaration(node) ||
    ts.isArrowFunction(node) ||
    ts.isFunctionExpression(node) ||
    ts.isMethodDeclaration(node) ||
    ts.isConstructorDeclaration(node) ||
    ts.isGetAccessorDeclaration(node) ||
    ts.isSetAccessorDeclaration(node)
  );
}

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
    // The if/else chain. Counted per FUNCTION rather than per statement,
    // because each `if` is its own node and a per-statement count would see
    // three separate one-code decisions where a reader sees one table.
    if (isFunctionScope(node)) {
      const compared = new Set<string>();
      const walk = (inner: ts.Node) => {
        // STOP at a nested function. Each is visited in its own right by the
        // outer traversal, so descending would attribute a child's comparison
        // to every ancestor — and an outer function holding two unrelated
        // single-code callbacks would be reported as a table it is not.
        if (inner !== node && isFunctionScope(inner)) {
          return;
        }
        if (
          ts.isBinaryExpression(inner) &&
          (inner.operatorToken.kind === ts.SyntaxKind.EqualsEqualsEqualsToken ||
            inner.operatorToken.kind === ts.SyntaxKind.EqualsEqualsToken)
        ) {
          for (const side of [inner.left, inner.right]) {
            if (ts.isStringLiteral(side) && CURRENCY_CODES.has(side.text)) {
              compared.add(side.text);
            }
          }
        }
        ts.forEachChild(inner, walk);
      };
      if (node.body !== undefined) {
        walk(node.body);
      }
      if (compared.size >= 2) {
        found.push(`compare ${at(node)}`);
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

// The files that OWN a per-currency table, named one by one rather than by
// directory. `format/` as a PREFIX was the first draft and it was too wide: a
// second symbol table added beside `minorunits.ts` would have passed the census
// this test exists to keep at zero — the same defect in the gate that the gate
// is about in the code.
//
// `minorunits.ts` is the minor-unit scale, held in step with its Go half by
// `backend/frontendminorunits_test.go`. `format.ts` holds no table at all and
// is deliberately NOT listed — it asks Intl. The test files are listed because
// a suite about currencies has to name them to assert about them.
const CURRENCY_TABLE_OWNERS = [
  "format/minorunits.ts",
  "format/minorunits.test.ts",
  "format/format.test.ts",
  "format/currency-table-coverage.test.ts",
  // Generated from the contract, which names currencies because the CONTRACT
  // does.
  "api/schema.d.ts",
];

function tables(): string[] {
  const files = sourceFiles(srcRoot)
    .map((file) => [relative(srcRoot, file), file] as const)
    .filter(([name]) => !CURRENCY_TABLE_OWNERS.includes(name));
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
    [
      // Found by a reviewer probing the first draft: the same table with no
      // switch and no object literal, which is exactly how the deleted
      // symbolFor would most naturally be rewritten.
      "an if-chain, which is the table wearing no detectable shape",
      'function s(c: string) { if (c === "EUR") return "€"; if (c === "USD") return "$"; return c; }',
      1,
    ],
    [
      "a table in a class METHOD, which was neither a root nor a boundary",
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
    // What must stay invisible.
    [
      // Found by a bot probing the if-chain arm: descending into nested
      // functions attributed a child's comparison to every ancestor, so a
      // function merely CONTAINING two unrelated one-code callbacks was
      // reported as a table.
      "two unrelated one-code callbacks inside one function",
      'function outer() { a(() => { if (c === "JPY") return 0; }); b(() => { if (c === "VND") return 0; }); }',
      0,
    ],
    [
      "two unrelated one-code METHODS in one class",
      'class F { a(c: string) { if (c === "JPY") return 0; } b(c: string) { if (c === "VND") return 0; } }',
      0,
    ],
    [
      "ONE code compared, which is a decision about one currency",
      'function whole(c: string, a: number) { if (c === "JPY") { return a; } return a / 100; }',
      0,
    ],
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
