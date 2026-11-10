// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Asking the browser to copy has one spelling, and it is `useClipboardCopy`.
//
// `navigator.clipboard` is undefined outside a secure context, so the missing
// capability throws on the property access rather than rejecting, and a
// `.catch` never runs. That is invisible in every environment a developer tests
// in, so one site gets it right for all.
//
// The subject is the access, not the import: a local alias
// (`const writer = navigator.clipboard`) is the same offence.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

// The files allowed to name it: the one implementation, and the helper that
// installs a stub for it in tests.
const OWNERS: ReadonlySet<string> = new Set([
  "src/design-system/clipboardcopy.tsx",
  "src/design-system/clipboard-testing.ts",
]);

/**
 * Whether a file can carry the offence, decided without walking its tree. The
 * matcher fires on a property the source names, and every such name appears in
 * the text. So the filter is wider than the matcher, and spares the walk of
 * every file that never says the word.
 */
export function mayReachForTheClipboard(text: string): boolean {
  return text.includes("clipboard");
}

/**
 * The property this node reads, where the source says which one: the
 * identifier after a dot, or a static string in brackets. A computed key
 * (`navigator[whicheverApi]`) yields null, since no parser can decide it.
 */
function staticPropertyName(node: ts.Node): string | null {
  if (ts.isPropertyAccessExpression(node)) {
    return node.name.text;
  }
  if (
    ts.isElementAccessExpression(node) &&
    ts.isStringLiteralLike(node.argumentExpression)
  ) {
    return node.argumentExpression.text;
  }
  return null;
}

/**
 * Whether a binding pattern takes `clipboard` off the navigator. A rename
 * (`{ clipboard: writer }`) binds the same property, so the property is read,
 * never the local name.
 */
function destructuresTheClipboard(node: ts.Node): boolean {
  if (
    !ts.isVariableDeclaration(node) ||
    node.initializer === undefined ||
    !ts.isObjectBindingPattern(node.name)
  ) {
    return false;
  }
  if (!isTheNavigator(node.initializer)) {
    return false;
  }
  return node.name.elements.some(
    (element) => boundPropertyName(element) === "clipboard",
  );
}

/**
 * The property a binding element takes, where the source states which. The
 * quoted and bracketed keys bind the same property, and their text would carry
 * the quotes.
 */
function boundPropertyName(element: ts.BindingElement): string | null {
  const key = element.propertyName ?? element.name;
  if (ts.isIdentifier(key) || ts.isStringLiteralLike(key)) {
    return key.text;
  }
  if (
    ts.isComputedPropertyName(key) &&
    ts.isStringLiteralLike(key.expression)
  ) {
    return key.expression.text;
  }
  return null;
}

/** Whether `expression` names the navigator, however it is reached. */
function isTheNavigator(expression: ts.Expression): boolean {
  // Bare, `window.navigator`, `globalThis["navigator"]`: what matters is the
  // last name in the chain, not the route taken to it.
  if (ts.isIdentifier(expression)) {
    return expression.text === "navigator";
  }
  return staticPropertyName(expression) === "navigator";
}

/** Every line in `source` that reaches for `navigator.clipboard`. */
export function clipboardAccessesIn(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node) => {
    if (
      ((ts.isPropertyAccessExpression(node) ||
        ts.isElementAccessExpression(node)) &&
        staticPropertyName(node) === "clipboard" &&
        isTheNavigator(node.expression)) ||
      destructuresTheClipboard(node)
    ) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart()).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return lines;
}

/** Every line of one file that reaches for the clipboard. */
const sitesIn = (file: CensusFile): string[] =>
  file.source && mayReachForTheClipboard(file.text)
    ? clipboardAccessesIn(file.source).map((line) => `${file.rel}:${line}`)
    : [];

export const rule: CensusRule = {
  name: "clipboard-spelling",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.source !== undefined,
    ),
  // The count floor cannot notice the extension half's loss on its own.
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 modules were read"]),
    ...(corpus.some((file) => file.area === "extensions")
      ? []
      : ["the census covered frontend/src but no extension frontend layer"]),
    ...(corpus.some(
      (file) =>
        file.rel === "src/design-system/clipboardcopy.tsx" &&
        file.text.includes("navigator.clipboard"),
    )
      ? []
      : ["clipboardcopy.tsx no longer reaches for navigator.clipboard"]),
  ],
  visit: (file) => (OWNERS.has(file.rel) ? [] : sitesIn(file)),
  advice:
    "Copy through `useClipboardCopy` (src/design-system/clipboardcopy.tsx). " +
    "It guards the missing API outside a secure context and shows the one " +
    "notice; a hand-rolled `navigator.clipboard` gets one of them wrong.",
};
