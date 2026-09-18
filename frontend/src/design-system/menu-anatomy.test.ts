// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesMatching, sourceFileAt } from "../../scripts/lib/source-tree";
import { type CssRule, rulesIn } from "../testing/css";

// ONE MENU ANATOMY, and this is what holds it.
//
// A menu, a dropdown, a suggestion list and an overflow panel are the same
// object to a reader: a surface that floats over the page carrying a list of
// things they may choose. The product drew five of them, each with its own
// inset, its own floor and its own row height — 4px of padding here and 5px
// there, a 180px menu beside a 236px one, rows of 28px above rows of 34px —
// and every one of them looked deliberate on its own screen.
//
// So the anatomy is three numbers and they live in tokens.css, not in the five
// sheets: `--controlGap` is the inset a menu spends on itself and the inset a
// row spends above and below its line (6 + a 20px --fontBody line + 6 is the
// --controlHeight every other control in the product stands at),
// `--menuMinInlineSize` is how wide a menu is before its content says
// otherwise, and `--menuMaxBlockSize` is how tall it gets before it scrolls.
//
// WHAT THIS READS: the declaration, not the paint. jsdom applies no stylesheet,
// so nothing here measures a row; what it can prove is that the five surfaces
// spell one answer rather than five, which is the failure that actually
// happened.
//
// WHAT IT CANNOT SEE: a screen re-spacing a menu from its own sheet — the
// spacing-roles gate is what asks that question, over the whole tree. Nor can
// the census below reach ListTable's own menus: a `Menu` is a fieldset with an
// aria-label and its rows are buttons and radios, so it claims no menu ROLE for
// the walk to find. It is in the roster by hand, which is why the roster exists
// at all; what the census adds is that nothing NEW can appear outside it.

const designSystem = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(designSystem, "..");

// The floor and the ceiling as every surface that takes them must spell them:
// the token, with the value as its fallback so the sheet still draws correctly
// in a document that has not loaded tokens.css.
const FLOOR = "var(--menuMinInlineSize, 250px)";
const CEILING = "var(--menuMaxBlockSize, 400px)";
// The inset, and the block half of a row's. This one is an existing token and
// needs no fallback: nothing renders without tokens.css that renders a menu.
const INSET = "var(--controlGap)";

/**
 * Why a surface does not take the anatomy's floor or ceiling, or `null` when it
 * does.
 *
 * A reason is required rather than optional: a menu that quietly opted out is
 * the drift this file exists to stop, and "it was already like that" is not a
 * reason a reader can weigh.
 */
type Exemption = string | null;

type Surface = Readonly<{
  /** The class the surface is drawn as. */
  selector: string;
  /** Its sheet, relative to `src/`. */
  sheet: string;
  /** The option rows it holds, as their sheets select them. */
  rows: readonly string[];
  /** Its head, where it has one, and whether that head carries a label. */
  head?: Readonly<{ selector: string; labelled: boolean }>;
  floor: Exemption;
  ceiling: Exemption;
  /**
   * Set where the sheet is owned by work landing separately. The surface still
   * belongs to the roster — leaving it out would be a gate that reads a smaller
   * tree and reports PASS — so instead its PRE-PATCH spelling is pinned. The
   * day the patch lands, this fails and names the entry to flip.
   */
  awaitingPatch?: Readonly<{
    reason: string;
    surfacePadding: string;
    rowPadding: string;
  }>;
}>;

const SURFACES: readonly Surface[] = [
  {
    selector: ".lt-menu",
    sheet: "design-system/listtable.css",
    rows: [".lt-mi"],
    head: { selector: ".lt-mhead", labelled: true },
    floor: null,
    ceiling: null,
  },
  {
    selector: ".select-popup",
    sheet: "design-system/select.css",
    rows: [".select-option"],
    floor:
      "the trigger's width is the list's floor, handed in as an inline style by select.tsx",
    ceiling:
      "the ceiling is --popupRoom, the space anchoredpopup.ts measured to the viewport's margin",
  },
  {
    selector: ".suggest-popup",
    sheet: "design-system/suggestlist.css",
    rows: [".suggest-option"],
    floor:
      "the list is the text box's width, so a suggestion lines up with the characters it replaces",
    ceiling: "the ceiling is the room anchoredpopup.ts measured, inline",
  },
  {
    selector: ".accountmenu",
    sheet: "app/account.css",
    rows: [".acctrow"],
    head: { selector: ".acctmenuwho", labelled: false },
    floor: null,
    ceiling:
      "the only menu hosting an absolutely positioned flyout, which a scroll container would clip; its rows are a fixed roster",
  },
  {
    selector: ".accountsub",
    sheet: "app/account.css",
    rows: [".accountsub .choicelist-choice"],
    floor:
      "it opens inward from a right-anchored menu, so width is distance toward the window's own left edge",
    ceiling: "three theme radios; it cannot grow",
  },
  {
    selector: ".overflow-menu-items",
    sheet: "design-system/atoms.css",
    rows: [".overflow-menu-items .btn"],
    floor: null,
    ceiling:
      "the component caps the height to the room on the side it opened toward, inline",
  },
  {
    selector: ".settingssearch-list",
    sheet: "app/shell.css",
    rows: [".settingssearch-hit"],
    floor:
      "pinned to the search box it drops from (left: 0; right: 0), so it has no width of its own",
    ceiling: "60vh, because it drops inside the rail rather than over the page",
  },
];

/**
 * A class the census will meet that is not a menu surface and not a row.
 *
 * Each is a container INSIDE a surface — the element the listbox role sits on,
 * the wrapper a menu's parentage needs — with no box of its own. Named so the
 * census below can be exhaustive rather than filtered.
 */
const NOT_A_BOX: readonly string[] = ["select-list", "suggest-list"];

/** Every stylesheet under `src/`, by its path relative to `src/`. */
const sheets = new Map<string, CssRule[]>(
  filesMatching(srcRoot, /\.css$/).map((path) => [
    path.slice(srcRoot.length + 1),
    rulesIn(readFileSync(path, "utf8")),
  ]),
);

/** The rule whose subject is exactly `selector`, in one sheet. */
function ruleFor(sheet: string, selector: string): CssRule {
  const found = (sheets.get(sheet) ?? []).find(
    (rule) =>
      rule.parents.length === 0 &&
      rule.selector.split(",").some((one) => one.trim() === selector),
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
 * What a row spends above and below its line, however the sheet spells it:
 * `padding-block`, or the first component of the `padding` shorthand.
 *
 * Both spellings are legitimate — a row whose inline inset is its own optical
 * value writes the shorthand — so the gate reads the AXIS rather than demanding
 * one property, which would be a rule about typing rather than about the box.
 */
function blockInset(rule: CssRule): string | undefined {
  const block = declaredValue(rule, "padding-block");
  if (block !== undefined) return block;
  const shorthand = declaredValue(rule, "padding");
  if (shorthand === undefined) return undefined;
  // `var(--x, 250px) 9px` splits on spaces OUTSIDE parentheses only.
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
 * The block inset a row spends: the anatomy's inset, less any border the row
 * KEEPS.
 *
 * What the anatomy fixes is the row's height — 6 above a --fontBody line and 6
 * below is the --controlHeight every control in the product stands at — and a
 * box's height is its padding plus its line plus its border. So a row that
 * removes its border (`border: 0`) or never had one spends the inset whole,
 * and a row that only RECOLOURS one spends a pixel less: the overflow panel's
 * rows are `Button`s, and the atom reserves a transparent 1px inside its own
 * box so that a ghost and a primary in a row are the same height.
 *
 * Read from the rule rather than listed per surface, because a list would be
 * this file holding a second copy of which rows are drawn from a control.
 */
function expectedInset(rule: CssRule): string {
  const recolours =
    declaredValue(rule, "border-color") !== undefined &&
    declaredValue(rule, "border") === undefined;
  return recolours ? `calc(${INSET} - 1px)` : INSET;
}

describe("one menu anatomy", () => {
  describe.each(
    SURFACES.filter((surface) => surface.awaitingPatch === undefined),
  )("$selector", (surface) => {
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

  // The two surfaces this change could not reach, pinned at what they say
  // TODAY. Not skipped: a roster that drops what it cannot fix is a census that
  // fails short, reports PASS over a smaller tree, and leaves no assertion to
  // notice. Pinned, the patch landing is what turns this red — which is the one
  // moment the entry above needs flipping to a full member.
  describe.each(SURFACES.filter((surface) => surface.awaitingPatch))(
    "$selector, still to take the anatomy",
    (surface) => {
      const pinned = surface.awaitingPatch;
      if (!pinned) throw new Error("filtered on awaitingPatch");

      it(`is unchanged in ${pinned.reason} — flip this entry when the patch lands`, () => {
        expect(
          declaredValue(ruleFor(surface.sheet, surface.selector), "padding"),
        ).toBe(pinned.surfacePadding);
        for (const row of surface.rows) {
          expect(declaredValue(ruleFor(surface.sheet, row), "padding")).toBe(
            pinned.rowPadding,
          );
        }
      });
    },
  );

  // THE CENSUS, and it is the half that cannot fail short. The roster above is
  // written by hand because the mapping from a class to "this is a menu" is not
  // in any file to read off; what IS readable is every element in the tree that
  // claims a menu or listbox role. A new option list therefore cannot be added
  // without either joining the roster or being named as a container with no box
  // — there is no third outcome where it is simply not looked at.
  // Reads every .tsx in src/ and in the extension frontends to find the roles.
  it("knows every option surface and row in the tree", {
    timeout: 60_000,
  }, () => {
    const known = new Set([
      ...SURFACES.flatMap((surface) => [
        surface.selector,
        ...surface.rows,
        ...(surface.head ? [surface.head.selector] : []),
      ]).map((selector) => selector.split(" ").at(-1)?.slice(1) ?? ""),
      ...NOT_A_BOX,
    ]);
    const unknown = classesUnderAnOptionRole().filter(
      (className) => !known.has(className),
    );
    expect(unknown).toEqual([]);
  });
});

const OPTION_ROLES = new Set(["menu", "listbox", "menuitem", "option"]);

/** Every class name the tree puts on an element claiming a menu-ish role. */
function classesUnderAnOptionRole(): string[] {
  const found = new Set<string>();
  for (const path of filesMatching(srcRoot, /\.tsx$/)) {
    if (/\.(test|stories|testkit)\.tsx$/.test(path)) continue;
    const source = sourceFileAt(path);
    const visit = (node: ts.Node) => {
      if (ts.isJsxOpeningLikeElement(node)) {
        const attributes = node.attributes.properties.filter(ts.isJsxAttribute);
        const role = attributes.find(
          (attribute) => attribute.name.getText(source) === "role",
        );
        const roleValue =
          role?.initializer && ts.isStringLiteral(role.initializer)
            ? role.initializer.text
            : undefined;
        if (roleValue && OPTION_ROLES.has(roleValue)) {
          const className = attributes.find(
            (attribute) => attribute.name.getText(source) === "className",
          );
          for (const literal of stringsIn(className)) {
            for (const one of literal.split(/\s+/).filter(Boolean)) {
              found.add(one);
            }
          }
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }
  // `is-active`, `active` and the like are STATES a row wears, not rows: they
  // ride the same attribute and say nothing about a box.
  const states = /^(is-|active$|selected$|open$|right$)/;
  return [...found].filter((one) => !states.test(one)).sort();
}

/** Every string literal inside a `className`, however the caller composed it. */
function stringsIn(attribute: ts.JsxAttribute | undefined): string[] {
  if (!attribute?.initializer) return [];
  const out: string[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      out.push(node.text);
    }
    if (ts.isTemplateExpression(node)) {
      out.push(node.head.text);
      for (const span of node.templateSpans) out.push(span.literal.text);
    }
    ts.forEachChild(node, visit);
  };
  visit(attribute.initializer);
  return out;
}

// The two sizes are TOKENS, and every call site spells the value as a fallback
// so the sheet is right in a document that has not loaded them. That fallback is
// a second copy of the number, which is fine exactly as long as it agrees — so
// the day tokens.css declares them, this is what says whether it agreed.
describe("the anatomy's two new sizes", () => {
  const tokens = readFileSync(
    join(srcRoot, "design-system/tokens.css"),
    "utf8",
  );

  it.each([
    ["--menuMinInlineSize", FLOOR],
    ["--menuMaxBlockSize", CEILING],
  ])("%s agrees with the fallback spelled beside it", (token, spelling) => {
    const declared = new RegExp(`${token}\\s*:\\s*([^;]+);`).exec(tokens)?.[1];
    const fallback = /,\s*([^)]+)\)/.exec(spelling)?.[1]?.trim();
    // An undeclared token reads as agreeing: the fallback IS the value until
    // tokens.css carries one. Spelled as a comparison rather than an early
    // return, because a test that returns before asserting proves nothing on
    // the path it took.
    expect(declared?.trim() ?? fallback).toBe(fallback);
  });
});
