// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A titled boxed surface standing in a page's column is a `Panel`, whatever
// the page. The panel's head is one fixed band at one height, which lets a
// reader scan a column of zones by their titles. A card's header grows with its
// description, so a page mixing the two draws one zone at several heights.
//
// Two shapes of the offence: the prop form `<Card title=…>`, and a
// `SectionHeader` put in by hand under a padding wrapper.
//
// An untitled `Card` is not judged. A list item, an inset or a dialog body is
// still the one card surface, and only the head band claims a zone.

import ts from "typescript";
import { SCREEN_ROOTS } from "../../src/testing/screenfiles";
import type { CensusFile, CensusRule } from "../tree";

/**
 * The screens whose titled `Card` stands inside another surface rather than in
 * a page column, each with the reason. A file, not a line: a line number moves
 * with every edit above it.
 */
export const notARecordZone = new Map<string, string>([
  [
    "features/contacts/contactnetwork/edgedetail.tsx",
    "the selected node's detail in the relationship map's own panel slot, level 3 under the map's heading",
  ],
]);

/** The head band a card draws when it is claiming to be a zone. */
const HEAD_BAND = "SectionHeader";

/**
 * The tags that own a head of their own. A `SectionHeader` inside one is that
 * surface's head, not the card's around it, so the walk stops there.
 */
const OWNS_ITS_HEAD = new Set(["Card", "Panel", "RailPanel", "Modal"]);

/**
 * The JSX children a reader sees. An expression wrapper is looked through:
 * `{staged && <SectionHeader/>}` and `{rows.map(() => <SectionHeader/>)}` both
 * render the header. What an expression yields comes back through here, since
 * a fragment draws no tag of its own.
 */
function renderedChildren(children: readonly ts.JsxChild[]): ts.JsxChild[] {
  return children.flatMap((child) => {
    if (ts.isJsxText(child)) {
      return child.containsOnlyTriviaWhiteSpaces ? [] : [child];
    }
    if (ts.isJsxFragment(child)) {
      return renderedChildren(child.children);
    }
    if (ts.isJsxExpression(child)) {
      return child.expression === undefined
        ? []
        : renderedChildren(jsxWithin(child.expression));
    }
    return [child];
  });
}

/** The JSX an expression renders, however many guards and calls are in the way. */
function jsxWithin(node: ts.Node): ts.JsxChild[] {
  if (
    ts.isJsxElement(node) ||
    ts.isJsxSelfClosingElement(node) ||
    ts.isJsxFragment(node)
  ) {
    return [node];
  }
  const found: ts.JsxChild[] = [];
  ts.forEachChild(node, (child) => {
    found.push(...jsxWithin(child));
  });
  return found;
}

function tagOf(child: ts.JsxChild): string | undefined {
  if (ts.isJsxElement(child)) {
    return child.openingElement.tagName.getText();
  }
  if (ts.isJsxSelfClosingElement(child)) {
    return child.tagName.getText();
  }
  return undefined;
}

function childrenOf(child: ts.JsxChild): ts.JsxChild[] {
  return ts.isJsxElement(child) ? renderedChildren(child.children) : [];
}

/**
 * The hand-rolled head: a `SectionHeader` among the card's own children, or
 * inside the padding wrapper that is its first child. Deeper than two levels
 * the header belongs to something inside the card.
 */
function drawsAHeadBand(card: ts.JsxElement): boolean {
  const children = renderedChildren(card.children);
  if (children.some((child) => tagOf(child) === HEAD_BAND)) {
    return true;
  }
  const first = children[0];
  if (first === undefined || OWNS_ITS_HEAD.has(tagOf(first) ?? "")) {
    return false;
  }
  return childrenOf(first).some((child) => tagOf(child) === HEAD_BAND);
}

function carriesATitle(attributes: ts.JsxAttributes): boolean {
  return attributes.properties.some(
    (property) =>
      ts.isJsxAttribute(property) && property.name.getText() === "title",
  );
}

export type TitledCard = { where: string; line: number };

/** Every titled `Card` in one parsed module, in source order. */
export function titledCardsIn(
  where: string,
  source: ts.SourceFile,
): TitledCard[] {
  const found: TitledCard[] = [];
  const visit = (node: ts.Node) => {
    const opening = ts.isJsxElement(node)
      ? node.openingElement
      : ts.isJsxSelfClosingElement(node)
        ? node
        : undefined;
    if (opening !== undefined && opening.tagName.getText() === "Card") {
      const titled =
        carriesATitle(opening.attributes) ||
        (ts.isJsxElement(node) && drawsAHeadBand(node));
      if (titled) {
        found.push({
          where,
          line:
            source.getLineAndCharacterOfPosition(opening.getStart()).line + 1,
        });
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

/** A screen module as the allowlist spells it: relative to `src/`. */
const pathOf = (file: CensusFile) => file.rel.slice("src/".length);

const SCREEN = new RegExp(`^src/(${SCREEN_ROOTS.join("|")})/.*\\.tsx$`);

// A directory-deep file is named too: a walk that stopped at the top level
// clears any floor the flat screens supply.
const MUST_REACH = [
  "features/companies/company360.tsx",
  "components/recordfields.tsx",
  "features/contacts/contactnetwork/edgedetail.tsx",
];

export const rule: CensusRule = {
  name: "cardzones",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        SCREEN.test(file.rel) &&
        !/\.(test|stories|testkit)\.tsx$/.test(file.rel),
    ),
  floors: (corpus) => {
    const paths = new Set(corpus.map(pathOf));
    return [
      ...(corpus.length > 80 ? [] : ["fewer than 80 screen modules were read"]),
      ...MUST_REACH.filter((path) => !paths.has(path)).map(
        (path) => `the census did not reach ${path}`,
      ),
    ];
  },
  across: (corpus) => {
    const found = corpus.flatMap((file) =>
      file.source ? titledCardsIn(pathOf(file), file.source) : [],
    );
    const carriesOne = new Set(found.map((card) => card.where));
    return [
      ...found
        .filter((card) => !notARecordZone.has(card.where))
        .map(
          (card) =>
            `${card.where}:${card.line} <Card title=…> is a titled zone — use Panel (design-system/panel.tsx)`,
        ),
      ...[...notARecordZone.keys()]
        .filter((where) => !carriesOne.has(where))
        .map((where) => `${where}: stale waiver: remove the entry`),
    ];
  },
  advice:
    "A titled Card in a page column is a zone; use Panel " +
    "(design-system/panel.tsx). If the card stands inside another surface " +
    "rather than in a page column, allowlist the file in " +
    "census/rules/cardzones.census.ts with the reason.",
};
