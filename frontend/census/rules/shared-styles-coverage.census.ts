// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A screen that draws the record page's shared shapes imports their sheet.
//
// `company360.css` defines the record page's row and card shapes. A screen
// that draws them without importing it renders right only where the company
// record page has already loaded the sheet. Mounted elsewhere, Storybook among
// them, the meta parts run together and the row name loses its link look.
//
// The screens come from the directory, so a screen that starts drawing one of
// these classes is covered the day it is written.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { storySuffixes } from "../../scripts/lib/story-files";
import { type CensusFile, type CensusRule, frontendRoot } from "../tree";
import { screenCorpus } from "./screenfiles.census";

const SHARED_SHEET = "company360.css";

// The classes only `company360.css` defines. A class a screen's own sheet also
// declares is that screen's to own.
const BORROWED = ["co-rowlink", "co-row-meta", "co-card"];

const storybookMain = join(frontendRoot, ".storybook", "main.ts");

/** The suffixes Storybook loads stories from, read from its own config. */
const storyFileSuffixes = (): string[] =>
  storySuffixes(storybookMain, readFileSync(storybookMain, "utf8")) ?? [];

/** A borrowed class inside a className, not a mention in a comment. */
export function drawsBorrowedClass(source: string): boolean {
  const stripped = source
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");
  return BORROWED.some((name) =>
    new RegExp(`className=[^\\n]*\\b${name}\\b`).test(stripped),
  );
}

/** The import itself; a comment naming the sheet is not one. */
export function importsSharedSheet(source: string): boolean {
  return new RegExp(`^\\s*import\\s+["'][^"']*${SHARED_SHEET}["'];`, "m").test(
    source,
  );
}

export const rule: CensusRule = {
  name: "shared-styles-coverage",
  corpus: (tree) => {
    const suffixes = storyFileSuffixes();
    return screenCorpus(tree).filter(
      (file) => !suffixes.some((suffix) => file.path.endsWith(suffix)),
    );
  },
  visit: (file: CensusFile) =>
    drawsBorrowedClass(file.text) && !importsSharedSheet(file.text)
      ? [`${file.rel} draws a shape of ${SHARED_SHEET} without importing it`]
      : [],
  advice:
    `A screen draws ${BORROWED.join(", ")} without importing ${SHARED_SHEET}, ` +
    "so it renders unstyled wherever the company record page has not loaded " +
    `it. Add 'import "./${SHARED_SHEET}";'.`,
};
