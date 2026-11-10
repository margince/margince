// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A disclosure nested inside a disclosure keeps the atom's body.
//
// A screen that draws its sections as `Disclosure`s gives each section's body
// the pane's inset. Spelt as a descendant (`.rail .sect .disclosure-body`), it
// also reaches every disclosure nested in the section. That one pays it twice,
// and its rows start right of the rows above. Scoped with `>`, it stops.
//
// It reads what the sheets declare, not paint, and only the descendant
// `.disclosure-body` shape. An inset put on the nested disclosure by another
// selector is not seen.

import { selectorList } from "../../scripts/lib/css-rules";
import { withoutComments } from "../../src/testing/css";
import type { CensusFile, CensusRule } from "../tree";

/** One selector mentioning `.disclosure-body`, and where it is written. */
export type Mention = Readonly<{ sheet: string; selector: string }>;

// Selector preludes, one per comma, comments blanked first.
export function mentionsIn(sheet: string, css: string): Mention[] {
  return [...withoutComments(css).matchAll(/([^{}]+)\{[^{}]*\}/g)].flatMap(
    (rule) =>
      selectorList(rule[1])
        .map((one) => one.replace(/\s+/g, " "))
        .filter((one) => one.includes(".disclosure-body"))
        .map((selector) => ({ sheet, selector })),
  );
}

// The atom's own rule names the body with nothing to its left.
const isAtomRule = (one: Mention) => /^\.disclosure-body\b/.test(one.selector);

const mentionsOf = (corpus: readonly CensusFile[]) =>
  corpus.flatMap((file) => mentionsIn(file.rel, file.text));

export const rule: CensusRule = {
  name: "disclosure-inset",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.rel.endsWith(".css"),
    ),
  floors: (corpus) => {
    const mentions = mentionsOf(corpus);
    return [
      ...(corpus.length > 20 ? [] : ["fewer than 20 stylesheets were read"]),
      ...(mentions.some(isAtomRule)
        ? []
        : ["the scan did not find the atom's own `.disclosure-body` rule"]),
      ...(mentions.filter((one) => !isAtomRule(one)).length >= 2
        ? []
        : ["fewer than two scoped `.disclosure-body` selectors were read"]),
    ];
  },
  across: (corpus) =>
    mentionsOf(corpus)
      .filter((one) => !isAtomRule(one))
      .filter((one) => !/>\s*\.disclosure-body\b/.test(one.selector))
      .map(
        (one) => `${one.sheet}: \`${one.selector}\` insets the body by descent`,
      ),
  advice:
    "A section's inset scoped to `.disclosure-body` by descent reaches every " +
    "disclosure nested inside it, whose rows then start right of the rows " +
    "above them. Scope it with `>`.",
};
