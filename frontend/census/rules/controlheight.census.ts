// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A control is one height, and the height is `--controlHeight`.
//
// tokens.css states it: an icon button takes the same height as a verb in a
// row of verbs. This reads the sheets and reports every height a pressable
// thing declares that is not that token. A control gets fixed. Anything else (a
// row, a field, a door with its own touch floor) says so in ACCEPTED below.
//
// The pressable classes are a union of two readings. Classes a rule gives
// `cursor: pointer` reach a class built in a template. Classes the markup puts
// on a `<button>` reach a control with no cursor of its own.

import {
  classesOf,
  selectorList,
  subjectsOf,
} from "../../scripts/lib/css-rules";
import { classNamesOn } from "../../src/testing/classnames";
import { rulesIn } from "../../src/testing/css";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

/**
 * The compound a rule selects, one per comma part and per `:is()` alternative:
 * the last link in the chain. `.row button:not(.rank)` selects a button and
 * excuses `.rank`, which is not the subject.
 */
function subjectsIn(selector: string): string[] {
  return selectorList(selector).flatMap(subjectsOf);
}

/** A rule that draws a `<button>` element itself, rather than a class on one. */
function selectsButton(compound: string): boolean {
  return /^button(?![\w-])/.test(compound);
}

/**
 * The four properties that stand a box at a height. `max-height` is a ceiling
 * and `line-height` is type; the lookbehind keeps both out.
 */
const HEIGHT =
  /(?<![\w-])(min-height|height|min-block-size|block-size)\s*:([^;}]+)/g;

const THE_HEIGHT = "var(--controlHeight)";

/** One height a pressable thing declares, and where. */
type Finding = {
  where: string;
  line: number;
  selector: string;
  says: string;
  keys: string[];
};

/**
 * Every height in one sheet that a pressable thing declares and that is not
 * `--controlHeight`. `keys` is what ACCEPTED is looked up by: the classes the
 * rule selects, plus the selector itself.
 */
export function findingsIn(
  where: string,
  css: string,
  corpus: Set<string>,
): Finding[] {
  const out: Finding[] = [];
  for (const rule of rulesIn(css)) {
    const subjects = subjectsIn(rule.selector);
    const names = subjects.flatMap((subject) => [...classesOf(subject)]);
    const pressable =
      subjects.some(selectsButton) || names.some((name) => corpus.has(name));
    if (!pressable) continue;
    const said = [...rule.body.matchAll(HEIGHT)]
      .filter((found) => found[2].trim() !== THE_HEIGHT)
      .map((found) => `${found[1]}:${found[2].trim()}`);
    if (said.length === 0) continue;
    const selector = rule.selector.replaceAll(/\s+/g, " ");
    out.push({
      where,
      line: rule.line,
      selector,
      says: said.join("; "),
      keys: [...names, selector],
    });
  }
  return out;
}

function saysWhere(finding: Finding): string {
  return `${finding.where}:${finding.line} ${finding.selector} — ${finding.says}`;
}

export const ACCEPTED = new Map<string, string>([
  // Fields. A text box is taller than a verb: a place to put something is not
  // a verb you hit (tokens.css, --inputHeight).
  ["input", "the text field's own box, --inputHeight, and its two flush arms"],
  [
    "select-control",
    "the Select TRIGGER stands in a form column where a text box would, so it " +
      "takes the field's box rather than the control's",
  ],
  [
    "topbar-search",
    "a button drawn as the FIELD the palette opens with, so it wears the " +
      "field's height too (app/topbar.css says so where it is drawn)",
  ],
  [
    ".record-split-verbs .btn:not(.btn-primary)",
    "a verb tile in the record's verb bar stacks a glyph over its word, so it " +
      "is a two-line box that takes an equal share of the bar's width",
  ],
  [
    ".standing-timeline-mark",
    "a glyph chip on the How it stands line: it marks one logged item among " +
      "others a few pixels apart, so it is a mark on an axis and not a verb in a row",
  ],
  // Text, not boxes. The link variant and the affordances that sit inside a
  // running line: a 32px box there sets the height of the line around it.
  [
    "link-button",
    "the text affordance: the quietest rung is not a box at all, and the width " +
      "and height floors come straight back off",
  ],
  ["btn-link", "the same affordance, worn by Button's link variant"],
  ["stat-card-open", "the card's own 'open' link, not a control on it"],
  // Touch floors, not second heights. Each stands at --controlHeight for a
  // mouse and is lifted under `@media (pointer: coarse)`, where WCAG 2.2 AA asks
  // 44px of a thumb. `max()` keeps them a floor.
  [
    ".topbar-trail .iconbtn, .topbar-trail .btn.btn-bare",
    "the top bar's glyphs take a 44px-tall box under a coarse pointer, as wide " +
      "as half the gap to each neighbour allows, and clip their fill to the " +
      "--controlHeight square inside it (app/topbar.css)",
  ],
  [
    ".segmented button",
    "a segment takes the coarse-pointer 44px floor; the strip is --controlHeight " +
      "for a mouse, which is what makes it read as one control",
  ],
  [
    "rail-count-go",
    "a digest chip takes the coarse-pointer 44px floor; it is a line of text for " +
      "a mouse and a target for a thumb",
  ],
  [
    "record-details-toggle",
    "the queue drawer's handle takes the coarse-pointer 44px floor, for the " +
      "reason the chips beside it do",
  ],
  [
    ".worklist-more .btn",
    "the worklist's one load-more verb takes the coarse-pointer 44px floor " +
      "the row verbs beside it get",
  ],
  [
    "button:has(> .visibility-opens)",
    "a visibility mark's button takes the coarse-pointer 44px floor; the pill " +
      "inside it keeps its own height, so a row of marks keeps one line",
  ],
  [
    ".lt-search, .lt-vtrigger, .lt-vpanel :is(.lt-mi, .lt-mback), .lt-vfoot .btn",
    "the list's search field and View button, the panel's rows, its way back " +
      "and its footer verbs take the coarse-pointer 44px floor; at a fine " +
      "pointer they are --controlHeight",
  ],
  [
    "worklist-rank-select",
    "the rank NUMBER made pressable, held at the column's floor so a row " +
      "offering no press keeps its title at the same x",
  ],
  [
    "token-remove",
    "the cross inside a token pill: a box at the control height would grow " +
      "the pill it stands in",
  ],
  // Rows. A list's rhythm belongs to the list rather than to the control
  // vocabulary, and at phone width to the thumb.
  [
    "navitem",
    "a rail ROW: 32 in the column and 48 in the phone bar is the rail's own " +
      "rhythm, and it moves with the rail rather than with the control token",
  ],
  ["railmore", "the same rail row, in its two phone states"],
  ["pageswitch", "the phone header's page switcher, a row rather than a verb"],
  ["palette-row", "a result row, at the phone's thumb floor"],
  ["ob-live-card-head", "a disclosure head — the row IS the control"],
  ["ob-triage-row-summary", "the same shape, one row of a triage list"],
  // Doors and targets. A full-width action owes every pointer 44px, above the
  // control height rather than a second answer to it.
  [
    ".auth-actions .btn",
    "the sign-in door: full width, its own 44+ floor, for the reason " +
      ".btn-federated beside it carries one",
  ],
  ["ob-gate-submit", "the onboarding gate's door, the same floor"],
  [
    "commstatus",
    "a 44px finger target around a mark that stays 18px, so the mark keeps " +
      "its place in the line it sits in",
  ],
  ["commstatus-dense", "the same target, overflowing a short row instead"],
  [
    ".worklist-row button:not(.worklist-rank-select), .worklist-row a.btn, .worklist-row .link-button",
    "the queue's touch floor at phone width, deliberately every pressable " +
      "thing in a row rather than a list of today's verb groups",
  ],
  [
    ".co-lead .today-verb .btn, .co-lead .co-todo .btn, .co-lead .panel-head .btn",
    "the 44px target floor a coarse pointer owes an aimed-at control " +
      "(WCAG 2.5.8), scoped to the needs-you panel's verbs and its head link — " +
      "named one by one rather than as the pane, so the sources under a move's " +
      "reason stay an inline citation run at their own height",
  ],
  [
    "worklist-row-why",
    "the compact row's count, which keeps the row's first line rather than " +
      "setting it",
  ],
  // Not controls at all: a menu row, a card, an area, a hit box.
  [
    ".overflow-menu-items .btn",
    "a Button turned into a menu ROW — the fill, the border and the control " +
      "height all go, and what is left is the Select option's geometry",
  ],
  [
    "compose-choice",
    "a Button turned into a thread ROW of two lines, as tall as its subject " +
      "and its summary need",
  ],
  ["stat-card", "a card: the strip's tile, and the tile filling its cell"],
  ["arhit", "the agent orb's hit area, sized to the orb rather than to a verb"],
  ["fdz", "a drop ZONE is an area to aim at, not a control"],
  ["fdz-input", "the invisible file input covering that zone"],
  ["filterpill", "chip-shaped, and the chip is its own rung on the toolbar"],
  // The filter row's own segments, which fill the row they stand in.
  ["lt-frow-seg", "fills the filter row's height, whatever that row stands at"],
  [
    "lt-frow-more",
    "the same, with a floor under the percentage: the pill is 32 OUTSIDE its " +
      "border, so a segment claiming the control height would push it to 34",
  ],
]);

const inSrc = (file: CensusFile) => file.area === "src";
const sheetsOf = (tree: CensusTree) =>
  tree.files.filter((file) => inSrc(file) && file.rel.endsWith(".css"));

/** Classes some rule's subject gives `cursor: pointer`. */
function pressableClasses(sheets: readonly CensusFile[]): Set<string> {
  const out = new Set<string>();
  for (const sheet of sheets) {
    for (const rule of rulesIn(sheet.text)) {
      if (!/(?<![\w-])cursor\s*:\s*pointer/.test(rule.body)) continue;
      for (const subject of subjectsIn(rule.selector)) {
        for (const name of classesOf(subject)) out.add(name);
      }
    }
  }
  return out;
}

/** Classes the markup puts on a `<button>`, read as a syntax tree. */
function buttonClasses(modules: readonly CensusFile[]): Set<string> {
  return new Set(
    modules.flatMap((file) =>
      file.source
        ? classNamesOn(file.source, "button").map(({ name }) => name)
        : [],
    ),
  );
}

type Reading = {
  pressable: Set<string>;
  buttons: Set<string>;
  findings: Finding[];
};

const readings = new WeakMap<CensusTree, Reading>();

function readingOf(modules: readonly CensusFile[], tree: CensusTree): Reading {
  const known = readings.get(tree);
  if (known) return known;
  const sheets = sheetsOf(tree);
  const pressable = pressableClasses(sheets);
  const buttons = buttonClasses(modules);
  const corpus = new Set([...pressable, ...buttons]);
  const reading = {
    pressable,
    buttons,
    findings: sheets.flatMap((sheet) =>
      findingsIn(sheet.rel, sheet.text, corpus),
    ),
  };
  readings.set(tree, reading);
  return reading;
}

export const rule: CensusRule = {
  name: "controlheight",
  // The modules that draw the app; a story's markup and a test's assertion
  // are not the product putting a class on a control.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        inSrc(file) &&
        file.rel.endsWith(".tsx") &&
        !/\.(test|stories)\.tsx$/.test(file.rel) &&
        !file.rel.includes(".testkit."),
    ),
  floors: (corpus, tree) => {
    const { pressable, buttons } = readingOf(corpus, tree);
    return [
      ...(sheetsOf(tree).length > 150
        ? []
        : ["fewer than 150 stylesheets were read"]),
      ...(corpus.length > 400 ? [] : ["fewer than 400 modules were read"]),
      ...(pressable.size > 60 ? [] : ["60 or fewer classes read as pressable"]),
      ...(buttons.size > 60 ? [] : ["60 or fewer classes read off a <button>"]),
    ];
  },
  across: (corpus, tree) => {
    const { findings } = readingOf(corpus, tree);
    const answered = new Set(findings.flatMap((finding) => finding.keys));
    return [
      ...findings
        .filter((finding) => !finding.keys.some((key) => ACCEPTED.has(key)))
        .map(saysWhere),
      ...[...ACCEPTED.keys()]
        .filter((key) => !answered.has(key))
        .map(
          (key) =>
            `ACCEPTED excuses ${key}, which nothing declares any more; delete the line`,
        ),
    ];
  },
  advice:
    "A control is --controlHeight; anything else says what it is in ACCEPTED " +
    "(census/rules/controlheight.census.ts), which only shrinks.",
};
