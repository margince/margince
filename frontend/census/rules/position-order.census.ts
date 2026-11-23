// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Rows that carry a `position` are put in that order by `inPositionOrder` alone,
// so a tie-break added there reaches every screen that lists a pipeline.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

const OWNER = "src/lib/pipelines.queries.ts";

const unwrapped = (node: ts.Expression): ts.Expression => {
  let at = node;
  for (;;) {
    if (
      ts.isParenthesizedExpression(at) ||
      ts.isNonNullExpression(at) ||
      ts.isAsExpression(at) ||
      ts.isSatisfiesExpression(at)
    ) {
      at = at.expression;
    } else if (
      ts.isBinaryExpression(at) &&
      at.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken
    ) {
      at = at.left;
    } else {
      return at;
    }
  }
};

// What stands for a parameter's `position` in the body: `a.position` for a
// plain name, or the local a destructured `{ position }` is bound to.
function positionRead(
  param: ts.ParameterDeclaration,
): (node: ts.Expression) => boolean {
  if (ts.isIdentifier(param.name)) {
    const name = param.name.text;
    return (node) =>
      ts.isPropertyAccessExpression(node) &&
      node.name.text === "position" &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === name;
  }
  if (ts.isObjectBindingPattern(param.name)) {
    const bound = param.name.elements.find((element) => {
      const key = element.propertyName ?? element.name;
      return ts.isIdentifier(key) && key.text === "position";
    });
    const local =
      bound && ts.isIdentifier(bound.name) ? bound.name.text : undefined;
    return (node) => ts.isIdentifier(node) && node.text === local;
  }
  return () => false;
}

type Comparator =
  | ts.ArrowFunction
  | ts.FunctionExpression
  | ts.FunctionDeclaration
  | ts.MethodDeclaration;

const isComparator = (node: ts.Node): node is Comparator =>
  ts.isArrowFunction(node) ||
  ts.isFunctionExpression(node) ||
  ts.isFunctionDeclaration(node) ||
  ts.isMethodDeclaration(node);

function returnedBy(fn: Comparator): ts.Expression[] {
  if (fn.body === undefined) return [];
  if (!ts.isBlock(fn.body)) return [fn.body];
  const returned: ts.Expression[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isReturnStatement(node) && node.expression) {
      returned.push(node.expression);
    } else if (!ts.isFunctionLike(node)) {
      node.forEachChild(visit);
    }
  };
  fn.body.forEachChild(visit);
  return returned;
}

function comparesPositions(fn: Comparator): boolean {
  const [first, second] = fn.parameters;
  if (fn.parameters.length !== 2 || !first || !second) return false;
  const firstRead = positionRead(first);
  const secondRead = positionRead(second);
  return returnedBy(fn).some((expression) => {
    const result = unwrapped(expression);
    if (
      !ts.isBinaryExpression(result) ||
      result.operatorToken.kind !== ts.SyntaxKind.MinusToken
    ) {
      return false;
    }
    const left = unwrapped(result.left);
    const right = unwrapped(result.right);
    return (
      (firstRead(left) && secondRead(right)) ||
      (secondRead(left) && firstRead(right))
    );
  });
}

/** The line of every two-argument function that returns one row's position less another's. */
export function positionComparators(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node): void => {
    if (isComparator(node) && comparesPositions(node)) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
      );
    }
    node.forEachChild(visit);
  };
  visit(source);
  return lines;
}

const judged = (file: CensusFile) =>
  file.area === "src" && file.source !== undefined;

export const rule: CensusRule = {
  name: "position-order",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) => {
    const owner = corpus.find((file) => file.rel === OWNER);
    if (owner?.source === undefined) return [`${OWNER} was not read`];
    return positionComparators(owner.source).length === 1
      ? []
      : [`${OWNER} holds no comparator this rule recognises`];
  },
  visit: (file) =>
    file.rel === OWNER || file.source === undefined
      ? []
      : positionComparators(file.source).map(
          (line) => `${file.rel}:${line}: compares two rows by position`,
        ),
  advice:
    "Put rows in position order with `inPositionOrder` from " +
    "src/lib/pipelines.queries.ts. A filter stays at the call site.",
};
