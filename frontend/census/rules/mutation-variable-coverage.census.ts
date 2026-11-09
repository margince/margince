// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A `mutationFn` takes the value it needs as a variable, never from its
// closure.
//
// `useMutation` re-arms its options in a passive effect. Until that effect
// runs, the observer holds the previous render's closure, where the state is
// still null. A click in that window runs the old function and refuses.
//
// The tell is a zero-parameter `mutationFn` that throws on a falsy check of a
// value it closed over. Such a guard reports only the stale-closure window, and
// tells a reader with a filled form there is nothing to submit.
//
// Not caught: an unguarded closure read. Telling component state from a
// stable prop needs the type checker; the guard is the author's own admission
// that the value can be absent.

import ts from "typescript";
import type { CensusRule } from "../tree";

/** Every name bound inside the function body, so a local cannot read as free. */
function localNames(body: ts.Node): Set<string> {
  const names = new Set<string>();
  const visit = (node: ts.Node) => {
    // A destructured `const { data } = await …` binds through a BindingElement.
    if (
      (ts.isVariableDeclaration(node) ||
        ts.isParameter(node) ||
        ts.isBindingElement(node)) &&
      ts.isIdentifier(node.name)
    ) {
      names.add(node.name.text);
    }
    ts.forEachChild(node, visit);
  };
  visit(body);
  return names;
}

/** The root identifier of `!x` / `!x.y.z`, for every negation in the test. */
function negatedRoots(expression: ts.Node): string[] {
  const roots: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isPrefixUnaryExpression(node) &&
      node.operator === ts.SyntaxKind.ExclamationToken
    ) {
      let operand: ts.Node = node.operand;
      while (ts.isPropertyAccessExpression(operand)) {
        operand = operand.expression;
      }
      if (ts.isIdentifier(operand)) {
        roots.push(operand.text);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(expression);
  return roots;
}

function refuses(statement: ts.Node): boolean {
  let found = false;
  const visit = (node: ts.Node) => {
    if (ts.isThrowStatement(node)) {
      found = true;
    }
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "throwProblem"
    ) {
      found = true;
    }
    ts.forEachChild(node, visit);
  };
  visit(statement);
  return found;
}

export function findingsIn(shown: string, source: ts.SourceFile): string[] {
  const findings: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isPropertyAssignment(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "mutationFn" &&
      (ts.isArrowFunction(node.initializer) ||
        ts.isFunctionExpression(node.initializer)) &&
      node.initializer.parameters.length === 0 &&
      node.initializer.body
    ) {
      const body = node.initializer.body;
      const locals = localNames(body);
      const guards = (inner: ts.Node) => {
        if (ts.isIfStatement(inner) && refuses(inner.thenStatement)) {
          const free = negatedRoots(inner.expression).filter(
            (name) => !locals.has(name),
          );
          if (free.length > 0) {
            const { line } = source.getLineAndCharacterOfPosition(
              inner.getStart(source),
            );
            findings.push(
              `${shown}:${line + 1} — mutationFn takes no variable and refuses on closed-over ${free.map((n) => `\`${n}\``).join(", ")}; pass it to mutate() instead`,
            );
          }
        }
        ts.forEachChild(inner, guards);
      };
      guards(body);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return findings;
}

export const rule: CensusRule = {
  name: "mutation-variable-coverage",
  // A file with no `mutationFn` in its text cannot hold a finding.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.area === "src" &&
        /\.tsx?$/.test(file.path) &&
        file.text.includes("mutationFn"),
    ),
  visit: (file) => (file.source ? findingsIn(file.rel, file.source) : []),
  advice:
    "A mutationFn refuses on state it only closed over, so a click before the " +
    "observer re-arms refuses a filled form. Pass the value to mutate() and " +
    "take it as the mutationFn's parameter.",
};
