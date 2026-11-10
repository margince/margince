// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A panel or card head is a title and, optionally, the verbs that act on it.
// Never a description: copy that adds something is body content.
//
// A head that grows for a sentence stands at two heights, so a column of
// panels goes ragged. design-system/panelhead.test.tsx holds the components'
// props and the title's type; this rule holds the tree, in three arms:
//
//   call  - a caller passing `sub`, `description` or `intro` to a head
//           component, which a component spreading `...rest` would take.
//   class - markup wearing `card-sub`, the class the slot wore.
//   sheet - a stylesheet declaring the class such a slot would need.

import ts from "typescript";
import { selectorList } from "../../scripts/lib/css-rules";
import { withoutComments } from "../../src/testing/css";
import type { CensusFile, CensusRule } from "../tree";

// The four components a head is built from.
const HEAD_COMPONENTS = new Set([
  "Panel",
  "PanelGroupHead",
  "Card",
  "SectionHeader",
]);

// Three spellings of the same slot, so renaming the prop does not answer it.
export const DESCRIPTION_PROPS = ["sub", "description", "intro"];

// The tree carries a thousand-odd components and nearly two hundred sheets.
const TSX_FLOOR = 500;
const CSS_FLOOR = 50;

const lineOf = (source: ts.SourceFile, node: ts.Node) =>
  source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

/** Every description attribute passed to a head component, as `Tag prop`. */
export function descriptionAttributes(
  source: ts.SourceFile,
): { line: number; attribute: string }[] {
  const found: { line: number; attribute: string }[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const tag = node.tagName.getText(source);
      if (HEAD_COMPONENTS.has(tag)) {
        for (const property of node.attributes.properties) {
          if (!ts.isJsxAttribute(property)) continue;
          const attribute = property.name.getText(source);
          if (!DESCRIPTION_PROPS.includes(attribute)) continue;
          found.push({
            line: lineOf(source, property),
            attribute: `${tag} ${attribute}`,
          });
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

// `card-sub` is the tree's own spelling of the slot. The boundaries keep
// `dcard-subject` and `card-subtitle` out.
const SUB_CLASS = /(?<![\w-])card-sub(?![\w-])/;

// Read off the attribute's whole initializer, so a list assembled in a
// template or picked by a condition is read too.
export function subClassLines(source: ts.SourceFile): number[] {
  const found: number[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isJsxAttribute(node) &&
      node.name.getText(source) === "className" &&
      node.initializer !== undefined &&
      SUB_CLASS.test(node.initializer.getText(source))
    ) {
      found.push(lineOf(source, node));
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

// A class a description slot would need: the `-head-sub` or `-title-sub`
// suffix on whichever surface grew it, or `card-sub` itself.
const HEAD_SUB_CLASS =
  /\.[\w-]*-(?:head|title)-sub(?![\w-])|\.card-sub(?![\w-])/;

// The other shape: a generic `.sub` scoped inside a head, with no name of its
// own for a name-only detector to see.
const SUB_IN_HEAD = /\.(?:panel-head|section-header)\b[^,{}]*\s\.sub(?![\w-])/;

export function headSubSelectors(css: string): readonly string[] {
  return [...withoutComments(css).matchAll(/([^{}]+)\{[^{}]*\}/g)]
    .flatMap(([, selectors]) => selectorList(selectors))
    .map((selector) => selector.replace(/\s+/g, " "))
    .filter(
      (selector) => HEAD_SUB_CLASS.test(selector) || SUB_IN_HEAD.test(selector),
    );
}

// Every .tsx under src and every module of an extension layer: a unit's
// screen is shipped UI in the same bundle. The sheets are the core's.
const isMarkup = (file: CensusFile) =>
  (file.area === "src" && file.path.endsWith(".tsx")) ||
  (file.area === "extensions" && file.source !== undefined);
const isSheet = (file: CensusFile) =>
  file.area === "src" && file.path.endsWith(".css");

export const rule: CensusRule = {
  name: "panelhead",
  corpus: (tree) =>
    tree.files.filter((file) => isMarkup(file) || isSheet(file)),
  floors: (corpus) => [
    ...(corpus.filter(isMarkup).length > TSX_FLOOR
      ? []
      : [`${TSX_FLOOR} or fewer components were read`]),
    ...(corpus.some((file) => file.area === "extensions")
      ? []
      : ["the census reached no extension frontend layer"]),
    ...(corpus.filter(isSheet).length > CSS_FLOOR
      ? []
      : [`${CSS_FLOOR} or fewer stylesheets were read`]),
  ],
  visit: (file) => {
    if (isSheet(file)) {
      return headSubSelectors(file.text).map(
        (selector) => `${file.rel}: ${selector}`,
      );
    }
    if (!file.source) return [];
    return [
      ...descriptionAttributes(file.source).map(
        ({ line, attribute }) =>
          `${file.rel}:${line}: passes ${attribute} to a head`,
      ),
      ...subClassLines(file.source).map(
        (line) => `${file.rel}:${line}: wears card-sub`,
      ),
    ];
  },
  advice:
    "A panel or card head is a title and its verbs, never a description; put the sentence in the body.",
};
