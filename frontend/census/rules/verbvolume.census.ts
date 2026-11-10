// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// An AI verb draws its own ✦: `Button` with variant `ai` or `aiQuiet` renders
// the Sparkles mark itself. A caller that hands it one more draws two.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

// A computed variant counts as AI, since a lookup can come out as "ai".
// Only a literal naming another variant is known to draw no ✦.
function isAiButton(open: ts.JsxOpeningElement, source: ts.SourceFile) {
  if (open.tagName.getText(source) !== "Button") return false;
  return open.attributes.properties.some(
    (attribute) =>
      ts.isJsxAttribute(attribute) &&
      attribute.name.getText(source) === "variant" &&
      !namesPlainVariant(attribute.initializer),
  );
}

function namesPlainVariant(value: ts.JsxAttributeValue | undefined): boolean {
  const literal = value && ts.isJsxExpression(value) ? value.expression : value;
  return (
    literal !== undefined &&
    ts.isStringLiteral(literal) &&
    !/^ai(Quiet)?$/.test(literal.text)
  );
}

function isSparkles(node: ts.Node, source: ts.SourceFile): boolean {
  const open = ts.isJsxElement(node)
    ? node.openingElement
    : ts.isJsxSelfClosingElement(node)
      ? node
      : undefined;
  return open?.tagName.getText(source) === "Sparkles";
}

/** The line of every Sparkles drawn inside an AI button in one module. */
export function secondMarks(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const walk = (node: ts.Node, inAiButton: boolean) => {
    const inside =
      inAiButton ||
      (ts.isJsxElement(node) && isAiButton(node.openingElement, source));
    if (inside && isSparkles(node, source)) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
      );
    }
    node.forEachChild((child) => walk(child, inside));
  };
  walk(source, false);
  return lines;
}

const drawn = (file: CensusFile) =>
  file.source !== undefined &&
  ((file.area === "src" && file.rel.endsWith(".tsx")) ||
    file.area === "extensions");

export const rule: CensusRule = {
  name: "verbvolume",
  corpus: (tree) => tree.files.filter(drawn),
  floors: (corpus) =>
    corpus.length > 100 ? [] : ["fewer than 100 files were read"],
  visit: (file) =>
    file.source
      ? secondMarks(file.source).map((line) => `${file.rel}:${line}`)
      : [],
  advice:
    "Button draws the ✦ for variant ai and aiQuiet. Remove the Sparkles the " +
    "caller hands it, or the button draws two.",
};
