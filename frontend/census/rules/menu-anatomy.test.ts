// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { selectorList } from "../../scripts/lib/css-rules";
import { parseSource } from "../../scripts/lib/source-tree";
import { type CssRule, rulesIn } from "../../src/testing/css";
import { type Area, plantedFindings, srcDir } from "../tree";
import {
  CEILING,
  classesUnderAnOptionRole,
  FLOOR,
  INSET,
  rule,
  SURFACES,
} from "./menu-anatomy.census";

// The roster reads the declaration, not the paint: jsdom applies no sheet, so
// what it proves is that the surfaces spell one answer.

const sheets = new Map<string, CssRule[]>();
function rulesOfSheet(sheet: string): CssRule[] {
  const known = sheets.get(sheet);
  if (known) return known;
  const read = rulesIn(readFileSync(join(srcDir, sheet), "utf8"));
  sheets.set(sheet, read);
  return read;
}

/** The top-level rule whose selector list holds `selector`, in one sheet. */
function ruleFor(sheet: string, selector: string): CssRule {
  const found = rulesOfSheet(sheet).find(
    (rule) =>
      rule.parents.length === 0 &&
      selectorList(rule.selector).includes(selector),
  );
  if (!found) {
    throw new Error(`${sheet} declares no rule for ${selector}`);
  }
  return found;
}

/** One declaration's value, or `undefined` where the rule does not set it. */
function declaredValue(rule: CssRule, property: string): string | undefined {
  const declarations = rule.body.split(";");
  for (const declaration of declarations.reverse()) {
    const at = declaration.indexOf(":");
    if (at < 0) continue;
    if (declaration.slice(0, at).trim() === property) {
      return declaration
        .slice(at + 1)
        .trim()
        .replace(/\s+/g, " ");
    }
  }
  return undefined;
}

/**
 * What a row spends above and below its line: `padding-block`, or the first
 * component of the `padding` shorthand. Both spellings are legitimate, so this
 * reads the axis rather than one property.
 */
function blockInset(rule: CssRule): string | undefined {
  const block = declaredValue(rule, "padding-block");
  if (block !== undefined) return block;
  const shorthand = declaredValue(rule, "padding");
  if (shorthand === undefined) return undefined;
  // `var(--x, 250px) 9px` splits on spaces outside parentheses only.
  const parts: string[] = [];
  let depth = 0;
  let current = "";
  for (const character of shorthand) {
    if (character === "(") depth++;
    if (character === ")") depth--;
    if (character === " " && depth === 0) {
      if (current) parts.push(current);
      current = "";
      continue;
    }
    current += character;
  }
  if (current) parts.push(current);
  return parts[0];
}

/**
 * The anatomy fixes a row's height, padding plus line plus border. A row that
 * keeps a recoloured 1px border (a `Button` in the overflow panel) spends a
 * pixel less of inset.
 */
function expectedInset(rule: CssRule): string {
  const recolours =
    declaredValue(rule, "border-color") !== undefined &&
    declaredValue(rule, "border") === undefined;
  return recolours ? `calc(${INSET} - 1px)` : INSET;
}

describe("one menu anatomy", () => {
  describe.each(SURFACES)("$selector", (surface) => {
    const rule = () => ruleFor(surface.sheet, surface.selector);

    it("spends the anatomy's inset on itself", () => {
      expect(declaredValue(rule(), "padding")).toBe(INSET);
    });

    it("stands at the anatomy's floor, or says why not", () => {
      const floor = declaredValue(rule(), "min-inline-size");
      if (surface.floor === null) {
        expect(floor).toBe(FLOOR);
      } else {
        expect(surface.floor.length).toBeGreaterThan(0);
        expect(floor).not.toBe(FLOOR);
      }
    });

    it("scrolls at the anatomy's ceiling, or says why not", () => {
      const ceiling = declaredValue(rule(), "max-block-size");
      if (surface.ceiling === null) {
        expect(ceiling).toBe(CEILING);
      } else {
        expect(surface.ceiling.length).toBeGreaterThan(0);
        expect(ceiling).not.toBe(CEILING);
      }
    });

    it.each(surface.instead ?? [])(
      "$selector declares $property: $value",
      ({ selector, property, value }) => {
        expect(declaredValue(ruleFor(surface.sheet, selector), property)).toBe(
          value,
        );
      },
    );

    it.each(surface.rows)("%s is one --controlHeight tall", (row) => {
      const rowRule = ruleFor(surface.sheet, row);
      expect(blockInset(rowRule)).toBe(expectedInset(rowRule));
    });

    if (surface.head) {
      const head = surface.head;
      it(`${head.selector} takes the same inset down the block axis`, () => {
        expect(blockInset(ruleFor(surface.sheet, head.selector))).toBe(INSET);
      });
      if (head.labelled) {
        it(`${head.selector} outweighs the rows under it`, () => {
          expect(
            declaredValue(ruleFor(surface.sheet, head.selector), "font-weight"),
          ).toBe("var(--fontWeightMedium)");
        });
      }
    }
  });
});

// The fallback beside each token is a second copy of the number, which holds
// while the two agree.
describe("the anatomy's two sizes", () => {
  const tokens = readFileSync(join(srcDir, "design-system/tokens.css"), "utf8");

  it.each([
    ["--menuMinInlineSize", FLOOR],
    ["--menuMaxBlockSize", CEILING],
  ])("%s agrees with the fallback spelled beside it", (token, spelling) => {
    const declared = new RegExp(`${token}\\s*:\\s*([^;]+);`).exec(tokens)?.[1];
    const fallback = /,\s*([^)]+)\)/.exec(spelling)?.[1]?.trim();
    // An undeclared token agrees: the fallback is the value until tokens.css
    // carries one.
    expect(declared?.trim() ?? fallback).toBe(fallback);
  });
});

describe("the option-role census", () => {
  it("reads the classes on a menu-ish role, less states and placements", () => {
    const source = parseSource(
      "fixture.tsx",
      `const a = <ul role="listbox" className="pick-list is-open">{rows}</ul>;
       const b = <li role="option" className={cx("pick-row", "active")} />;
       const c = <div role="dialog" className="not-a-menu" />;
       const d = <div role="menu" className="topbar-popover">{x}</div>;`,
    );
    expect(classesUnderAnOptionRole(source)).toEqual(["pick-list", "pick-row"]);
  });

  it("reports a planted option surface the roster does not know", () => {
    const area: Area = "src";
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.tsx"),
          area,
          text: 'export const A = () => <ul role="menu" className="lt-menu new-menu" />;',
        },
        {
          path: join(srcDir, "features", "planted.test.tsx"),
          area,
          text: 'export const B = () => <ul role="menu" className="fixture-menu" />;',
        },
      ]),
    ).toEqual([
      ".new-menu sits on a menu or listbox role and is in no roster entry",
    ]);
  });
});
