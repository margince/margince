// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import type { CensusRule } from "../tree";
import { reachedInTree } from "./bundle-reach.census";

// A stored status drawn as JSX text, `{row.status}`, reaches every reader as
// the English wire code. Any receiver counts, since a deal is often `row`.
// Blind spot: a status first held in a local (`const s = row.status`) and drawn
// as `{s}` is not followed.
export function rawStatusRenders(source: ts.SourceFile): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isJsxExpression(node) &&
      !ts.isJsxAttribute(node.parent) &&
      node.expression !== undefined &&
      drawsStatus(node.expression)
    ) {
      found.push(node.getText(source));
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

// `key ? t(key) : …`: the branch translates the very key the condition tested.
function translatesItsCondition(expression: ts.ConditionalExpression): boolean {
  const branch = expression.whenTrue;
  return (
    ts.isCallExpression(branch) &&
    ts.isIdentifier(branch.expression) &&
    branch.expression.text === "t" &&
    branch.arguments[0]?.getText() === expression.condition.getText()
  );
}

// What a wrapper still draws: a cast, a `!`, `String(x)` or a template slot.
function inner(expression: ts.Expression): readonly ts.Expression[] {
  if (
    ts.isParenthesizedExpression(expression) ||
    ts.isAsExpression(expression) ||
    ts.isSatisfiesExpression(expression) ||
    ts.isNonNullExpression(expression) ||
    ts.isTypeAssertionExpression(expression)
  ) {
    return [expression.expression];
  }
  if (
    ts.isCallExpression(expression) &&
    ts.isIdentifier(expression.expression) &&
    expression.expression.text === "String"
  ) {
    return expression.arguments.slice(0, 1);
  }
  if (ts.isTemplateExpression(expression)) {
    return expression.templateSpans.map((span) => span.expression);
  }
  return [];
}

// The value a child expression draws: a fallback or a branch draws it too.
function drawsStatus(expression: ts.Expression): boolean {
  const wrapped = inner(expression);
  if (wrapped.length > 0) {
    return wrapped.some(drawsStatus);
  }
  if (ts.isConditionalExpression(expression)) {
    // `key ? t(key) : row.status` shows a code this build has no word for.
    if (translatesItsCondition(expression)) {
      return false;
    }
    return (
      drawsStatus(expression.whenTrue) || drawsStatus(expression.whenFalse)
    );
  }
  if (
    ts.isBinaryExpression(expression) &&
    [ts.SyntaxKind.QuestionQuestionToken, ts.SyntaxKind.BarBarToken].includes(
      expression.operatorToken.kind,
    )
  ) {
    return drawsStatus(expression.left) || drawsStatus(expression.right);
  }
  // `cond && row.status` draws only its right side.
  if (
    ts.isBinaryExpression(expression) &&
    expression.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken
  ) {
    return drawsStatus(expression.right);
  }
  // `row.status.replace(…)` and `.toUpperCase()` still draw the code.
  if (
    ts.isCallExpression(expression) &&
    ts.isPropertyAccessExpression(expression.expression)
  ) {
    return drawsStatus(expression.expression.expression);
  }
  return (
    ts.isPropertyAccessExpression(expression) &&
    expression.name.text === "status"
  );
}

export const rule: CensusRule = {
  name: "dealstatusbadge",
  corpus: (tree) => {
    const reached = reachedInTree(tree);
    return tree.files.filter(
      (file) => file.path.endsWith(".tsx") && reached.has(file.path),
    );
  },
  floors: (corpus) =>
    corpus.length > 500 ? [] : ["the bundle reaches 500 screens or fewer"],
  visit: (file) =>
    file.source
      ? rawStatusRenders(file.source).map((found) => `${file.rel}: ${found}`)
      : [],
  advice:
    "Draw a record's status through its badge (DealStatusBadge for a deal), " +
    "which names it in the reader's language, never as the stored code.",
};
