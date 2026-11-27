// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A sum of minor amounts goes through addMinor or sumMinor (format/minorsum.ts).
// Each amount is bounded and a sum is not. Past 2^53 a plain `+` gives another
// number, and the screen prints it as the total.

// An operand names a minor amount when it ends in `_minor` or `Minor`, read
// through parentheses, `!`, `as` and the left side of `??`.

import ts from "typescript";
import type { CensusRule } from "../tree";

const MINOR_NAME = /(_minor|Minor)$/;
const HELPER = "src/format/minorsum.ts";

function namesMinor(expression: ts.Expression): boolean {
  let node: ts.Expression = expression;
  for (;;) {
    if (
      ts.isParenthesizedExpression(node) ||
      ts.isNonNullExpression(node) ||
      ts.isAsExpression(node)
    ) {
      node = node.expression;
    } else if (
      ts.isBinaryExpression(node) &&
      node.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken
    ) {
      node = node.left;
    } else {
      break;
    }
  }
  if (ts.isIdentifier(node)) return MINOR_NAME.test(node.text);
  if (ts.isPropertyAccessExpression(node))
    return MINOR_NAME.test(node.name.text);
  return false;
}

/** The line of every `+` or `+=` with an operand that names a minor amount. */
export function minorSums(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isBinaryExpression(node) &&
      (node.operatorToken.kind === ts.SyntaxKind.PlusToken ||
        node.operatorToken.kind === ts.SyntaxKind.PlusEqualsToken) &&
      (namesMinor(node.left) || namesMinor(node.right))
    ) {
      const at = node.operatorToken.getStart(source);
      lines.push(source.getLineAndCharacterOfPosition(at).line + 1);
    }
    node.forEachChild(visit);
  };
  visit(source);
  return lines;
}

export const rule: CensusRule = {
  name: "minor-sum",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.area === "src" &&
        file.source !== undefined &&
        file.rel !== HELPER &&
        !/\.(test|stories)\.tsx?$/.test(file.rel),
    ),
  visit: (file) =>
    file.source === undefined
      ? []
      : minorSums(file.source).map(
          (line) => `${file.rel}:${line}: adds minor amounts with a plain +`,
        ),
  advice:
    "A sum of minor amounts can pass 2^53 and print a wrong total. Add them " +
    "with addMinor or sumMinor (format/minorsum.ts) and show an undefined " +
    'total as too large (t("format.tooLargeExact")).',
};
