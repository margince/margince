// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import { parseSource } from "./source-tree";

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
  if (path.endsWith(".mdx")) return mdxTitle(text);
  const title = literalAt(defaultExportObject(parseSource(path, text)), [
    "title",
  ]);
  return title !== undefined && ts.isStringLiteralLike(title)
    ? title.text
    : null;
}

// No MDX parser is resolvable here, so the read is strict: one `<Meta>` outside
// comments and code fences, carrying only a quoted title. Anything else is null.
function mdxTitle(text: string): string | null {
  const prose = text
    .replace(/\{\/\*[\s\S]*?\*\/\}/g, "")
    .replace(/^```[^\n]*\n[\s\S]*?^```[^\n]*$/gm, "");
  const metas = [...prose.matchAll(/<Meta\b[^>]*>/g)];
  if (metas.length !== 1) return null;
  return /^<Meta\s+title="([^"{}]+)"\s*\/>$/.exec(metas[0][0])?.[1] ?? null;
}

// The object literal a module default-exports, directly or through the `const`
// it names; undefined for anything a literal read cannot resolve.
export function defaultExportObject(
  source: ts.SourceFile,
): ts.ObjectLiteralExpression | undefined {
  const exported = source.statements.find(ts.isExportAssignment);
  if (exported === undefined) return undefined;
  const value = unwrap(exported.expression);
  const resolved = ts.isIdentifier(value)
    ? source.statements
        .filter(ts.isVariableStatement)
        .flatMap((statement) => statement.declarationList.declarations)
        .find(
          (declaration) =>
            ts.isIdentifier(declaration.name) &&
            declaration.name.text === value.text,
        )?.initializer
    : value;
  const object = resolved && unwrap(resolved);
  return object && ts.isObjectLiteralExpression(object) ? object : undefined;
}

// `{ title: … }` and `{ "title": … }` are one property.
// A computed key is not resolved: a guessed title is worse than null.
export function literalAt(
  object: ts.ObjectLiteralExpression | undefined,
  path: string[],
): ts.Expression | undefined {
  let value: ts.Expression | undefined = object;
  for (const key of path) {
    if (value === undefined || !ts.isObjectLiteralExpression(value)) {
      return undefined;
    }
    const property = value.properties
      .filter(ts.isPropertyAssignment)
      .find(
        ({ name }) =>
          (ts.isIdentifier(name) || ts.isStringLiteral(name)) &&
          name.text === key,
      );
    value = property && unwrap(property.initializer);
  }
  return value;
}

// The type-only wrappers a story's metadata may be written through. They change
// nothing about the object underneath, so a scanner that stops at them reads no
// title where there is one.
export function unwrap(expression: ts.Expression): ts.Expression {
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
