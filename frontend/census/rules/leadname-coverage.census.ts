// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a lead is called is one fact: its own name if it has one, else its
// email address. `format/leadname.ts` answers it, mirroring `leadIdentityName`
// in the contacts module.
//
// The spelling refused here, `lead.full_name ?? lead.email`, fires on null
// alone. A lead with an empty `full_name` then renders blank, while the server
// names the same lead by its address.
//
// The subject is derived: every `??` or `||` chain under `src/` that falls back
// from a `full_name` to an `email` of the same record. Both operators count,
// because a second author of one rule is the defect.
//
// Left out of scope:
// - `full_name ?? ""` with no address behind it, which seeds a form field;
// - `email ?? full_name`, which is the address-first rule;
// - a name resolved through a variable or a helper, which needs the checker.

import ts from "typescript";
import { parseSource } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule } from "../tree";

/** The operators a fallback is spelled with. `??` carries the defect, and
 *  `||` is the same rule. */
function isFallbackOperator(kind: ts.SyntaxKind): boolean {
  return (
    kind === ts.SyntaxKind.QuestionQuestionToken ||
    kind === ts.SyntaxKind.BarBarToken
  );
}

/** An expression with its parentheses taken off. A reader's brackets do not
 *  change the rule. */
function unwrap(expression: ts.Expression): ts.Expression {
  return ts.isParenthesizedExpression(expression)
    ? unwrap(expression.expression)
    : expression;
}

/** The operands of one fallback chain, left to right, with both sides
 *  flattened. The order of the fields tells this rule from the address-first
 *  one. */
function chainOperands(node: ts.BinaryExpression): ts.Expression[] {
  return [node.left, node.right].flatMap((side) => {
    const inner = unwrap(side);
    return ts.isBinaryExpression(inner) &&
      isFallbackOperator(inner.operatorToken.kind)
      ? chainOperands(inner)
      : [inner];
  });
}

/** Whether this chain is the whole of its expression, climbing through
 *  parentheses. An inner link is one of the outer chain's operands, so it is
 *  not reported twice. */
function isOutermostChain(node: ts.BinaryExpression): boolean {
  let child: ts.Node = node;
  let parent: ts.Node = node.parent;
  while (ts.isParenthesizedExpression(parent)) {
    child = parent;
    parent = parent.parent;
  }
  return !(
    ts.isBinaryExpression(parent) &&
    isFallbackOperator(parent.operatorToken.kind) &&
    (parent.left === child || parent.right === child)
  );
}

/** One term of a chain: whose field it reads, and which. A contact's name with
 *  a lead's address behind it reads two records and is not this rule. */
type FieldRead = Readonly<{ receiver: string; field: string }>;

function fieldRead(expression: ts.Expression): FieldRead | null {
  const inner = ts.isParenthesizedExpression(expression)
    ? expression.expression
    : expression;
  if (ts.isPropertyAccessExpression(inner)) {
    return { receiver: inner.expression.getText(), field: inner.name.text };
  }
  if (ts.isElementAccessExpression(inner)) {
    const argument = inner.argumentExpression;
    return ts.isStringLiteral(argument)
      ? { receiver: inner.expression.getText(), field: argument.text }
      : null;
  }
  return null;
}

/** `<file>:<line>` for every hand-spelled lead name in one parsed file. */
function fallbacksOf(source: ts.SourceFile, name: string): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isBinaryExpression(node) &&
      isFallbackOperator(node.operatorToken.kind) &&
      isOutermostChain(node)
    ) {
      const terms = chainOperands(node).map(fieldRead);
      // Every name in the chain is asked. A company-first label puts another
      // record's `full_name` ahead of the lead's own.
      const named = terms.some(
        (term, at) =>
          term?.field === "full_name" &&
          terms
            .slice(at + 1)
            .some(
              (later) =>
                later?.field === "email" && later.receiver === term.receiver,
            ),
      );
      if (named) {
        const { line } = source.getLineAndCharacterOfPosition(node.getStart());
        found.push(`${name}:${line + 1}`);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

/** `<file>:<line>` for every hand-spelled lead name in planted text. */
export function fallbacksIn(fileName: string, text: string): string[] {
  return fallbacksOf(parseSource(fileName, text), fileName);
}

// The files that own the rule, named one by one. A second helper beside
// `leadname.ts` would otherwise pass the census.
const LEAD_NAME_OWNERS: ReadonlySet<string> = new Set([
  "src/format/leadname.ts",
  "src/format/leadname.test.ts",
]);

const judged = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.path) &&
  !LEAD_NAME_OWNERS.has(file.rel);

export const rule: CensusRule = {
  name: "leadname-coverage",
  corpus: (tree) => tree.files.filter(judged),
  visit: (file) => (file.source ? fallbacksOf(file.source, file.rel) : []),
  advice:
    "A lead's name is worked out in one place: call leadName() from " +
    "format/leadname.ts instead of spelling `full_name ?? email` again. " +
    "That fallback renders a lead with an empty name as blank.",
};
