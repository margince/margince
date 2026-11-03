// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  parseSource,
} from "../../scripts/lib/source-tree";
import { declarationOf } from "../testing/classnames";

// A reader's text matched by a part of it has one fold, `foldForSearch`, via
// `searchWords` and `holdsEveryWord`. A bare `toLowerCase` misses "Muller"
// for "Müller", which the palette finds. `foldForMatch` stays for equality.

// A case-folded needle is text a reader typed: one the code wrote needs no
// fold. A `foldForMatch` before a substring test is equality put to search.
// Either counts inline or through a binding declared from one.

const frontendRoot = resolve(__dirname, "..", "..");
const repoRoot = resolve(frontendRoot, "..");
const extensionsDir = join(repoRoot, "extensions");

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

function rawFoldMatchesIn(source: ts.SourceFile): number[] {
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

function filesInTheCensus(): string[] {
  return [
    ...filesUnder(join(frontendRoot, "src")),
    ...extensionFrontendFiles(extensionsDir),
  ].filter((path) => !FIXTURE.test(path));
}

function sitesIn(files: readonly string[]): string[] {
  return files.flatMap((path) => {
    const where = relative(repoRoot, path);
    const source = parseSource(path, readFileSync(path, "utf8"));
    return rawFoldMatchesIn(source).map((line) => `${where}:${line}`);
  });
}

// One substring match per line, in every shape the tree has spelled one.
const PLANTED_MATCHES = [
  "{ const needle = q.trim().toLowerCase(); name.toLowerCase().includes(needle); }",
  "{ const typed = q.toLocaleLowerCase(); row.label.startsWith(typed); }",
  "haystack.includes(query.toLowerCase());",
  "foldForMatch(deal.name).includes(needle);",
  "{ const term = q.normalize('NFKC').toLowerCase(); [a, b].join(' ').includes(term); }",
  "(label ?? '').indexOf(foldForMatch(q));",
  "{ const folded = q.toLowerCase(); name.includes(folded); }",
  "function f(q: string) { const n = q.toLowerCase(); return x.includes(n); }",
  "{ const deal = foldForMatch(name); deal.includes(needle); }",
] as const;

// Not a reader's text matched by a part of it: equality, the shared fold, a
// needle the code wrote, a list.
const PLANTED_INNOCENTS = [
  "foldForMatch(a) === foldForMatch(b);",
  "foldForSearch(name).includes(word);",
  "holdsEveryWord(searchWords(query), [name]);",
  "file.name.toLowerCase().endsWith('.csv');",
  "host.toLowerCase().endsWith('.' + allowed);",
  "picked.includes(name);",
  "const n = q.trim(); x.includes(n);",
] as const;

describe("one fold for a substring match", () => {
  it("is foldForSearch everywhere text is matched by a part of it", {
    timeout: 60_000,
  }, () => {
    const files = filesInTheCensus();

    // A census that judged nothing certifies nothing.
    expect(files.length).toBeGreaterThan(500);
    expect(
      files.some((file) => file.startsWith(`${extensionsDir}/`)),
      "the census covered frontend/src but no extension frontend layer",
    ).toBe(true);

    const sites = sitesIn(files);
    expect(sites, `\n${sites.join("\n")}\n`).toEqual([]);
  });

  it("finds the match however the caller folds it", () => {
    const planted = parseSource(
      "planted.ts",
      [...PLANTED_MATCHES, ...PLANTED_INNOCENTS].join("\n"),
    );

    expect(rawFoldMatchesIn(planted)).toEqual(
      PLANTED_MATCHES.map((_, index) => index + 1),
    );
  });
});
