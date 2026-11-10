// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A screen that keeps its own map of currency codes answers a settled question
// a second time. `format/minorunits.ts` owns how many minor units a currency
// has, and `format/format.ts` asks `Intl.NumberFormat` how it is written.
//
// A three-entry symbol table is wrong about the fourth currency, and tells a
// German reader "k" where their conventions abbreviate at the million.
//
// The subject is derived: a switch, an object literal or an if/else chain over
// two or more ISO-4217 codes, anywhere under src/. One code is a decision about
// one currency; two is a table.
//
// Left out of scope: a table built from an array of codes, a vocabulary of
// offered currencies (`format/currencychoices.ts`), and a computed key.

import ts from "typescript";
import { parseSource } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule } from "../tree";

// A sample of ISO-4217 wide enough that a plausible table cannot avoid it.
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

// One predicate for a function scope, used as the scan root and as the
// traversal boundary. Two lists would drift and skip or invent a table.
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

/** `<kind> <file>:<line>` for every per-currency table in one parsed file. */
function tablesOf(source: ts.SourceFile, fileName: string): string[] {
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
    // The if/else chain is counted per function: each `if` is its own node,
    // and a reader sees one table.
    if (isFunctionScope(node)) {
      const compared = new Set<string>();
      const walk = (inner: ts.Node) => {
        // A nested function is visited in its own right. Descending would
        // credit a child's comparison to every ancestor.
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

/** Every per-currency table in planted text. */
export function tablesIn(fileName: string, text: string): string[] {
  return tablesOf(parseSource(fileName, text), fileName);
}

// The files that own a per-currency table, named one by one: a second table
// beside `minorunits.ts` must still fail. The tests are listed because a suite
// about currencies names them, and schema.d.ts because the contract does.
const CURRENCY_TABLE_OWNERS: ReadonlySet<string> = new Set([
  "src/format/minorunits.ts",
  "src/format/minorunits.test.ts",
  "src/format/format.test.ts",
  "src/api/schema.d.ts",
]);

// Whether a file is worth parsing. A table needs two codes, and an escape can
// spell a code the characters do not show.
export function mayHoldTable(text: string): boolean {
  if (text.includes("\\u") || text.includes("\\x")) {
    return true;
  }
  let seen = 0;
  for (const code of CURRENCY_CODES) {
    if (text.includes(code) && ++seen === 2) {
      return true;
    }
  }
  return false;
}

const judged = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.path) &&
  !CURRENCY_TABLE_OWNERS.has(file.rel);

export const rule: CensusRule = {
  name: "currency-table-coverage",
  corpus: (tree) => tree.files.filter(judged),
  // A prefilter that admits nothing reads the same as a clean tree.
  floors: (corpus) =>
    corpus.some((file) => mayHoldTable(file.text))
      ? []
      : ["the prefilter admitted no file to parse"],
  visit: (file) =>
    file.source && mayHoldTable(file.text)
      ? tablesOf(file.source, file.rel)
      : [],
  advice:
    "A currency is described in one place. Ask format/minorunits.ts for its " +
    "minor units and format/format.ts (Intl) for how it is written, instead " +
    "of keeping a table of codes.",
};
