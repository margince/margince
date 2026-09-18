// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { rulesIn } from "../testing/css";

// A page's NAME is as wide as the page.
//
// The head used to be full width while the body was capped, so on a wide
// display "Saved views" began a hand's width to the left of the first card
// under it — the page disagreeing with itself about where it begins. The fix is
// not a second measure for the head; it is that there is only ever ONE measure,
// published on the column as --pageMeasure and applied to the head and the body
// by the same declaration. That is what this gate holds.
//
// Two obligations, and both decay silently — a title an inch off is a layout
// anyone can look straight past:
//
// 1. **One declaration caps both.** Every `max-width` on `.pagetitle` in
//    app/shell.css is declared in the same rule as the one on `.wrap`. A head
//    with a rule of its own is the only way the two can be handed different
//    numbers, so the gate refuses the rule rather than comparing the values —
//    comparing values is what lets them drift and then reports it afterwards.
// 2. **The settings pair shares a token.** A settings page's body is narrower
//    than its page, and the head follows it there. The stack's measure
//    (screens/settings.css) and the head's (app/shell.css) must be the SAME
//    custom property, and that property must be spelled once in the tree —
//    otherwise 720 exists twice and one of the two moves first.
//
// Derived from the sheets rather than restated here: the selectors are read out
// of the CSS and the token's NAME is read off the stack, so renaming either
// keeps the gate pointed at the thing it protects. Under-recognition is the
// failure that matters — a parse that finds nothing reports PASS — so each
// corpus fails closed on an empty read and the finder carries planted cases of
// every shape it claims to catch.

const appDir = dirname(fileURLToPath(import.meta.url));
const srcDir = join(appDir, "..");

const SHELL_CSS = join(appDir, "shell.css");
const SETTINGS_CSS = join(srcDir, "screens", "settings.css");
const TOKENS_CSS = join(srcDir, "design-system", "tokens.css");

// A sheet this small is a read that went wrong, not a sheet.
const RULES_FLOOR = 20;

type Capping = { selectors: string[]; value: string; line: number };

function read(path: string): string {
  return readFileSync(path, "utf8");
}

/** The selectors of one rule, as written, comma by comma. */
function selectorsOf(selector: string): string[] {
  return selector
    .split(",")
    .map((one) => one.replace(/\s+/g, " ").trim())
    .filter((one) => one.length > 0);
}

/** A rule's own `max-width`, or undefined — nested blocks are already stripped. */
function maxWidthOf(body: string): string | undefined {
  const found = /(?:^|;)\s*max-width\s*:([^;]+)/.exec(body);
  return found ? found[1].trim() : undefined;
}

/** Every rule that caps something, with what it caps and at what. */
function cappingRules(css: string): Capping[] {
  return rulesIn(css).flatMap((rule) => {
    const value = maxWidthOf(rule.body);
    return value === undefined
      ? []
      : [{ selectors: selectorsOf(rule.selector), value, line: rule.line }];
  });
}

/** Whether any selector in the rule targets this class as a whole token. */
function targets(rule: Capping, className: string): boolean {
  const token = new RegExp(`\\.${className}(?![\\w-])`);
  return rule.selectors.some((one) => token.test(one));
}

/**
 * What is wrong with how this sheet caps the page head, in words.
 *
 * Empty means the head and the column are capped by one declaration and by
 * nothing else — which is the whole invariant.
 */
function headMeasureFaults(css: string): string[] {
  const capping = cappingRules(css);
  const head = capping.filter((rule) => targets(rule, "pagetitle"));
  if (head.length === 0) {
    return ["nothing caps .pagetitle: the head is not sharing the column"];
  }
  return head.flatMap((rule) =>
    targets(rule, "wrap")
      ? []
      : [
          `line ${rule.line}: .pagetitle is capped at ${rule.value} by a rule ` +
            "that does not cap .wrap — a measure of its own is a measure that drifts",
        ],
  );
}

/** The custom property a rule's `max-width` reads, or undefined for a literal. */
function measureToken(value: string): string | undefined {
  const found = /var\(\s*(--[\w-]+)/.exec(value);
  return found ? found[1] : undefined;
}

describe("the page head takes the page's measure", () => {
  const shell = read(SHELL_CSS);

  it("reads a whole stylesheet", () => {
    expect(rulesIn(shell).length).toBeGreaterThan(RULES_FLOOR);
    expect(cappingRules(shell).length).toBeGreaterThan(0);
  });

  it("caps the head and the column with one declaration", () => {
    expect(headMeasureFaults(shell)).toEqual([]);
  });

  // The planted cases: each is a sheet the product must never be, and a finder
  // that stops seeing one of them goes quiet rather than failing.
  it.each([
    [
      "a head with a measure of its own",
      `.main-gridded .wrap { max-width: var(--pageColumn); }
       .main-gridded .pagetitle { max-width: 1280px; }`,
    ],
    [
      "two rules that agree today and are free to stop agreeing",
      `.main-gridded .wrap, .other { max-width: var(--pageColumn); }
       .main-gridded .pagetitle { max-width: var(--pageColumn); }`,
    ],
    [
      "a capped column above an uncapped head",
      ".main-gridded .wrap { max-width: var(--pageColumn); }",
    ],
  ])("sees %s", (_case, css) => {
    expect(headMeasureFaults(css).length).toBeGreaterThan(0);
  });

  it("passes the shape the shell actually ships", () => {
    expect(
      headMeasureFaults(
        `.main-gridded .wrap,
         .main-gridded .pagetitle { max-width: var(--pageMeasure); }`,
      ),
    ).toEqual([]);
  });
});

describe("the settings head and the settings stack share one token", () => {
  const settings = read(SETTINGS_CSS);
  const shell = read(SHELL_CSS);

  it("reads a whole stylesheet", () => {
    expect(rulesIn(settings).length).toBeGreaterThan(RULES_FLOOR);
  });

  // The stack is the thing being measured, so the token comes off IT. A gate
  // that named the property itself would be a third place the measure is
  // spelled, which is the defect it exists to prevent.
  const stack = cappingRules(settings).filter((rule) =>
    targets(rule, "settings-stack"),
  );
  // Read once, and deliberately not defaulted to anything usable: a stack that
  // no longer declares a token leaves this `"--none"`, and every assertion below
  // then fails rather than passing over a sheet the gate stopped understanding.
  const stackToken = measureToken(stack[0]?.value ?? "") ?? "--none";

  it("caps the settings stack with a token", () => {
    expect(stack).toHaveLength(1);
    expect(stackToken).toMatch(/^--(?!none$)/);
  });

  it("caps the head above it with that same token", () => {
    // The head's measure on a settings page is published as --pageMeasure by
    // the rule that recognises the stack; what it must READ is the stack's own
    // token, plus whatever gutter the head carries and the stack does not.
    const published = rulesIn(shell).filter((rule) =>
      new RegExp(`--pageMeasure\\s*:[^;]*var\\(\\s*${stackToken}`).test(
        rule.body,
      ),
    );
    expect(published.length).toBeGreaterThan(0);
  });

  it("spells that token exactly once in the tree", () => {
    const declaration = new RegExp(`^\\s*${stackToken}\\s*:`, "gm");
    const sheets = [shell, settings, read(TOKENS_CSS)];
    const declared = sheets.reduce(
      (count, sheet) => count + (sheet.match(declaration)?.length ?? 0),
      0,
    );
    expect(declared).toBe(1);
  });
});
