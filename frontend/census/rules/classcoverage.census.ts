// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A class on an element matches a rule somewhere, or it is a lie.
//
// A class name reads as intent, and a selector nobody wrote looks the same in
// review as one that is there. A class no sheet declares draws nothing, so the
// page draws something other than what the markup says.
//
// The other direction: a browser suite that walks a class no sheet declares
// makes the page carry that name for the suite's sake. Such a handle is a
// `data-testid`.
//
// The markup is read as a syntax tree, so a class name inside a comment is not
// a class name.

import ts from "typescript";
import { stable } from "../../src/format/collate";
import { classNamesOn } from "../../src/testing/classnames";
import { withoutComments } from "../../src/testing/css";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

export function selectorClassesIn(source: ts.SourceFile): string[] {
  const out: string[] = [];
  // `openEnded`: an interpolation follows, so a name running to the end is half
  // a name. `` `.row-${tone}` `` names no class this can look up.
  const read = (text: string, openEnded = false) => {
    for (const found of text.matchAll(/\.([a-z][\w-]*)/g)) {
      if (openEnded && (found.index ?? 0) + found[0].length === text.length) {
        continue;
      }
      out.push(found[1]);
    }
  };
  // A string in selector position is read whatever it looks like; one anywhere
  // else has to look like a selector.
  const readSelector = (node: ts.Node, positional: boolean) => {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      if (positional || looksLikeSelector(node.text)) {
        read(node.text);
      }
      return;
    }
    if (ts.isTemplateExpression(node)) {
      if (!positional && !looksLikeSelector(node.head.text)) {
        return;
      }
      read(node.head.text, true);
      node.templateSpans.forEach((span, index) => {
        read(span.literal.text, index < node.templateSpans.length - 1);
      });
    }
  };
  const visit = (node: ts.Node) => {
    if (ts.isCallExpression(node) && node.arguments.length > 0) {
      const callee = ts.isPropertyAccessExpression(node.expression)
        ? node.expression.name.text
        : node.expression.getText(source);
      if (SELECTOR_CALLS.has(callee)) {
        for (const argument of node.arguments) {
          readSelector(argument, true);
        }
      }
    }
    readSelector(node, false);
    ts.forEachChild(node, visit);
  };
  visit(source);
  // A string in selector position that also looks like one is read twice.
  return [...new Set(out)];
}

/**
 * The calls that take a CSS selector: Playwright's, the DOM's, and the helpers
 * this tree's suites wrap them in. The shape rule reads a selector handed to
 * any other call.
 */
const SELECTOR_CALLS = new Set([
  "locator",
  "querySelector",
  "querySelectorAll",
  "closest",
  "matches",
  "$",
  "$$",
  "$eval",
  "$$eval",
  "present",
  "edge",
  "topOf",
]);

/**
 * A string that is nothing but selector: punctuation, names, and spaces. An
 * `accept` list such as `".txt,.md,.srt"` fits the charset, so a selector also
 * carries a character only a selector uses, or a hyphenated class name.
 */
const SELECTOR_SHAPE = /^[.#[][\w.#[\]='"\s>+~:,()-]*$/;
const SELECTOR_ONLY = /[#[\]>+~:\s]|\.[a-z]\w*-/;

function looksLikeSelector(text: string): boolean {
  return SELECTOR_SHAPE.test(text) && SELECTOR_ONLY.test(text);
}

export function declaredIn(css: string): Set<string> {
  const out = new Set<string>();
  for (const chunk of withoutComments(css).split("{")) {
    // The selector only: the text after the previous rule closed. A body holds
    // dotted text of its own, and `url("./icons/a.svg")` would declare `icons`.
    const selector = chunk.slice(chunk.lastIndexOf("}") + 1);
    for (const found of selector.matchAll(/\.(-?[_a-zA-Z][\w-]*)/g)) {
      out.add(found[1]);
    }
  }
  return out;
}

/** One class name a module puts on an element, and where. */
type Rendered = { name: string; where: string; line: number };

/**
 * Each class name a module renders, with its line. The reading is shared with
 * the control-height spec in src/testing/classnames.ts, so two readers cannot
 * drift.
 */
export function renderedIn(source: ts.SourceFile, where: string): Rendered[] {
  return classNamesOn(source).map(({ name, at }) => ({
    name,
    where,
    line: source.getLineAndCharacterOfPosition(at.getStart(source)).line + 1,
  }));
}

/** A token that reads as a class name rather than as a word of copy. */
function isClassToken(name: string): boolean {
  return /^-?[_a-zA-Z][\w-]*$/.test(name);
}

/**
 * Suite selectors a test means to walk though nothing declares them. It only
 * shrinks.
 */
const onPurpose = new Map<string, string>([
  [
    "e2e/contact-network.spec.ts walks .pn-ring",
    "the retired drawing, asserted absent: `toHaveCount(0)` over classes nothing declares is the point of that case",
  ],
  ["e2e/contact-network.spec.ts walks .pn-node", "the same retired drawing"],
  ["e2e/contact-network.spec.ts walks .pn-edge", "the same retired drawing"],
]);

const inLayers = (file: CensusFile) =>
  file.area === "src" || file.area === "extensions";

// Every stylesheet the app loads, core and extension tier alike. A sheet the
// walk misses makes every class it declares look orphaned.
const sheetsOf = (tree: CensusTree) =>
  tree.files.filter((file) => inLayers(file) && file.rel.endsWith(".css"));

const suitesOf = (tree: CensusTree) =>
  tree.files.filter((file) => file.area === "e2e" && file.rel.endsWith(".ts"));

const declaredByTree = new WeakMap<CensusTree, Set<string>>();

function declaredOf(tree: CensusTree): Set<string> {
  const known = declaredByTree.get(tree);
  if (known) return known;
  const declared = new Set(
    sheetsOf(tree).flatMap((file) => [...declaredIn(file.text)]),
  );
  declaredByTree.set(tree, declared);
  return declared;
}

export const rule: CensusRule = {
  name: "classcoverage",
  // The modules that draw the app. A class named in a test is an assertion,
  // and a story's markup is scaffolding for one frame.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        inLayers(file) &&
        file.rel.endsWith(".tsx") &&
        !/\.(test|stories)\.tsx$/.test(file.rel) &&
        !file.rel.includes(".testkit."),
    ),
  // No sheets makes everything an orphan, and no modules makes nothing one.
  floors: (corpus, tree) => [
    ...(sheetsOf(tree).length > 100
      ? []
      : [
          "fewer than 100 stylesheets were read, so classes would read as orphaned",
        ]),
    ...(corpus.length > 300
      ? []
      : [
          "fewer than 300 modules were read, so few classes would be read at all",
        ]),
    ...(declaredOf(tree).size > 1000
      ? []
      : [
          "the sheets parsed to almost no class names, so this gate reads them wrong",
        ]),
    ...(suitesOf(tree).length > 5
      ? []
      : ["no browser suites were found, so the suite arm read nothing"]),
  ],
  across: (corpus, tree) => {
    const declared = declaredOf(tree);
    const orphans = corpus.flatMap((file) =>
      file.source
        ? renderedIn(file.source, file.rel)
            .filter(
              (rendered) =>
                isClassToken(rendered.name) && !declared.has(rendered.name),
            )
            .map(
              (rendered) =>
                `${rendered.where}:${rendered.line} .${rendered.name} is matched by no selector in any stylesheet`,
            )
        : [],
    );
    const pinned = suitesOf(tree).flatMap((file) =>
      file.source
        ? selectorClassesIn(file.source)
            .map((name) => ({ name, walked: `${file.rel} walks .${name}` }))
            .filter(
              ({ name, walked }) =>
                !declared.has(name) && !onPurpose.has(walked),
            )
            .map(
              ({ walked }) =>
                `${walked}, which no stylesheet declares; give the element a data-testid and walk that`,
            )
        : [],
    );
    return [...new Set([...orphans, ...pinned])].sort(stable);
  },
  advice:
    "A class put on an element and matched by no selector is a rule that " +
    "should exist and does not, or a name that should be deleted. A suite that " +
    "walks a class no sheet declares makes the page carry the name for the " +
    "suite's sake: give the element a data-testid and walk that instead.",
};
