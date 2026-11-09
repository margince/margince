// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One menu anatomy. A menu, a dropdown, a suggestion list and an overflow panel
// are one object to a reader: a floating surface carrying things to choose.
// The anatomy is three numbers in tokens.css: `--controlGap` (the surface's
// inset and a row's block inset), `--menuMinInlineSize` and `--menuMaxBlockSize`.
//
// The roster below is written by hand: which class is a menu is in no file to
// read. This rule holds that every element claiming a menu or listbox role
// wears classes the roster knows, so a new option list must join it.
//
// ListTable's `Menu` is a fieldset of buttons and radios, with no menu role for
// the walk to find, so it is in the roster by hand.

import ts from "typescript";
import { classNameLiterals } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule } from "../tree";

// The floor and the ceiling as every surface that takes them spells them. Each
// is the token, with the value as its fallback for a document without
// tokens.css.
export const FLOOR = "var(--menuMinInlineSize, 250px)";
export const CEILING = "var(--menuMaxBlockSize, 400px)";
// The inset, and the block half of a row's. Nothing renders a menu without
// tokens.css, so it needs no fallback.
export const INSET = "var(--controlGap)";

/**
 * Why a surface does not take the anatomy's floor or ceiling, or `null` when it
 * does. A reason is required: an unexplained opt-out is the drift this stops.
 */
type Exemption = string | null;

export type Surface = Readonly<{
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
  /** What an exemption's reason says is declared instead, read back. */
  instead?: readonly Readonly<{
    selector: string;
    property: string;
    value: string;
  }>[];
}>;

export const SURFACES: readonly Surface[] = [
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
    ceiling:
      "three theme radios, or help's five rows over the build; neither roster grows",
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
    selector: ".popover-panel.listpopover-panel",
    sheet: "design-system/listpopover.css",
    rows: [],
    floor:
      "a fixed 20rem, so the panel holds still while a server search answers under it",
    ceiling:
      "the list inside it scrolls at the ceiling, so the search box above the rows stays in view",
    instead: [
      {
        selector: ".popover-panel.listpopover-panel",
        property: "inline-size",
        value: "20rem",
      },
      {
        selector: ".listpopover-panel .listpopover-list",
        property: "max-block-size",
        value: CEILING,
      },
    ],
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

const OPTION_ROLES = new Set(["menu", "listbox", "menuitem", "option"]);

// A state a row wears (`is-active`, `active`) and a class that only places a
// surface (app/topbar-popover.css) say nothing about a box.
const STATES = /^(is-|active$|selected$|open$|right$)/;
const PLACEMENTS = new Set(["topbar-popover"]);

/** Every class one module puts on an element claiming a menu-ish role. */
export function classesUnderAnOptionRole(source: ts.SourceFile): string[] {
  const found = new Set<string>();
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
        for (const literal of classNameLiterals(className)) {
          for (const one of literal.split(/\s+/).filter(Boolean)) {
            found.add(one);
          }
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return [...found]
    .filter((one) => !STATES.test(one) && !PLACEMENTS.has(one))
    .sort();
}

/** The last class of every roster selector: what an element wears. */
export const KNOWN = new Set(
  SURFACES.flatMap((surface) => [
    surface.selector,
    ...surface.rows,
    ...(surface.head ? [surface.head.selector] : []),
  ]).map((selector) => selector.split(" ").at(-1)?.slice(1) ?? ""),
);

const isMarkup = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.path.endsWith(".tsx") &&
  !/\.(test|stories|testkit)\.tsx$/.test(file.path);

export const rule: CensusRule = {
  name: "menu-anatomy",
  corpus: (tree) => tree.files.filter(isMarkup),
  across: (corpus) =>
    [
      ...new Set(
        corpus.flatMap((file) =>
          file.source ? classesUnderAnOptionRole(file.source) : [],
        ),
      ),
    ]
      .filter((className) => !KNOWN.has(className))
      .sort()
      .map(
        (className) =>
          `.${className} sits on a menu or listbox role and is in no roster entry`,
      ),
  advice:
    "Every option surface and row joins the roster in census/rules/menu-anatomy.census.ts and takes the anatomy.",
};
