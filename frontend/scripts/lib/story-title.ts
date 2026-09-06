// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";

/**
 * The Storybook sidebar path a story file claims, or null.
 *
 * Resolves the DEFAULT EXPORT rather than reading the first `title:` in the
 * file, because a story's fixture data carries titles of its own —
 * dealroomthreads.stories.tsx opens with `title: "Commercial terms v4"`, the
 * name of a document in the fixture, and a scanner reading the first match
 * would file that story under a root called "Commercial terms v4".
 *
 * Shared rather than copied: two gates ask this question — the design-system
 * catalog checks every story's ROOT, and the settings catalog checks that every
 * settings story is filed under a group and page the catalog actually declares.
 * A second parser would drift from this one and the two would disagree about
 * which stories exist.
 */
export function storyTitle(path: string, text: string): string | null {
  const source = ts.createSourceFile(
    path,
    text,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const exported = source.statements.find(ts.isExportAssignment);
  if (!exported) return null;
  const named = unwrap(exported.expression);
  const meta = ts.isIdentifier(named)
    ? metaObjectNamed(source, named.text)
    : named;
  if (!meta || !ts.isObjectLiteralExpression(meta)) return null;
  for (const property of meta.properties) {
    if (!ts.isPropertyAssignment(property)) continue;
    if (propertyKey(property.name) !== "title") continue;
    const value = unwrap(property.initializer);
    if (ts.isStringLiteralLike(value)) return value.text;
  }
  return null;
}

// The key, whichever way it is written. `{ title: … }` and `{ "title": … }` are
// the same property, but reading the name's SOURCE TEXT compares the quotes
// too, so the quoted spelling matched nothing and the story fell out of the
// root check — skipped rather than reported, the one direction this gate must
// not be wrong in. A computed key is deliberately not resolved: what it
// evaluates to is not a question the parser can answer, and guessing would be
// worse than the honest null.
function propertyKey(name: ts.PropertyName): string | null {
  if (ts.isIdentifier(name) || ts.isStringLiteralLike(name)) return name.text;
  return null;
}

function metaObjectNamed(
  source: ts.SourceFile,
  name: string,
): ts.Expression | undefined {
  for (const statement of source.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (ts.isIdentifier(declaration.name) && declaration.name.text === name) {
        return declaration.initializer && unwrap(declaration.initializer);
      }
    }
  }
  return undefined;
}

// The type-only wrappers a story's metadata may be written through. They change
// nothing about the object underneath, so a scanner that stops at them reads no
// title where there is one.
function unwrap(expression: ts.Expression): ts.Expression {
  let node = expression;
  while (
    ts.isParenthesizedExpression(node) ||
    ts.isAsExpression(node) ||
    ts.isSatisfiesExpression(node) ||
    ts.isTypeAssertionExpression(node)
  ) {
    node = node.expression;
  }
  return node;
}
