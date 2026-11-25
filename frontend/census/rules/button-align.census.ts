// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A Button's label goes to the start edge through `align="start"` in base.css.
// A screen rule that does the same drifts: `left` for `start`, or text moved
// while the flex content stays centred.
// A rule is a Button's when a compound names `.btn`, a class handed to a
// `<Button>`, or `button`. A compound only before `+` or `~` is a sibling.

import {
  alternativesOf,
  classesOf,
  compounds,
  splitTopLevel,
} from "../../scripts/lib/css-rules";
import { classNamesOn } from "../../src/testing/classnames";
import { rulesIn } from "../../src/testing/css";
import { type CensusFile, type CensusRule, isAppSheet } from "../tree";
import { ownDeclarations, selectorsOf } from "./badge-spelling.census";

const homeSheet = "src/design-system/base.css";

const JUSTIFY_START = new Set(["flex-start", "start", "left"]);
const TEXT_START = new Set(["left", "start"]);

/** Whether one declaration puts content against the start (or left) edge. */
export function startsLabel(property: string, value: string): boolean {
  const words = value
    .toLowerCase()
    .replace(/!\s*important$/, "")
    .split(/\s+/)
    .filter((word) => word !== "" && word !== "safe" && word !== "unsafe");
  switch (property) {
    case "justify-content":
      return words.length === 1 && JUSTIFY_START.has(words[0]);
    // `place-content: <align> <justify>`; one value sets both.
    case "place-content":
      return JUSTIFY_START.has(words.at(-1) ?? "");
    case "text-align":
      return TEXT_START.has(words[0] ?? "");
    default:
      return false;
  }
}

/** The compound with its `:not()` and `:has()` taken out: neither names it. */
function withoutConditions(compound: string): string {
  let out = compound;
  for (let at = out.search(/:(not|has)\(/); at >= 0; ) {
    let depth = 0;
    let end = out.indexOf("(", at);
    for (; end < out.length; end++) {
      depth += out[end] === "(" ? 1 : out[end] === ")" ? -1 : 0;
      if (depth === 0) break;
    }
    out = out.slice(0, at) + out.slice(end + 1);
    at = out.search(/:(not|has)\(/);
  }
  return out;
}

function namesButton(compound: string, buttonClasses: Set<string>): boolean {
  const own = withoutConditions(compound);
  if (/^button(?![\w-])/i.test(own)) return true;
  if ([...classesOf(compound)].some((name) => buttonClasses.has(name))) {
    return true;
  }
  return [...own.matchAll(/\[\s*class\s*[~|^$*]?=\s*["']?([^"'\]]*)/g)].some(
    (match) => match[1].split(/\s+/).some((name) => buttonClasses.has(name)),
  );
}

/** The subject of a complex selector and the compounds it sits inside. */
function subjectAndAncestors(selector: string): string[] {
  const runs = splitTopLevel(selector, "+~");
  return runs.flatMap((run, at) => {
    const chain = compounds(run);
    return at === runs.length - 1 ? chain : chain.slice(0, -1);
  });
}

export function selectsButton(
  selector: string,
  buttonClasses: Set<string>,
): boolean {
  return alternativesOf(selector).some((alternative) =>
    subjectAndAncestors(alternative).some((compound) =>
      namesButton(compound, buttonClasses),
    ),
  );
}

/** `left` is a finding even in BASELINE: it is the wrong edge right to left. */
export type Aligned = { key: string; says: string; left: boolean };

/** Every declaration in one sheet that start-aligns a Button's content. */
export function alignedIn(
  rel: string,
  css: string,
  buttonClasses: Set<string>,
): Aligned[] {
  return rulesIn(css).flatMap((rule) => {
    const declarations = ownDeclarations(rule).filter((one) =>
      startsLabel(one.property, one.value),
    );
    if (declarations.length === 0) return [];
    return selectorsOf(rule)
      .map((selector) => selector.replaceAll(/\s+/g, " "))
      .filter((selector) => selectsButton(selector, buttonClasses))
      .flatMap((selector) =>
        declarations.map((one) => ({
          key: `${rel} ${selector}`,
          says: `${rel}:${one.line} ${selector} { ${one.property}: ${one.value} }`,
          left: /\bleft\b/i.test(one.value),
        })),
      );
  });
}

/** `btn`, and every class the shipped markup hands a `<Button>`. */
export function buttonClassesIn(modules: readonly CensusFile[]): Set<string> {
  return new Set([
    "btn",
    ...modules.flatMap((file) =>
      file.source
        ? classNamesOn(file.source, "Button").map(({ name }) => name)
        : [],
    ),
  ]);
}

/** The findings BASELINE does not excuse, and its entries nothing matches. */
export function judged(
  found: readonly Aligned[],
  baseline: ReadonlyMap<string, string>,
): string[] {
  const keys = new Set(found.map((one) => one.key));
  return [
    ...found
      .filter((one) => one.left || !baseline.has(one.key))
      .map((one) => one.says),
    ...[...baseline.keys()]
      .filter((key) => !keys.has(key))
      .map(
        (key) =>
          `BASELINE excuses ${key}, which no rule start-aligns any more; delete the line`,
      ),
  ];
}

// Rules that keep their own alignment, keyed by sheet and resolved selector.
// It only shrinks: a new entry needs a reason `align="start"` cannot serve.
export const BASELINE = new Map<string, string>([
  [
    "src/design-system/atoms.css .overflow-menu-items .btn",
    "the menu's rows are whatever Buttons a caller hands it; the panel's anatomy sets them as list lines",
  ],
  [
    "src/design-system/select.css .select-button",
    "a trigger whose face and chevron stand apart (space-between), which align would undo",
  ],
  [
    "src/features/records/share.css .share-subject-row",
    "a picker row whose name and level stand apart (space-between), which align would undo",
  ],
  [
    "src/design-system/atoms.css .recordpicker-option",
    "a column whose label is centred on the block axis, which align's justify-content would move",
  ],
  [
    "src/design-system/stageladder.css .stage-ladder-bar-step",
    "the same rule draws a span step and a Button step, a column whose cross axis holds the bar",
  ],
  [
    "src/design-system/stageladder.css .stage-ladder-bar-step.btn",
    "the Button half of the step rule above; align would set the column's block axis",
  ],
  [
    "src/design-system/companycard.css .company-card-name > :is(a, button)",
    "a raw button or link a caller puts in the card's title, drawn as the card's name",
  ],
  [
    "src/features/onboarding/onboarding-conversation/conversation.css .ob-company-card-facts dd button",
    "a raw dotted-underline button inside a fact, which no Button variant draws",
  ],
]);

const shippedModule = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.rel.endsWith(".tsx") &&
  !/\.(test|stories)\.tsx$/.test(file.rel) &&
  !file.rel.includes(".testkit.");

const readingOf = (corpus: readonly CensusFile[]) => {
  const buttonClasses = buttonClassesIn(corpus.filter(shippedModule));
  const sheets = corpus.filter(isAppSheet);
  return {
    buttonClasses,
    sheets,
    found: sheets
      .filter((sheet) => sheet.rel !== homeSheet)
      .flatMap((sheet) => alignedIn(sheet.rel, sheet.text, buttonClasses)),
    home: sheets
      .filter((sheet) => sheet.rel === homeSheet)
      .flatMap((sheet) => alignedIn(sheet.rel, sheet.text, buttonClasses)),
  };
};

export const rule: CensusRule = {
  name: "button-align",
  corpus: (tree) =>
    tree.files.filter((file) => isAppSheet(file) || shippedModule(file)),
  floors: (corpus) => {
    const { buttonClasses, sheets, home } = readingOf(corpus);
    const rules = sheets.reduce(
      (count, sheet) => count + rulesIn(sheet.text).length,
      0,
    );
    return [
      ...(sheets.length > 100 ? [] : ["fewer than 100 stylesheets were read"]),
      ...(rules > 2000 ? [] : ["fewer than 2000 CSS rules were read"]),
      ...(buttonClasses.size > 30
        ? []
        : ["30 or fewer classes were read off a <Button>"]),
      ...(home.length >= 2
        ? []
        : [
            "base.css's own align rule no longer reads as start alignment, so the reader is blind",
          ]),
    ];
  },
  across: (corpus) => judged(readingOf(corpus).found, BASELINE),
  advice:
    'Give the Button `align="start"` and delete the local justify-content / ' +
    "text-align. A rule that must keep its own (a row whose parts stand apart, " +
    "a raw element that is not a Button) goes in BASELINE with its reason.",
};
