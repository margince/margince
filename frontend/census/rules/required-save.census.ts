// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A form with a required field keeps its submit enabled. Pressing it puts
// "{Field} is required." on each empty one and focuses the first
// (`useFieldFaults` in src/design-system/savepage.tsx). A submit greyed out
// while a field is empty says nothing about why.
//
// The rule finds a `confirmDisabled`, or a `disabled` on a `Button`, whose
// expression tests a value for emptiness, directly or through one const of the
// same file. A button that gives its reason (`reason`, `reasonId`,
// `confirmReason`) says why, so it is spared.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

const EMPTINESS =
  /\.trim\(\)|[!=]==\s*""|""\s*[!=]==|\.length\s*(===|>|<)\s*[01]\b/;
const EXPLAINED = new Set(["reason", "reasonId", "confirmReason"]);

function attributeNamed(
  element: ts.JsxOpeningLikeElement,
  name: string,
): ts.JsxAttribute | undefined {
  return element.attributes.properties.find(
    (property): property is ts.JsxAttribute =>
      ts.isJsxAttribute(property) && property.name.getText() === name,
  );
}

// Each const's initializer text by name, so `disabled={!ready}` is judged by
// what `ready` tests.
function constInitializers(source: ts.SourceFile): Map<string, string> {
  const consts = new Map<string, string>();
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer
    ) {
      consts.set(node.name.text, node.initializer.getText(source));
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return consts;
}

function testsEmptiness(
  expression: ts.Expression,
  consts: ReadonlyMap<string, string>,
): boolean {
  if (EMPTINESS.test(expression.getText())) return true;
  let named = false;
  const visit = (node: ts.Node) => {
    if (ts.isIdentifier(node) && EMPTINESS.test(consts.get(node.text) ?? "")) {
      named = true;
    }
    if (!named) ts.forEachChild(node, visit);
  };
  visit(expression);
  return named;
}

function heldSubmit(
  element: ts.JsxOpeningLikeElement,
): ts.Expression | undefined {
  for (const name of EXPLAINED) {
    if (attributeNamed(element, name)) return undefined;
  }
  const tag = element.tagName.getText();
  const held =
    attributeNamed(element, "confirmDisabled") ??
    (tag === "Button" ? attributeNamed(element, "disabled") : undefined);
  const initializer = held?.initializer;
  return initializer &&
    ts.isJsxExpression(initializer) &&
    initializer.expression
    ? initializer.expression
    : undefined;
}

/** Every line in `source` that greys out a submit on an empty value. */
export function heldSubmitsIn(source: ts.SourceFile): number[] {
  const consts = constInitializers(source);
  const lines: number[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const held = heldSubmit(node);
      if (held && testsEmptiness(held, consts)) {
        lines.push(
          source.getLineAndCharacterOfPosition(held.getStart(source)).line + 1,
        );
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return lines;
}

const judged = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.source !== undefined &&
  file.rel.endsWith(".tsx") &&
  !/\.(test|stories)\.tsx$/.test(file.rel);

export const rule: CensusRule = {
  name: "required-save",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) =>
    corpus.length > 200 ? [] : ["fewer than 200 modules were read"],
  visit: (file) =>
    file.source
      ? heldSubmitsIn(file.source).map((line) => `${file.rel}:${line}`)
      : [],
  advice:
    "Keep the submit enabled. On press, put each empty required field's " +
    "sentence on it with `useFieldFaults().show(blankRequired(...))` " +
    "(src/design-system/savepage.tsx, src/lib/faultsentence.ts), which also " +
    "focuses the first. A button that must stay off says why with `reason`.",
};
