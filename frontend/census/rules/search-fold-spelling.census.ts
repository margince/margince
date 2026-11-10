// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A reader's text matched by a part of it has one fold, `foldForSearch`, via
// `searchWords` and `holdsEveryWord`. A bare `toLowerCase` misses "Muller"
// for "Müller", which the palette finds. `foldForMatch` stays for equality.

// A case-folded needle is text a reader typed: one the code wrote needs no
// fold. A `foldForMatch` before a substring test is equality put to search.
// Either counts inline or through a binding declared from one.

import ts from "typescript";
import { declarationOf } from "../../src/testing/classnames";
import type { CensusFile, CensusRule } from "../tree";

const FIXTURE = /\.(test|stories)\.[cm]?[jt]sx?$|\.testkit\.[jt]sx?$/;

const SUBSTRING_TESTS: ReadonlySet<string> = new Set([
  "includes",
  "startsWith",
  "endsWith",
  "indexOf",
]);

const CASE_FOLDS: ReadonlySet<string> = new Set([
  "toLowerCase",
  "toLocaleLowerCase",
  "toUpperCase",
  "toLocaleUpperCase",
  "foldForMatch",
]);
const EQUALITY_FOLD: ReadonlySet<string> = new Set(["foldForMatch"]);

// Whether an expression's value comes out of one of `folds`, anywhere along
// its chain: `a.toLowerCase()`, `a.trim().toLowerCase().normalize()`,
// `foldForMatch(a)`, or a name bound to one of those.
function comesOutOf(
  folds: ReadonlySet<string>,
  node: ts.Expression,
  seen = new Set<ts.Node>(),
): boolean {
  if (seen.has(node)) return false;
  seen.add(node);
  if (ts.isParenthesizedExpression(node) || ts.isNonNullExpression(node)) {
    return comesOutOf(folds, node.expression, seen);
  }
  if (ts.isIdentifier(node)) {
    const declared = declarationOf(node);
    return (
      declared !== undefined &&
      ts.isVariableDeclaration(declared) &&
      declared.initializer !== undefined &&
      comesOutOf(folds, declared.initializer, seen)
    );
  }
  if (ts.isCallExpression(node)) {
    const callee = node.expression;
    if (ts.isIdentifier(callee)) return folds.has(callee.text);
    if (ts.isPropertyAccessExpression(callee)) {
      return (
        folds.has(callee.name.text) ||
        comesOutOf(folds, callee.expression, seen)
      );
    }
  }
  if (ts.isPropertyAccessExpression(node)) {
    return comesOutOf(folds, node.expression, seen);
  }
  return false;
}

export function rawFoldMatchesIn(source: ts.SourceFile): number[] {
  const lines = new Set<number>();
  const visit = (node: ts.Node) => {
    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      SUBSTRING_TESTS.has(node.expression.name.text) &&
      (node.arguments.some((needle) => comesOutOf(CASE_FOLDS, needle)) ||
        comesOutOf(EQUALITY_FOLD, node.expression.expression))
    ) {
      lines.add(source.getLineAndCharacterOfPosition(node.getStart()).line + 1);
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return [...lines];
}

const judged = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.source !== undefined &&
  !FIXTURE.test(file.path);

export const rule: CensusRule = {
  name: "search-fold-spelling",
  corpus: (tree) => tree.files.filter(judged),
  // A census that judged nothing certifies nothing.
  floors: (corpus) => [
    ...(corpus.length > 500 ? [] : ["fewer than 500 modules were read"]),
    ...(corpus.some((file) => file.area === "extensions")
      ? []
      : ["the census covered frontend/src but no extension frontend layer"]),
  ],
  visit: (file) =>
    file.source
      ? rawFoldMatchesIn(file.source).map((line) => `${file.rel}:${line}`)
      : [],
  advice:
    "Match a reader's text by a part of it through foldForSearch, with " +
    "searchWords and holdsEveryWord. A bare toLowerCase or foldForMatch " +
    "before includes/startsWith/endsWith/indexOf misses accented spellings.",
};
