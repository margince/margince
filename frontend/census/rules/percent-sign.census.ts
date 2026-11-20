// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A percent on screen is spelled by a percent formatter (`formatPercent`,
// `formatAnalyticsPercent`, `formatAnalyticsPercentPoints`), never by a literal
// `%` glued to a formatted number. German sets a no-break space before the
// sign and English none, so a hand-glued sign is wrong in one of them, and a
// board that mixes the two writes one number two ways.
//
// The subject is a formatter's output followed by a `%`: in a template
// literal, as the JSX text after it, or as the string added to it. A style
// value such as `${width}%` comes from no formatter and is not read.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

/** Whether a file can carry the offence: every match has a `%` in its text. */
export function mayGlueAPercent(text: string): boolean {
  return text.includes("%");
}

function isFormatterCall(node: ts.Node): boolean {
  if (!ts.isCallExpression(node)) return false;
  const callee = node.expression;
  const name = ts.isIdentifier(callee)
    ? callee.text
    : ts.isPropertyAccessExpression(callee)
      ? callee.name.text
      : "";
  return /^format/.test(name);
}

const opensWithPercent = (text: string) => /^\s?%/.test(text);

// The next span's expression puts the sign on: a string, or a conditional
// whose one branch is the sign.
function suppliesAPercent(node: ts.Expression): boolean {
  if (ts.isParenthesizedExpression(node)) {
    return suppliesAPercent(node.expression);
  }
  if (ts.isStringLiteralLike(node)) return opensWithPercent(node.text);
  if (ts.isConditionalExpression(node)) {
    return suppliesAPercent(node.whenTrue) || suppliesAPercent(node.whenFalse);
  }
  return false;
}

function spanGluesAPercent(span: ts.TemplateSpan): boolean {
  if (!isFormatterCall(span.expression)) return false;
  if (opensWithPercent(span.literal.text)) return true;
  const spans = span.parent.templateSpans;
  const next = spans[spans.indexOf(span) + 1];
  return (
    span.literal.text.trim() === "" &&
    next !== undefined &&
    suppliesAPercent(next.expression)
  );
}

function gluesAPercent(node: ts.Node): boolean {
  if (ts.isTemplateSpan(node)) return spanGluesAPercent(node);
  if (
    ts.isBinaryExpression(node) &&
    node.operatorToken.kind === ts.SyntaxKind.PlusToken
  ) {
    return (
      isFormatterCall(node.left) &&
      ts.isStringLiteralLike(node.right) &&
      opensWithPercent(node.right.text)
    );
  }
  if (ts.isJsxText(node)) {
    const parent = node.parent;
    if (!(ts.isJsxElement(parent) || ts.isJsxFragment(parent))) return false;
    const siblings = parent.children;
    const before = siblings[siblings.indexOf(node) - 1];
    return (
      before !== undefined &&
      ts.isJsxExpression(before) &&
      before.expression !== undefined &&
      isFormatterCall(before.expression) &&
      opensWithPercent(node.text)
    );
  }
  return false;
}

/** Every line in `source` that glues a `%` to a formatted number. */
export function gluedPercentsIn(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node) => {
    if (gluesAPercent(node)) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart()).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return lines;
}

const sitesIn = (file: CensusFile): string[] =>
  file.source && mayGlueAPercent(file.text)
    ? gluedPercentsIn(file.source).map((line) => `${file.rel}:${line}`)
    : [];

export const rule: CensusRule = {
  name: "percent-sign",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.source !== undefined,
    ),
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 modules were read"]),
    ...(corpus.some(
      (file) =>
        file.rel === "src/format/format.ts" &&
        file.text.includes('style: "percent"'),
    )
      ? []
      : ["src/format/format.ts no longer formats a percent with Intl"]),
  ],
  visit: sitesIn,
  advice:
    "Spell a percent with formatPercent (a fraction) or " +
    "formatAnalyticsPercentPoints (a figure from 0 to 100) from " +
    "src/format/format.ts. They set the sign as the reader's locale does.",
};
