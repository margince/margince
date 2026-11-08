// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One plural rule: outside `i18n/`, a count does not pick a message key.
//
// A count that picks between two wordings follows some language's plural rule.
// This product uses the reader's, through `Intl.PluralRules` in
// `format/plural.ts`. A `count === 1` at the call site assumes two forms split
// at one. That holds for English, German and Vietnamese, not for Polish.
//
// The subject is derived. A conditional is a plural choice when both arms are
// keys the catalogue carries. The catalogue is imported, so a new key is in
// scope at once. No naming convention or suffix decides it.
//
// Not findings: a choice between two keys on something other than a count
// (`isPaid ? "x" : "y"`). A count comparison that yields no pair of keys (a
// width, an index, a boolean). A finding is a comparison against the
// literal 1 whose two branches are both catalogue keys.

import { join, relative, sep } from "node:path";
import ts from "typescript";
import { parseSource } from "../../scripts/lib/source-tree";
import { en } from "../../src/i18n/en";
import { type CensusRule, frontendRoot, srcDir } from "../tree";

// The home of the rule, so the one directory allowed to compare a count
// against 1 to reach a key. `format/plural.ts` owns the locale's categories and
// `i18n/index.tsx` owns the catalogue lookup; both implement the rule.
const ruleHome = join(srcDir, "i18n");
const pluralModule = join(srcDir, "format", "plural.ts");

const CATALOG_KEYS: ReadonlySet<string> = new Set(Object.keys(en));

type Finding = Readonly<{ file: string; line: number; text: string }>;

/** Whether this expression is a comparison of something against the literal 1. */
function comparesAgainstOne(test: ts.Expression): boolean {
  if (!ts.isBinaryExpression(test)) {
    return false;
  }
  const op = test.operatorToken.kind;
  const isEquality =
    op === ts.SyntaxKind.EqualsEqualsEqualsToken ||
    op === ts.SyntaxKind.EqualsEqualsToken ||
    op === ts.SyntaxKind.ExclamationEqualsEqualsToken ||
    op === ts.SyntaxKind.ExclamationEqualsToken ||
    op === ts.SyntaxKind.GreaterThanToken ||
    op === ts.SyntaxKind.LessThanToken ||
    op === ts.SyntaxKind.GreaterThanEqualsToken ||
    op === ts.SyntaxKind.LessThanEqualsToken;
  if (!isEquality) {
    return false;
  }
  // Either side: `1 === count` says the same thing.
  const one = (node: ts.Node): boolean =>
    ts.isNumericLiteral(node) && node.text === "1";
  return one(test.left) || one(test.right);
}

/**
 * The catalogue key this expression is, or null.
 *
 * A string literal or a no-substitution template: both are keys a caller can
 * write. A computed key is not one this gate can rule on, so it is no finding.
 *
 * A call carrying a key is the same choice one level down:
 * `n === 1 ? t("a") : t("b")` picks a wording as `t(n === 1 ? …)` does. The
 * callee is not matched, so renaming the translator is no way around the gate.
 * Any argument may be the key: `translate(locale, "a")` carries it second.
 */
function catalogKey(node: ts.Expression): string | null {
  if (ts.isParenthesizedExpression(node)) {
    return catalogKey(node.expression);
  }
  if (ts.isCallExpression(node)) {
    for (const argument of node.arguments) {
      const key = catalogKey(argument);
      if (key !== null) {
        return key;
      }
    }
    return null;
  }
  const text = ts.isStringLiteral(node)
    ? node.text
    : ts.isNoSubstitutionTemplateLiteral(node)
      ? node.text
      : null;
  return text !== null && CATALOG_KEYS.has(text) ? text : null;
}

// A count compared with 1 that picks between two keys is most often a plural
// choice, but not always. "One open deal, so naming it adds nothing" picks
// between two sentences, not two forms of one. The key names do not tell the
// two apart: the tree spells pairs `_one`, `One`, `Many` and bare nouns.
//
// So the author waives a site in the source, with a reason, as `craft:ignore`
// does. A waiver with no reason is not honoured.
const WAIVER = /plural-rule:allow[ \t]+(?<reason>\S.*)/;

/**
 * Whether the comment attached above this site waives it, with a reason.
 *
 * The comment is read off the syntax tree, from the statement the site sits
 * in, not by counting lines up from the expression. The formatter owns where
 * lines break, so a waiver matched by line count would come undone on the
 * next reflow.
 */
function waivedAt(source: string, node: ts.Node): boolean {
  let statement: ts.Node = node;
  while (statement.parent && !ts.isStatement(statement)) {
    statement = statement.parent;
  }
  const comments = [
    ...(ts.getLeadingCommentRanges(source, statement.getFullStart()) ?? []),
    ...(ts.getLeadingCommentRanges(source, node.getFullStart()) ?? []),
  ];
  return comments.some((range) =>
    WAIVER.test(source.slice(range.pos, range.end)),
  );
}

export function findingsIn(path: string, source: string): Finding[] {
  const parsed = parseSource(path, source);
  const rel = relative(frontendRoot, path).split(sep).join("/");
  const found: Finding[] = [];
  const walk = (node: ts.Node): void => {
    if (
      ts.isConditionalExpression(node) &&
      comparesAgainstOne(node.condition)
    ) {
      const whenTrue = catalogKey(node.whenTrue);
      const whenFalse = catalogKey(node.whenFalse);
      const line =
        parsed.getLineAndCharacterOfPosition(node.getStart(parsed)).line + 1;
      if (whenTrue && whenFalse && !waivedAt(source, node)) {
        found.push({
          file: rel,
          line,
          text: `${whenTrue} / ${whenFalse}`,
        });
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(parsed);
  return found;
}

export const rule: CensusRule = {
  name: "one-plural-rule",
  // The unit trees ship UI in the same bundle and the same three languages, so
  // the extension layers are read too.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.source !== undefined &&
        !file.path.startsWith(`${ruleHome}/`) &&
        file.path !== pluralModule,
    ),
  // Fail closed. A walk pointed at the wrong tree reports PASS over nothing.
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 files were read"]),
    ...(corpus.some((file) => file.area === "extensions")
      ? []
      : ["the census read no extension frontend layer"]),
  ],
  visit: (file) =>
    findingsIn(file.path, file.text).map(
      (f) => `${f.file}:${f.line} — ${f.text}`,
    ),
  advice:
    "A count picks a message key outside i18n/. Use usePlural() and a " +
    "<base>_one / <base>_other pair, so the form comes from the reader's " +
    "own plural rule rather than from a comparison with 1.",
};
