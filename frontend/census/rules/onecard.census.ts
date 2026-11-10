// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// `Card` is the one card surface, and a hand-rolled one is a second card the
// moment any of its chrome moves.
//
// So the rule is derived from the card. Any rule that declares `.card`'s own
// chrome (its token values plus the card shadow) is a second card, whatever it
// is called. The design system is judged too, minus the rule the subject is
// read from.
//
// A variant of the card is a design decision, not a duplicate a test can name.
// "A border, a radius and a padded background" fits dozens of chips and plates.
// So this asks only whether a rule is the card surface, copied.

import { join } from "node:path";
import { withoutComments } from "../../src/testing/css";
import { type CensusFile, type CensusRule, isAppSheet, srcDir } from "../tree";

const designSystem = join(srcDir, "design-system");
// The one rule the subject is read from, so the one rule not reported as a
// copy of itself.
export const CARD_SOURCE = join(designSystem, "atoms.css");

/** One `selector { … }` rule, as written. */
type Rule = Readonly<{ selector: string; body: string }>;

// Innermost braces only, so the rules inside a media query are read as
// themselves and the query is never read as a rule.
function rules(css: string): Rule[] {
  return [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)].map((match) => ({
    selector: match[1].trim(),
    body: match[2],
  }));
}

/**
 * `.card`'s own declarations, read from the design system. A second `.card`
 * rule in atoms.css would make the subject ambiguous, so it must be one.
 */
export function cardChrome(atoms: string): readonly string[] | string {
  const declared = rules(withoutComments(atoms)).filter(
    (rule) => rule.selector === ".card",
  );
  if (declared.length !== 1) {
    return `atoms.css declares .card ${declared.length} times; this rule needs one to read its subject from`;
  }
  return declarations(declared[0].body);
}

/** The `property: value` pairs in a rule body, in one spelling. */
function declarations(body: string): readonly string[] {
  return body
    .split(";")
    .map((part) =>
      part
        .trim()
        .replace(/\s+/g, " ")
        // A formatter erases these differences, and this rule exists for the
        // hand that does not run it.
        .replace(/\s*:\s*/g, ": ")
        .replace(/\(\s+/g, "(")
        .replace(/\s+\)/g, ")"),
    )
    .filter((part) => part.length > 0)
    .flatMap(longhand);
}

/**
 * The two shorthands `.card` uses, expanded in the one form each takes there.
 * A retyped copy often writes `background-color` where the source says
 * `background`. Both sides run through here, so an unknown shorthand still
 * compares against itself.
 */
function longhand(declaration: string): readonly string[] {
  const colon = declaration.indexOf(": ");
  if (colon < 0) {
    return [declaration];
  }
  const property = declaration.slice(0, colon);
  const value = declaration.slice(colon + 2);
  if (property === "background" && !value.includes(" ")) {
    return [`background-color: ${value}`];
  }
  const border = /^(\S+) (\S+) (\S+)$/.exec(value);
  if (property === "border" && border !== null) {
    return [
      `border-width: ${border[1]}`,
      `border-style: ${border[2]}`,
      `border-color: ${border[3]}`,
    ];
  }
  return [declaration];
}

/** Every rule in one sheet that declares the whole card chrome. */
export function secondCards(
  sheet: CensusFile,
  chrome: readonly string[],
): string[] {
  return rules(withoutComments(sheet.text))
    .filter(
      (rule) => !(sheet.path === CARD_SOURCE && rule.selector === ".card"),
    )
    .filter((rule) => {
      const declared = new Set(declarations(rule.body));
      return chrome.every((property) => declared.has(property));
    })
    .map((rule) => `${sheet.rel}: ${rule.selector}`);
}

const chromeIn = (atoms: CensusFile | undefined) =>
  atoms ? cardChrome(atoms.text) : "atoms.css is not in the tree";

export const rule: CensusRule = {
  name: "onecard",
  corpus: (tree) => tree.files.filter(isAppSheet),
  // A census of zero over a clean tree reads the same as a broken walk. So the
  // floor names a file in each place this rule looks for a second card.
  floors: (corpus, tree) => {
    const chrome = chromeIn(tree.at(CARD_SOURCE));
    return [
      ...(typeof chrome === "string" ? [chrome] : []),
      ...(corpus.length > 40 ? [] : ["40 or fewer stylesheets were read"]),
      ...(corpus.some((file) =>
        file.path.endsWith("features/onboarding/onboarding.css"),
      )
        ? []
        : [
            "the walk did not reach src/features/, where every card this rule was written for lives",
          ]),
      ...(corpus.some(
        (file) =>
          file.path.startsWith(designSystem) && file.path !== CARD_SOURCE,
      )
        ? []
        : [
            "the walk did not reach the design system's other stylesheets, where a second card sits beside the real one",
          ]),
    ];
  },
  across: (corpus, tree) => {
    const chrome = chromeIn(tree.at(CARD_SOURCE));
    if (typeof chrome === "string") return [chrome];
    return corpus.flatMap((sheet) => secondCards(sheet, chrome));
  },
  advice:
    "these rules draw a second card surface. `Card` is the " +
    "one card — `as` picks the element, `className` still carries whatever " +
    "content styling the screen owns. A copy drifts silently: the two cards " +
    "this replaced had identical chrome and text rules that had already " +
    "disagreed, and nobody chose the difference.",
};
