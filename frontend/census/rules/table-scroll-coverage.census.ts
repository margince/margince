// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A table that runs past its box needs a way for the keyboard to reach the
// rest. `TableScroll` (tablescroll.tsx) is that box: it takes a tab stop and
// names itself as a region while it holds something past its edge. A
// hand-written `overflow-x: auto` gives the mouse everything and the keyboard
// nothing, and the page still looks right.
//
// The subject is every `<table>` in JSX under src/. Not caught: a table
// rendered into a wrapper from another file, and a local component that is
// only named `TableScroll`. The ancestor is matched by tag name.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

// `listgrid.tsx` draws its own `.lt-scroll` box through `useScrollRegion`, the
// hook TableScroll calls, so it has the same tab stop and name.
const THE_BOX_ITSELF = ["src/design-system/listgrid.tsx"];

/** The names of JSX elements enclosing `node`, innermost first. */
function ancestorElements(node: ts.Node, source: ts.SourceFile): string[] {
  const names: string[] = [];
  for (let at = node.parent; at !== undefined; at = at.parent) {
    if (ts.isJsxElement(at)) {
      names.push(at.openingElement.tagName.getText(source));
    }
  }
  return names;
}

// A table drawn only for a screen reader has no visible box to scroll. The
// `sr-only` class already states that, so no file list is needed.
function srOnly(
  opening: ts.JsxOpeningElement | ts.JsxSelfClosingElement,
  source: ts.SourceFile,
): boolean {
  return opening.attributes.properties.some(
    (attribute) =>
      ts.isJsxAttribute(attribute) &&
      attribute.name.getText(source) === "className" &&
      attribute.initializer !== undefined &&
      ts.isStringLiteral(attribute.initializer) &&
      attribute.initializer.text.split(/\s+/).includes("sr-only"),
  );
}

/** The line of every `<table>` in one module with no TableScroll above it. */
export function unwrappedIn(source: ts.SourceFile): number[] {
  const found: number[] = [];
  const visit = (node: ts.Node) => {
    const opening = ts.isJsxElement(node)
      ? node.openingElement
      : ts.isJsxSelfClosingElement(node)
        ? node
        : undefined;
    if (
      opening !== undefined &&
      opening.tagName.getText(source) === "table" &&
      !ancestorElements(node, source).includes("TableScroll") &&
      !srOnly(opening, source)
    ) {
      found.push(
        source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const drawn = (file: CensusFile) =>
  file.area === "src" &&
  file.rel.endsWith(".tsx") &&
  !/\.(test|stories)\.tsx$/.test(file.rel) &&
  !THE_BOX_ITSELF.includes(file.rel);

export const rule: CensusRule = {
  name: "table-scroll-coverage",
  corpus: (tree) => tree.files.filter(drawn),
  // A sweep that found no table and a tree with none look the same.
  floors: (corpus) =>
    corpus.some((file) => file.text.includes("<table"))
      ? []
      : ["no <table> was found under src/"],
  visit: (file) =>
    file.source
      ? unwrappedIn(file.source).map((line) => `${file.rel}:${line}`)
      : [],
  advice:
    "Wrap a hand-drawn <table> in TableScroll (design-system/tablescroll.tsx), " +
    "so a keyboard reader can reach the columns past the edge.",
};
