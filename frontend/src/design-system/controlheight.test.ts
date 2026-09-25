// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { filesMatching, parseSource } from "../../scripts/lib/source-tree";
import { classNamesOn } from "../testing/classnames";
import { rulesIn } from "../testing/css";

// A CONTROL IS ONE HEIGHT, AND THE HEIGHT IS `--controlHeight`.
//
// tokens.css states it: there is no small rung any more, an icon button takes
// the same height as a verb in a row of verbs, and a control that changed size
// under a touchscreen was one more thing that had to be remembered on every
// surface and was not. `.btn` and `.iconbtn` wear it; the rungs that drifted
// away from it were spelled in four different ways — `height: 26px`,
// `min-block-size: var(--space-6)`, `block-size: 2rem`, and a `padding: 5px`
// that arrived at 32 by arithmetic nobody could check.
//
// So this reads the SHEETS and reports every height a pressable thing in the
// tree declares that is not that token. Each one is either a control, and gets
// fixed, or it is something else — a row, a card, a field, a link, a door with
// its own touch floor — and says so in ACCEPTED below, once, in words.
//
// Under-recognition is the way a census like this fails without failing: read a
// narrower corpus, find nothing, report PASS. So the corpus is a UNION of two
// readings that cover each other's holes, floored below so a walk that found a
// corner of the tree fails instead of passing, and the register may only shrink.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const sourceRoot = join(frontendRoot, "src");

function fromFrontend(path: string): string {
  return relative(frontendRoot, path).replaceAll("\\", "/");
}

/** Every stylesheet in the app, core tier and screens alike. */
function sheets(): string[] {
  return filesMatching(sourceRoot, /\.css$/).map(fromFrontend);
}

/**
 * The modules that draw the app. A story's fixture markup and a test's
 * assertion are not the product putting a class on a control.
 */
function modules(): string[] {
  return filesMatching(sourceRoot, /\.tsx$/)
    .map(fromFrontend)
    .filter((where) => !/\.(test|stories)\.tsx$/.test(where))
    .filter((where) => !where.includes(".testkit."));
}

function read(where: string): string {
  return readFileSync(join(frontendRoot, where), "utf8");
}

/**
 * The compound a rule SELECTS, one per comma part: the last link in the chain,
 * with `:not()` blanked first. `.worklist-row button:not(.worklist-rank-select)`
 * selects a button and excuses one class — reading the excused class as the
 * subject would file the rule against the very thing it leaves alone.
 */
function subjectsOf(selector: string): string[] {
  return selector
    .split(",")
    .map((part) => part.replaceAll(/:not\([^)]*\)/g, " ").trim())
    .map(
      (part) =>
        part
          .split(/[\s>+~]+/)
          .filter(Boolean)
          .pop() ?? "",
    )
    .filter(Boolean);
}

function classesIn(compound: string): string[] {
  return [...compound.matchAll(/\.(-?[_a-zA-Z][\w-]*)/g)].map(
    (found) => found[1],
  );
}

/** A rule that draws a `<button>` element itself, rather than a class on one. */
function selectsButton(compound: string): boolean {
  return /^button(?![\w-])/.test(compound);
}

/**
 * The four properties that stand a box at a height. `max-height` is a ceiling
 * and `line-height` is type, and the lookbehind is what keeps both out.
 */
const HEIGHT =
  /(?<![\w-])(min-height|height|min-block-size|block-size)\s*:([^;}]+)/g;

const THE_HEIGHT = "var(--controlHeight)";

/**
 * A class this tree makes PRESSABLE, read from the sheets: some rule whose
 * subject names it says `cursor: pointer`.
 *
 * This is the reading that covers what the markup cannot say. A class assembled
 * from a template — `.btn-${variant}` — or joined in a helper is invisible to
 * an AST walk of the markup, and three of the controls below (`.railmore`,
 * `.calendar-day`, `.probe-button`) arrive only this way.
 */
function pressableClasses(): Set<string> {
  const out = new Set<string>();
  for (const where of sheets()) {
    for (const rule of rulesIn(read(where))) {
      if (!/(?<![\w-])cursor\s*:\s*pointer/.test(rule.body)) continue;
      for (const subject of subjectsOf(rule.selector)) {
        for (const name of classesIn(subject)) {
          out.add(name);
        }
      }
    }
  }
  return out;
}

/**
 * A class the markup puts on a `<button>`, read as a syntax tree.
 *
 * The reading that covers what the sheets cannot say: a control drawn with no
 * `cursor` of its own — it inherits one, or the rule lives under a parent —
 * is a button all the same, and `.acctrow`, `.lt-vtab` and the onboarding
 * cards arrive here rather than above.
 */
function buttonClasses(): Set<string> {
  const out = new Set<string>();
  for (const where of modules()) {
    const source = parseSource(where, read(where));
    for (const { name } of classNamesOn(source, "button")) {
      out.add(name);
    }
  }
  return out;
}

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
 * `--controlHeight`.
 *
 * `keys` is what ACCEPTED is looked up by: the classes the rule SELECTS, plus
 * the selector itself for a rule that draws the `button` element or that
 * excuses one place a shared class stands.
 */
function findingsIn(
  where: string,
  css: string,
  corpus: Set<string>,
): Finding[] {
  const out: Finding[] = [];
  for (const rule of rulesIn(css)) {
    const subjects = subjectsOf(rule.selector);
    const names = subjects.flatMap(classesIn);
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

function findings(corpus: Set<string>): Finding[] {
  return sheets().flatMap((where) => findingsIn(where, read(where), corpus));
}

function saysWhere(finding: Finding): string {
  return `${finding.where}:${finding.line} ${finding.selector} — ${finding.says}`;
}

// EVERY SECOND HEIGHT IN THE TREE, AND WHY IT IS NOT A CONTROL'S.
//
// Keyed by what the rule selects — the class, or the whole selector where the
// thing selected is the `button` element or one placement of a shared class.
// The reason is the point: a line here says what the box IS, so the next reader
// can tell a field from a control from a row without measuring anything.
//
// It only shrinks. A key that stops answering a finding fails below, so a rung
// this list excuses cannot outlive the rule it was written for.
const ACCEPTED = new Map<string, string>([
  // Fields. A text box is deliberately taller than a verb: a place to put
  // something is not a verb you hit (tokens.css, --inputHeight).
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
  // Text, not boxes. The link variant and the affordances that sit inside a
  // running line: a 32px box there sets the height of the line around it.
  [
    "link-button",
    "the text affordance: the quietest rung is not a box at all, and the width " +
      "and height floors come straight back off",
  ],
  ["btn-link", "the same affordance, worn by Button's link variant"],
  [
    "explain-toggle",
    "sits INSIDE a line of running figures; the min-* pair is what holds it " +
      "under the .iconbtn floor it is drawn beside",
  ],
  ["stat-card-open", "the card's own 'open' link, not a control on it"],
  // Touch FLOORS, not second heights. Each of these stands at --controlHeight
  // for a mouse and is lifted only under `@media (pointer: coarse)`, where WCAG
  // 2.2 AA asks 44px of a thumb. `max()` is what keeps them a floor: a later
  // rise in the shared height passes straight through.
  [
    "user",
    "the account chip takes the coarse-pointer 44px floor; at a fine pointer it " +
      "is --controlHeight like every other control",
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
    "worklist-rank-select",
    "the rank NUMBER made pressable, held at the column's floor so a row " +
      "offering no press keeps its title at the same x",
  ],
  [
    "token-remove",
    "the cross inside a token pill: a box at the control height would grow " +
      "the pill it stands in",
  ],
  // Rows. A list's rhythm, which belongs to the list rather than to the
  // control vocabulary — and at phone width to the thumb.
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
  // Doors and targets. A full-width action owes every pointer 44px, which is
  // above the control height rather than a second answer to it.
  [
    ".auth-actions .btn",
    "the sign-in door: full width, its own 44+ floor, for the reason " +
      ".btn-federated beside it carries one",
  ],
  ["ob-gate-submit", "the onboarding gate's door, the same floor"],
  ["probe-button", "the standalone MCP view's one action, sized for a thumb"],
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
  ["stat-card", "a card: the strip's tile, and the tile filling its cell"],
  ["arhit", "the agent orb's hit area, sized to the orb rather than to a verb"],
  [
    "avatar",
    "a contact's MARK inside something pressable — a picture sized to the row " +
      "it stands in, and nothing anybody aims at on its own",
  ],
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

// The three gates below each read every stylesheet and every module in the
// tree. Synchronous file I/O, not a unit's work, and past 10s on CI's coverage
// run; nothing here can hang, so the budget is only a floor under a slow runner.
describe("one control height", { timeout: 60_000 }, () => {
  // THE FLOORS. Everything below reads a corpus, and a corpus that came back
  // empty would report the same word as a clean tree.
  it("reads the whole tree rather than a corner of it", () => {
    expect(sheets().length).toBeGreaterThan(150);
    expect(modules().length).toBeGreaterThan(400);
    expect(pressableClasses().size).toBeGreaterThan(60);
    expect(buttonClasses().size).toBeGreaterThan(60);
  });

  it("stands every pressable box at --controlHeight, or ACCEPTED says why", () => {
    const corpus = new Set([...pressableClasses(), ...buttonClasses()]);
    const unaccounted = findings(corpus).filter(
      (finding) => !finding.keys.some((key) => ACCEPTED.has(key)),
    );
    expect(
      unaccounted.map(saysWhere),
      "a control is --controlHeight; anything else says what it is in ACCEPTED",
    ).toEqual([]);
  });

  it("keeps ACCEPTED to what it still excuses", () => {
    const corpus = new Set([...pressableClasses(), ...buttonClasses()]);
    const answered = new Set(
      findings(corpus).flatMap((finding) => finding.keys),
    );
    expect(
      [...ACCEPTED.keys()].filter((key) => !answered.has(key)),
      "these excuse nothing any more — delete the line",
    ).toEqual([]);
  });
});

// THE READER, ASKED ABOUT EVERY SHAPE IT HAS TO SEE.
//
// The arms above can only report what this resolves, so a reader that quietly
// stopped seeing one spelling would take the whole spec to PASS with nothing
// failing. Each case is a shape the tree writes, including the four the census
// found and the one it must NOT report.
describe("what a second height looks like", () => {
  const corpus = new Set(["thing", "field"]);
  const found = (css: string) =>
    findingsIn("probe.css", css, corpus).map((one) => one.says);

  it("reads a fixed height, a floor, and their logical spellings", () => {
    expect(found(".thing { height: 26px; }")).toEqual(["height:26px"]);
    expect(found(".thing { min-height: 44px; }")).toEqual(["min-height:44px"]);
    expect(found(".thing { block-size: 2rem; }")).toEqual(["block-size:2rem"]);
    expect(found(".thing { min-block-size: var(--space-6); }")).toEqual([
      "min-block-size:var(--space-6)",
    ]);
  });

  it("passes the token itself, in any of the four spellings", () => {
    expect(found(".thing { min-block-size: var(--controlHeight); }")).toEqual(
      [],
    );
    expect(found(".thing { height: var(--controlHeight); }")).toEqual([]);
  });

  it("reads a rule the element itself selects", () => {
    expect(found(".bar button { height: 26px; }")).toEqual(["height:26px"]);
  });

  it("reads a height inside a breakpoint, which is where a second one hides", () => {
    expect(
      found("@media (max-width: 700px) { .thing { min-height: 48px; } }"),
    ).toEqual(["min-height:48px"]);
  });

  it("files a rule against what it SELECTS, not what it descends from", () => {
    // `.thing .mark` draws the mark, and a finding filed against `.thing`
    // would be a control blamed for the size of a glyph inside it.
    expect(found(".thing .mark { height: 14px; }")).toEqual([]);
    // And the class a `:not()` excuses is not the subject either.
    expect(found(".bar button:not(.field) { min-height: 44px; }")).toEqual([
      "min-height:44px",
    ]);
  });

  it("reads neither a ceiling nor a line", () => {
    expect(found(".thing { max-height: 60px; line-height: 20px; }")).toEqual(
      [],
    );
  });

  it("says nothing about a box nobody presses", () => {
    expect(found(".plate { height: 26px; }")).toEqual([]);
  });

  it("names the class and the selector, so ACCEPTED can be keyed by either", () => {
    const [one] = findingsIn(
      "probe.css",
      ".bar .thing { height: 26px; }",
      corpus,
    );
    expect(one.keys).toEqual(["thing", ".bar .thing"]);
  });
});
