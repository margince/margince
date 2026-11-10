// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the chip fill is drawn under, derived rather than listed.
//
// tokens.test.ts measures each ink in chipInks over --bgChip. This rule finds
// every rule under src/ that paints --bgChip, every rule that draws inside one,
// and the ink each sets. An unmeasured ink reads as an ordinary declaration,
// and no pair says what it comes out at over the fill.
//
// A rule is in scope when a compound of its selector carries every class of the
// chip's subject: `.segmented button` inks what renders on `.segmented`'s
// track. A class name is a token, not a prefix. A rule that paints a ground of
// its own answers to that ground.
//
// The components say who renders inside whom: `.token-remove` names no token
// class, and TokenInput renders it inside one. They also say which elements
// draw text, for the chip that names no ink and inherits its row's.

import { chipInks } from "../../scripts/lib/chip-inks";
import {
  alternativesOf,
  classesOf,
  compounds,
  inks,
  type Rule,
  rulesOfText,
  subjectClasses,
  subjectOf,
} from "../../scripts/lib/css-rules";
import {
  type DrawnElement,
  drawnElements,
  landsOn,
  overriddenInside,
  renderedInside,
} from "../../scripts/lib/rendered-inside";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

type Reading = {
  rules: Rule[];
  chips: Rule[];
  inside: Map<string, Set<string>[]>;
  drawn: DrawnElement[];
};

const inSrc = (file: CensusFile) => file.area === "src";

// Floors and findings both read the components; the tree does not change.
const readings = new WeakMap<CensusTree, Reading>();

function readingOf(tree: CensusTree): Reading {
  const known = readings.get(tree);
  if (known) return known;
  const rules = tree.files
    .filter((file) => inSrc(file) && file.rel.endsWith(".css"))
    .flatMap((file) => rulesOfText(file.rel, file.text));
  const components = tree.files.flatMap((file) =>
    inSrc(file) && file.rel.endsWith(".tsx") && file.source
      ? [file.source]
      : [],
  );
  const reading = {
    rules,
    chips: rules.filter(({ body }) =>
      /background(?:-color)?:[^;]*var\(--bgChip\)/.test(body),
    ),
    inside: renderedInside(components),
    drawn: drawnElements(components),
  };
  readings.set(tree, reading);
  return reading;
}

// WCAG 1.4.3 exempts an inactive control, which a disabled segment is.
function isDisabledState(selector: string): boolean {
  return /:disabled|\[disabled\]|\[aria-disabled="true"\]/.test(selector);
}

// A chip written `:is(.a, .b)` is two chips, each searched on its own subject.
function chipSubjects(chips: Rule[]) {
  return chips.flatMap((chip) =>
    alternativesOf(chip.selector).map((selector) => ({
      chip,
      selector,
      wanted: subjectClasses(selector),
    })),
  );
}

export function chainsNaming(
  selector: string,
  wanted: Set<string>,
): string[][] {
  return alternativesOf(selector)
    .map(compounds)
    .filter((parts) =>
      parts.some((part) =>
        [...wanted].every((name) => classesOf(part).has(name)),
      ),
    );
}

function paintsOwnGround(body: string): boolean {
  return /background(?:-color)?:(?![^;]*var\(--bgChip\))[^;]*(?:var\(|#|rgb)/.test(
    body,
  );
}

/** Inks drawn on a chip's fill that no contrast pair measures. */
function unmeasuredInks({ rules, chips, inside }: Reading): string[] {
  const offenders: string[] = [];
  for (const { chip, wanted } of chipSubjects(chips)) {
    // A subject with no class of its own is reached through the track's rule.
    if (wanted.size === 0) continue;
    // Inside by selector, or inside by the components.
    const below = [...wanted].flatMap((name) => inside.get(name) ?? []);
    const subtree = rules.filter((rule) => {
      if (rule === chip) return false;
      if (chainsNaming(rule.selector, wanted).length > 0) return true;
      return below.some((element) => landsOn(rule, element, below));
    });
    for (const rule of [chip, ...subtree]) {
      if (isDisabledState(rule.selector)) continue;
      if (rule !== chip && paintsOwnGround(rule.body)) continue;
      // A rule naming both the chip and this element sets the ink that lands;
      // the generic rule beneath it inks the element elsewhere.
      if (rule !== chip && overriddenInside(rules, chip, rule, inside))
        continue;
      for (const ink of inks(rule.body)) {
        if (chipInks.includes(ink)) continue;
        offenders.push(
          `${rule.file}: ${rule.selector} draws on ${chip.selector}'s --bgChip in ${ink}, which no pair measures`,
        );
      }
    }
  }
  return offenders;
}

// A type in the selector must be the element's own: `button.foo` inks no
// `<span className="foo">`.
const tagOf = (compound: string) => /^[a-z][\w-]*/.exec(compound)?.[0];
const stateOf = (compound: string) =>
  compound.replace(/\.[\w-]+/g, "").replace(/^[a-z][\w-]*/, "");

/**
 * Elements on the fill that name no ink of their own. Their own rule, a base,
 * or a rule spelled as the chip's counts. A contextual rule does not:
 * `.overflow-menu-items .btn` inks a button in a menu. A track drawing no text
 * is exempt.
 */
function inheritedInks({ rules, chips, drawn }: Reading): string[] {
  const inked = rules.filter((rule) => inks(rule.body).length > 0);
  const names = (chipSelector: string, element: DrawnElement) =>
    inked.some((rule) =>
      alternativesOf(rule.selector).some((selector) => {
        if (selector === chipSelector) return true;
        const parts = compounds(selector);
        const compound = parts[0] ?? "";
        const own = classesOf(compound);
        const tag = tagOf(compound);
        return (
          parts.length === 1 &&
          own.size > 0 &&
          [...own].every((name) => element.classes.has(name)) &&
          (tag === undefined || tag === element.tag) &&
          ["", stateOf(subjectOf(chipSelector))].includes(stateOf(compound))
        );
      }),
    );
  const offenders: string[] = [];
  for (const { chip, selector, wanted } of chipSubjects(chips)) {
    if (wanted.size === 0 || isDisabledState(selector)) continue;
    if (inks(chip.body).length > 0) continue;
    const carriers = drawn.filter(({ classes }) =>
      [...wanted].every((name) => classes.has(name)),
    );
    // A class only ever computed is held to its own name.
    const read: DrawnElement[] =
      carriers.length > 0
        ? carriers.filter(({ textless }) => !textless)
        : [{ classes: wanted, tag: undefined, textless: false }];
    for (const element of read) {
      if (names(selector, element)) continue;
      offenders.push(
        `${chip.file}: ${selector} paints --bgChip under [${[...element.classes].join(" ")}], which names no ink and reads in whatever its row is drawn in`,
      );
    }
  }
  return offenders;
}

/**
 * --bgChip painted inside something already painted in it. The two composite,
 * and every pair above measures the wrong ground. A longer chain is a
 * descendant; the same length is the same element in another state.
 */
function stackedFills({ chips, inside }: Reading): string[] {
  const offenders: string[] = [];
  for (const { chip, selector, wanted } of chipSubjects(chips)) {
    if (wanted.size === 0) continue;
    const below = [...wanted].flatMap((name) => inside.get(name) ?? []);
    for (const rule of chips) {
      if (rule === chip) continue;
      const chained = chainsNaming(rule.selector, wanted).some(
        (parts) => parts.length > compounds(selector).length,
      );
      if (chained || below.some((element) => landsOn(rule, element, below))) {
        offenders.push(
          `${rule.file}: ${rule.selector} paints --bgChip inside ${chip.selector}, which already does`,
        );
      }
    }
  }
  return offenders;
}

// The readers, falsified on the tree: a reader that stopped finding anything
// would leave every scan passing.
function readerFloors({ chips, inside, drawn }: Reading): string[] {
  const carrying = (name: string) =>
    drawn.filter(({ classes }) => classes.has(name));
  const rail = (element: Set<string>) => element.has("pe-rail-label");
  const svgs = drawn.filter(({ tag }) => tag === "svg");
  const checks: [boolean, string][] = [
    [chips.length > 0, "no rule paints --bgChip"],
    [inside.size > 0, "the containment map is empty"],
    [drawn.length > 0, "no drawn element was read"],
    [
      (inside.get("token") ?? []).some((element) =>
        element.has("token-remove"),
      ),
      "TokenInput renders .token-remove inside .token; the reader did not see it",
    ],
    [
      [...inside.values()].some((below) => below.some(rail)),
      "the contact rail renders .pe-rail-label inside a .pe-rail-row; the reader did not see it",
    ],
    [
      !(inside.get("pn-relay-owner") ?? []).some(rail),
      "containment chained across components, which reports rules against chips they never land on",
    ],
    [
      carrying("progressbar").length > 0 &&
        carrying("progressbar").every(({ textless }) => textless),
      "a .progressbar is not read as a textless track",
    ],
    [
      carrying("ai-latency-track").length > 0 &&
        carrying("ai-latency-track").every(({ textless }) => textless),
      "an .ai-latency-track is not read as a textless track",
    ],
    [
      carrying("token").some(({ textless }) => !textless),
      "no .token is read as drawing text",
    ],
    [
      carrying("token-standalone").length > 0 &&
        carrying("token-standalone").every(({ classes }) =>
          classes.has("token"),
        ),
      "a .token-standalone is not read beside its .token base",
    ],
    [
      svgs.length > 0 && svgs.every(({ textless }) => !textless),
      "an <svg> is read as textless, though an icon draws in the ink it inherits",
    ],
    [
      carrying("token").every(({ tag }) => tag !== undefined),
      "a .token element is read without its tag",
    ],
  ];
  return checks.flatMap(([holds, why]) => (holds ? [] : [why]));
}

export const rule: CensusRule = {
  name: "chipfill",
  corpus: (tree) =>
    tree.files.filter((file) => inSrc(file) && file.rel.endsWith(".css")),
  floors: (_corpus, tree) => readerFloors(readingOf(tree)),
  across: (_corpus, tree) => {
    const reading = readingOf(tree);
    return [
      ...new Set([
        ...unmeasuredInks(reading),
        ...inheritedInks(reading),
        ...stackedFills(reading),
      ]),
    ];
  },
  advice:
    "Every ink drawn on --bgChip is one the contrast gate measures " +
    "(scripts/lib/chip-inks.ts and tokens.test.ts). A chip names the ink of " +
    "what it draws, and --bgChip is never painted inside something already " +
    "painted in it; a pressed state takes a ground from the ladder instead.",
};
