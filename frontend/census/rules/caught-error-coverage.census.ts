// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A caught failure reaches the screen through `problemMessageOf`
// (lib/problem.ts), never in the server's own words.
//
// The server composes a refusal's detail from its error text, so a raw
// `permission_denied` reads "contact.update: permission denied". Rendering it
// shows the refused reader the shape of the authority model.
// `problemMessageOf` replaces that arm with catalog copy.
//
// The subject set is every `catch` binding under src/, found by parsing, so a
// new screen is enrolled the moment it is written.
//
// Five arms, each keyed on what hid a real copy:
//   1. `.message` off the binding, often behind an `instanceof Error` guard.
//   2. `String(binding)`, the fallback half of that same ternary.
//   3. `${binding}` in a template, and `binding.toString()`.
//   4. A local alias such as `const failure = err`.
//   5. `<anything>.error.message`, a react-query result read with no catch.
//
// Not caught: a binding handed to a helper that coerces it, a binding stored
// in state and read later, and a destructured read (`const { message } = err`).
// Following those needs the type checker and a rule about which helpers render.
//
// The one sanctioned console path is `logUnexpectedError`, which takes the
// error itself and never its text, so no console sink is exempt.

import ts from "typescript";
import type { CensusRule } from "../tree";

// A cast, a parenthesis, `satisfies` and a non-null assertion all leave the
// same value in place. One helper serves every arm, so no arm misses a wrapper.
function unwrap(node: ts.Expression): ts.Expression {
  let inner = node;
  while (
    ts.isParenthesizedExpression(inner) ||
    ts.isAsExpression(inner) ||
    ts.isSatisfiesExpression(inner) ||
    ts.isNonNullExpression(inner) ||
    ts.isTypeAssertionExpression(inner)
  ) {
    inner = inner.expression;
  }
  return inner;
}

/** A rendered read of one caught binding: `<arm> <file>:<line>`. */
export function disclosuresIn(
  fileName: string,
  source: ts.SourceFile,
): string[] {
  const found: string[] = [];
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

  const visitCatch = (clause: ts.CatchClause) => {
    const declared = clause.variableDeclaration?.name;
    if (!declared || !ts.isIdentifier(declared)) {
      return;
    }
    // Arm 4: the binding plus every local alias of it, grown as the block is
    // walked. Declarations precede their uses, so one pass suffices.
    const names = new Set([declared.text]);

    const isBinding = (node: ts.Expression): boolean => {
      const inner = unwrap(node);
      return ts.isIdentifier(inner) && names.has(inner.text);
    };

    const scan = (node: ts.Node) => {
      if (
        ts.isVariableDeclaration(node) &&
        ts.isIdentifier(node.name) &&
        node.initializer &&
        isBinding(node.initializer)
      ) {
        names.add(node.name.text);
      }
      if (ts.isPropertyAccessExpression(node) && isBinding(node.expression)) {
        if (node.name.text === "message") {
          found.push(`message ${fileName}:${at(node)}`);
        }
        if (node.name.text === "toString") {
          found.push(`toString ${fileName}:${at(node)}`);
        }
      }
      // `err["message"]` is the same read through the other accessor.
      if (
        ts.isElementAccessExpression(node) &&
        isBinding(node.expression) &&
        ts.isStringLiteral(node.argumentExpression) &&
        node.argumentExpression.text === "message"
      ) {
        found.push(`message ${fileName}:${at(node)}`);
      }
      if (
        ts.isCallExpression(node) &&
        ts.isIdentifier(node.expression) &&
        node.expression.text === "String" &&
        node.arguments.length === 1 &&
        isBinding(node.arguments[0])
      ) {
        found.push(`String ${fileName}:${at(node)}`);
      }
      if (ts.isTemplateSpan(node) && isBinding(node.expression)) {
        found.push(`template ${fileName}:${at(node)}`);
      }
      ts.forEachChild(node, scan);
    };
    scan(clause.block);
  };

  // Arm 5 needs no catch clause. Either accessor on either hop counts:
  // `r.error.message`, `r["error"].message`, `r.error["message"]` and both
  // brackets are one read.
  const namesProperty = (node: ts.Node, property: string): boolean => {
    if (ts.isPropertyAccessExpression(node)) {
      return node.name.text === property;
    }
    return (
      ts.isElementAccessExpression(node) &&
      ts.isStringLiteral(node.argumentExpression) &&
      node.argumentExpression.text === property
    );
  };
  const accessed = (node: ts.Node): ts.Expression | undefined =>
    ts.isPropertyAccessExpression(node) || ts.isElementAccessExpression(node)
      ? node.expression
      : undefined;
  const visitResultError = (node: ts.Node) => {
    const target = accessed(node);
    const beneath = target === undefined ? undefined : unwrap(target);
    if (
      namesProperty(node, "message") &&
      beneath !== undefined &&
      namesProperty(beneath, "error")
    ) {
      found.push(`result.error ${fileName}:${at(node)}`);
    }
    ts.forEachChild(node, visitResultError);
  };

  const visit = (node: ts.Node) => {
    if (ts.isCatchClause(node)) {
      visitCatch(node);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  visitResultError(source);
  return found;
}

export const rule: CensusRule = {
  name: "caught-error-coverage",
  // Arms 1 to 4 need a catch clause and arm 5 needs `error`, so a file with
  // neither word cannot hold a finding. `r["error"]` has no dot, so the
  // narrowing is on the bare word.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.area === "src" &&
        /\.tsx?$/.test(file.path) &&
        !/\.test\.tsx?$/.test(file.path) &&
        (file.text.includes("catch") || file.text.includes("error")),
    ),
  visit: (file) => (file.source ? disclosuresIn(file.rel, file.source) : []),
  advice:
    "A caught failure reaches the screen in the server's own words. Render it " +
    "through problemMessageOf(err, t) from lib/problem.ts.",
};
