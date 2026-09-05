// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { withoutComments } from "../testing/css";

// Fitness function for the chrome disappearing behind the page.
//
// The shell is two boxes side by side: the sidebar and the work column. The
// sidebar's collapsed tooltips hang PAST its right edge, over the column — the
// only part of the chrome that overlaps the page at all, and therefore the one
// place a layering mistake shows.
//
// It showed. Screens stack freely inside their own column — the list table
// alone declares 1 for its sticky header, 2 for the frozen identity column, 3
// for that column's shade and 20 for a menu — and while the column formed no
// stacking context, those numbers were not internal at all: they competed in
// the ROOT context against the sidebar's 2, where the column, being later in
// the tree, won every tie. A rail tooltip was cut off at the table's left edge,
// behind the frozen column, on every list screen in the product.
//
// The fix is an invariant with two ends, and either end alone is green while
// the defect is on screen: `.main` isolates, so a screen's layers stay the
// screen's, and the sidebar takes a layer above the column's own. This holds
// both, and reads the numbers from the sheet rather than repeating them —
// a gate that carries its own copy of its subject stops being a gate the day
// somebody restyles the real thing.
//
// WHAT IT CANNOT SEE, deliberately:
//
//   - Paint. It reads what the sheet declares; no test in this suite renders a
//     stacking context, because jsdom applies no stylesheet. What decays here
//     is the SPELLING of the rule, which is what actually went missing.
//   - Anything portalled to `document.body` — a select popup, a dialog, the
//     design system's own tooltip. Those leave the column altogether and are
//     layered against the stack in design-system/, not against this one.
//   - A new sibling of `.main` inside `.app` given a layer of its own. There is
//     no census of siblings here; there are three, and they are in shell.tsx.

const shellSheet = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "shell.css",
);

type Rule = Readonly<{
  /** The selectors this block is declared against, one per entry. */
  selectors: readonly string[];
  body: string;
  /** Inside an `@media` or `@supports`, rather than at the top level. */
  nested: boolean;
}>;

/**
 * Every innermost rule block in a stylesheet.
 *
 * Innermost is what the pattern matches — a prelude and a body that contain no
 * braces of their own — and a block inside a media query is innermost too. That
 * is the half a scan loses first: this sheet declares the sidebar TWICE, once
 * as a desktop column and once as a phone bar inside `@media`, and a scan that
 * stopped at the top level would read one of the two layers and pass.
 */
function rules(css: string): Rule[] {
  const text = withoutComments(css);
  const found: Rule[] = [];
  for (const match of text.matchAll(/([^{}]*)\{([^{}]*)\}/g)) {
    found.push({
      selectors: match[1]
        .split(",")
        .map((one) => one.trim().replace(/\s+/g, " "))
        .filter(Boolean),
      body: match[2],
      nested: openBlocks(text.slice(0, match.index)) > 0,
    });
  }
  return found;
}

function openBlocks(before: string): number {
  let depth = 0;
  for (const character of before) {
    if (character === "{") {
      depth += 1;
    } else if (character === "}") {
      depth -= 1;
    }
  }
  return depth;
}

function declared(rule: Rule, property: string): string | undefined {
  const match = new RegExp(`(?:^|[;{\\s])${property}\\s*:\\s*([^;}]+)`).exec(
    rule.body,
  );
  return match?.[1].trim();
}

/**
 * The layer a rule sits on. An element with no `z-index` is `auto`, which
 * against a positive layer loses exactly as 0 does — the question this asks is
 * only ever "does the chrome outrank it".
 */
function layer(rule: Rule): number {
  const value = declared(rule, "z-index");
  return value === undefined ? 0 : Number(value);
}

const sheet = readFileSync(shellSheet, "utf8");
const shell = rules(sheet);
const workColumn = shell.filter((rule) => rule.selectors.includes(".main"));
// The panel itself in either state, not the rows and tooltips inside it: those
// are layered within whatever context the panel makes, which is the point.
const sidebar = shell.filter((rule) =>
  rule.selectors.some((one) => /^\.rail(\.(collapsed|expanded))?$/.test(one)),
);

describe("the work column never paints over the chrome", () => {
  // A census that fails short reports PASS over a sheet it could not read, and
  // there is no failing assertion to notice. Both boxes are named, and the
  // sidebar is asserted to have been found on BOTH layouts — the phone one
  // lives inside a media query, so this is also what proves the scan descends
  // into at-rules.
  it("finds both boxes, on both layouts", () => {
    expect(workColumn.length).toBeGreaterThanOrEqual(1);
    expect(
      sidebar.filter((rule) => !rule.nested).length,
    ).toBeGreaterThanOrEqual(1);
    expect(
      sidebar.some((rule) => rule.nested && declared(rule, "z-index")),
      "the scan did not reach the phone bar's layer inside its media query",
    ).toBe(true);
  });

  it("isolates the column, so a screen's layers stay the screen's", () => {
    expect(
      workColumn.map((rule) => declared(rule, "isolation")),
      "`.main` must form a stacking context: without one, a sticky table header " +
        "and a rail tooltip are two numbers in the SAME context, and the column " +
        "wins the tie by being later in the tree",
    ).toContain("isolate");
  });

  it("gives the sidebar a layer above the column's", () => {
    const column = Math.max(...workColumn.map(layer));
    const layers = sidebar
      .filter((rule) => declared(rule, "z-index"))
      .map(layer);
    expect(layers.length).toBeGreaterThanOrEqual(2);
    for (const above of layers) {
      expect(above).toBeGreaterThan(column);
    }
  });

  // `z-index` on a static element is inert, and inert in the quietest way: the
  // declaration is right there in the sheet for the next reader to trust.
  it("positions the sidebar, so its layer is not inert", () => {
    for (const rule of sidebar.filter((one) => declared(one, "z-index"))) {
      expect(declared(rule, "position")).not.toBe("static");
      expect(declared(rule, "position")).toBeDefined();
    }
  });
});
