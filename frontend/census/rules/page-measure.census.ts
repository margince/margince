// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A page's column and its head take the page's one measure. A `.wrap` or
// `.pagetitle` capped at a width of its own, in any sheet the bundle ships, is
// a page that begins where no other page does.

import {
  classesOf,
  type Rule,
  rulesOfText,
  subjectsOf,
} from "../../scripts/lib/css-rules";
import type { CensusFile, CensusRule } from "../tree";
import { isAppSheet } from "../tree";

const PAGE_MEASURE = "var(--pageMeasure)";
const COLUMN_CLASSES = ["wrap", "pagetitle"];
const CAP = /(?:^|[;{])\s*(max-width|max-inline-size)\s*:([^;}]+)/g;
const MUST_READ = ["src/app/shell.css", "src/features/settings/settings.css"];

type ColumnCap = Readonly<{
  file: string;
  selector: string;
  property: string;
  value: string;
}>;

const rulesOfSheets = (sheets: readonly CensusFile[]): Rule[] =>
  sheets.flatMap((sheet) => rulesOfText(sheet.rel, sheet.text));

/** Every cap a rule puts on the column or its head, through any `:is()` arm. */
function columnCaps(rules: readonly Rule[]): ColumnCap[] {
  return rules.flatMap((rule) => {
    const capsColumn = subjectsOf(rule.selector).some((subject) => {
      const classes = classesOf(subject);
      return COLUMN_CLASSES.some((name) => classes.has(name));
    });
    if (!capsColumn) {
      return [];
    }
    return [...rule.body.matchAll(CAP)].map(([, property, value]) => ({
      file: rule.file,
      selector: rule.selector,
      property,
      value: value.trim(),
    }));
  });
}

export const rule: CensusRule = {
  name: "page-measure",
  corpus: (tree) => tree.files.filter(isAppSheet),
  floors: (corpus) => {
    const read = new Set(corpus.map((sheet) => sheet.rel));
    const unread = MUST_READ.filter((rel) => !read.has(rel)).map(
      (rel) => `${rel} was not read`,
    );
    const capped = new Set(
      columnCaps(rulesOfSheets(corpus)).flatMap((cap) =>
        COLUMN_CLASSES.filter((name) =>
          new RegExp(`\\.${name}(?![\\w-])`).test(cap.selector),
        ),
      ),
    );
    const uncapped = COLUMN_CLASSES.filter((name) => !capped.has(name)).map(
      (name) => `no sheet caps .${name}, so the cap was not found`,
    );
    return [...unread, ...uncapped];
  },
  across: (corpus) =>
    columnCaps(rulesOfSheets(corpus))
      .filter((cap) => cap.value.replaceAll(/\s+/g, "") !== PAGE_MEASURE)
      .map(
        (cap) =>
          `${cap.file}: ${cap.selector} { ${cap.property}: ${cap.value} }`,
      ),
  advice: `The column and its head take ${PAGE_MEASURE} and no other width, so every page begins where the others do.`,
};
