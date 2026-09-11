// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { withoutComments } from "../testing/css";

// Fitness function for a disclosure nested inside a disclosure.
//
// A screen that draws its sections as `Disclosure`s gives the section's body
// the pane's inset — that is how a slice's rows sit on the panel's gutter. Spelt
// as a DESCENDANT (`.co-rail .co-sect .disclosure-body`), that inset also
// reaches every disclosure nested inside the slice, which pays it a second time:
// the company rail's Details slice draws the postal address behind a disclosure
// of its own, and its six rows started a full pane-padding right of the nine
// rows above them, with the value column losing that width at both ends — long
// enough to break a street name mid-syllable while the fields above it did not.
//
// The invariant, then: a section's inset belongs to that section's OWN body, so
// a nested disclosure keeps the atom's body and both grids share one left edge.
// Scoped with `>` it does; there is no other way to spell it that a reader can
// see at the call site, which is why this asks about the combinator rather than
// about any one screen's rule.
//
// WHAT IT CANNOT SEE, deliberately:
//
//   - Paint. It reads what the sheets declare; jsdom applies no stylesheet, so
//     no test in this suite measures a left edge.
//   - An inset put on the nested disclosure by any other selector — a screen
//     class on the `<details>` itself, or padding on the grid inside it. The
//     descendant `.disclosure-body` is the shape that actually regressed, twice
//     in this tree.

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

function stylesheets(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : stylesheets(path);
    }
    return entry.name.endsWith(".css") ? [path] : [];
  });
}

/** One selector mentioning `.disclosure-body`, and where it was written. */
type Mention = Readonly<{ sheet: string; selector: string }>;

// Every selector in the tree that reaches a disclosure's body. Selector
// preludes, one per comma, comments blanked first — a rule spelt out in the
// paragraph above it is prose, and a commented-out rule styles nothing.
const mentions: Mention[] = stylesheets(srcRoot).flatMap((sheet) => {
  const css = withoutComments(readFileSync(sheet, "utf8"));
  return [...css.matchAll(/([^{}]+)\{[^{}]*\}/g)].flatMap((rule) =>
    rule[1]
      .split(",")
      .map((one) => one.trim().replace(/\s+/g, " "))
      .filter((one) => one.includes(".disclosure-body"))
      .map((selector) => ({ sheet: relative(srcRoot, sheet), selector })),
  );
});

// The atom's own rule is the one selector that names the body with nothing to
// its left; every other mention scopes it to something, and scoping is what
// this gate is about.
const scoped = mentions.filter(
  (one) => !/^\.disclosure-body\b/.test(one.selector),
);

describe("a section's inset stops at that section's own body", () => {
  // A scan that reads a smaller tree than it thinks reports PASS with no
  // failing assertion to notice, so the corpus is asserted before it is judged:
  // the sheets, the atom's own rule, and the scoped mentions the rule below
  // exists for.
  it("reads the whole tree of stylesheets", () => {
    expect(stylesheets(srcRoot).length).toBeGreaterThan(20);
    expect(
      mentions.some((one) => /^\.disclosure-body\b/.test(one.selector)),
      "the scan did not find the atom's own `.disclosure-body` rule, so it is " +
        "reading something other than this tree's stylesheets",
    ).toBe(true);
    expect(scoped.length).toBeGreaterThanOrEqual(2);
  });

  it("scopes every inset to a direct child", () => {
    for (const { sheet, selector } of scoped) {
      expect(
        selector,
        `${sheet} insets \`.disclosure-body\` by descent, so a disclosure ` +
          "nested inside that section pays the inset a second time and its " +
          "rows start right of the rows above them; scope it with `>`",
      ).toMatch(/>\s*\.disclosure-body\b/);
    }
  });
});
