// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A text limit is counted in characters, the server's unit, and the count has
// one spelling: `characterCount` (src/lib/charactercount.ts). Three arms:
//
//   native: a `maxLength` on a bare `<input>` or `<textarea>`. The browser
//     counts it in UTF-16 units, so an emoji takes two. `TextInput` and
//     `Textarea` take the same prop and count characters.
//   hand-rolled: `[...text].length` or `Array.from(text).length`, a second
//     copy of the count that can drift from the first.
//   units: `text.length` compared with a `TEXT_LIMITS` bound, which counts
//     UTF-16 units against a limit the server holds in characters.
//
// A spread prop reaching a bare element is past what a syntax tree can see.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

const OWNER = "src/lib/charactercount.ts";
const COMPARISONS: ReadonlySet<ts.SyntaxKind> = new Set([
  ts.SyntaxKind.LessThanToken,
  ts.SyntaxKind.LessThanEqualsToken,
  ts.SyntaxKind.GreaterThanToken,
  ts.SyntaxKind.GreaterThanEqualsToken,
]);

function nativeLimit(node: ts.Node): boolean {
  if (!ts.isJsxOpeningElement(node) && !ts.isJsxSelfClosingElement(node)) {
    return false;
  }
  const tag = node.tagName.getText();
  return (
    (tag === "input" || tag === "textarea") &&
    node.attributes.properties.some(
      (attribute) =>
        ts.isJsxAttribute(attribute) &&
        attribute.name.getText() === "maxLength",
    )
  );
}

function lengthOf(node: ts.Node): ts.Expression | undefined {
  return ts.isPropertyAccessExpression(node) && node.name.text === "length"
    ? node.expression
    : undefined;
}

// `[...text]` with nothing else in it, or `Array.from(text)` with no mapper.
function spreadsIntoCharacters(expression: ts.Expression): boolean {
  if (ts.isArrayLiteralExpression(expression)) {
    return (
      expression.elements.length === 1 &&
      ts.isSpreadElement(expression.elements[0])
    );
  }
  return (
    ts.isCallExpression(expression) &&
    expression.expression.getText() === "Array.from" &&
    expression.arguments.length === 1
  );
}

function unitsAgainstALimit(node: ts.Node): boolean {
  if (!ts.isBinaryExpression(node) || !COMPARISONS.has(node.operatorToken.kind))
    return false;
  const sides = [node.left, node.right];
  return sides.some(
    (side, index) =>
      lengthOf(side) !== undefined &&
      sides[1 - index].getText().includes("TEXT_LIMITS"),
  );
}

/** Every line in `source` that counts a text limit some other way. */
export function characterCountsIn(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node) => {
    const counted = lengthOf(node);
    if (
      nativeLimit(node) ||
      (counted !== undefined && spreadsIntoCharacters(counted)) ||
      unitsAgainstALimit(node)
    ) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return lines;
}

// Tests are left out: a case that proves the unit counts UTF-16 on purpose.
const judged = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.source !== undefined &&
  !/\.(test|stories)\.tsx?$/.test(file.rel) &&
  file.rel !== OWNER;

export const rule: CensusRule = {
  name: "character-count",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 modules were read"]),
    ...(corpus.some(
      (file) =>
        file.rel === "src/design-system/field.tsx" &&
        file.text.includes("withCharacterLimit"),
    )
      ? []
      : ["field.tsx no longer holds maxLength in characters"]),
  ],
  visit: (file) =>
    file.source
      ? characterCountsIn(file.source).map((line) => `${file.rel}:${line}`)
      : [],
  advice:
    "Count a text limit with `characterCount` (src/lib/charactercount.ts), " +
    "and give `TextInput` or `Textarea` the `maxLength`. The server counts " +
    "characters; `.length` and the native attribute count UTF-16 units.",
};
