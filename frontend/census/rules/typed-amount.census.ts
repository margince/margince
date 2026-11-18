// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A typed amount is read by typedMinorUnits in format/minorunits.ts, and
// nowhere else. `toMinorUnits(Number(text))` reads "12,5" as NaN and "1.500"
// as one and a half, whatever the reader meant; thirteen screens did that.
//
// The subject is a toMinorUnits call whose amount is a coercion of text:
// Number, parseFloat, parseInt or a unary plus. The coercion is written in
// place or held by a const. One passed through a function is left out.

import ts from "typescript";
import { parseSource } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule } from "../tree";

const OWNER = "src/format/minorunits.ts";
const COERCIONS = new Set(["Number", "parseFloat", "parseInt"]);

function isCoercion(node: ts.Expression): boolean {
  if (
    ts.isPrefixUnaryExpression(node) &&
    node.operator === ts.SyntaxKind.PlusToken
  ) {
    return true;
  }
  if (!ts.isCallExpression(node)) {
    return false;
  }
  const callee = node.expression;
  const name = ts.isPropertyAccessExpression(callee)
    ? callee.name.text
    : ts.isIdentifier(callee)
      ? callee.text
      : "";
  return COERCIONS.has(name);
}

/** `<file>:<line>` for every toMinorUnits over coerced text in one file. */
function coercedScalings(source: ts.SourceFile, fileName: string): string[] {
  const coercedConsts = new Set<string>();
  const calls: ts.CallExpression[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer &&
      isCoercion(node.initializer)
    ) {
      coercedConsts.add(node.name.text);
    }
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "toMinorUnits"
    ) {
      calls.push(node);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return calls
    .filter((call) => {
      const amount = call.arguments[0];
      return (
        amount !== undefined &&
        (isCoercion(amount) ||
          (ts.isIdentifier(amount) && coercedConsts.has(amount.text)))
      );
    })
    .map(
      (call) =>
        `${fileName}:${source.getLineAndCharacterOfPosition(call.getStart()).line + 1}`,
    );
}

/** Every coerced scaling in planted text. */
export function coercedScalingsIn(fileName: string, text: string): string[] {
  return coercedScalings(parseSource(fileName, text), fileName);
}

const judged = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.path) &&
  !/\.(test|stories)\.tsx?$/.test(file.path) &&
  file.rel !== OWNER;

export const rule: CensusRule = {
  name: "typed-amount",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) =>
    corpus.length > 100 ? [] : ["fewer than 100 files were read"],
  visit: (file) =>
    file.source && file.text.includes("toMinorUnits(")
      ? coercedScalings(file.source, file.rel)
      : [],
  advice:
    "Read typed amount text with typedMinorUnits (format/minorunits.ts), " +
    "which reads either decimal sign and refuses an ambiguous 1.500.",
};
