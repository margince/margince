// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import ts from "typescript";
import { parseSource } from "./source-tree.ts";

/**
 * The Storybook sidebar path a story file or docs page claims, or null.
 *
 * A CSF file is read at its DEFAULT EXPORT rather than at the first `title:`,
 * because a story's fixture data carries titles of its own —
 * dealroomthreads.stories.tsx opens with `title: "Commercial terms v4"`, the
 * name of a document in the fixture, and a scanner reading the first match
 * would file that story under a root called "Commercial terms v4". An `.mdx`
 * page is read by Storybook's own MDX analyser, so it is filed as Storybook
 * files it.
 *
 * Every gate that files stories by title reads it here: a second parser would
 * drift from this one, and the two would disagree about which stories exist.
 */
export async function storyTitle(
  path: string,
  text: string,
): Promise<string | null> {
  if (path.endsWith(".mdx")) return docsTitle(text);
  const title = literalAt(defaultExportObject(parseSource(path, text)), [
    "title",
  ]);
  return title !== undefined && ts.isStringLiteralLike(title)
    ? title.text
    : null;
}

export type TitledFile = { path: string; title: string | null };

export function titledStories(files: string[]): Promise<TitledFile[]> {
  return Promise.all(
    files.map(async (path) => ({
      path,
      title: await storyTitle(path, readFileSync(path, "utf8")),
    })),
  );
}

// Loaded on first use: the analyser pulls in Storybook's server, which a gate
// reading only CSF files has no need of.
async function docsTitle(text: string): Promise<string | null> {
  const { analyzeMdx } = await import("storybook/internal/core-server");
  try {
    return (await analyzeMdx(text)).title ?? null;
  } catch (refused) {
    // Storybook's indexer refuses this page too; null reports it as untitled.
    if (refused instanceof Error) return null;
    throw refused;
  }
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
