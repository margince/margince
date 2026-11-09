// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// StatCard owns its door, its figure's ink and the room its words take.
//
// No caller names the door: an `openLabel` is a second word for the action
// `STAT_CARD_ACTIONS` owns. The figure's ink is spelled once, in atoms.css. A
// long reading is fixed in the layout and never shortened. So the name and the
// figure wrap inside the tile in every sheet the app ships.

import { basename, relative } from "node:path";
import ts from "typescript";
import {
  alternativesOf,
  classesOf,
  colorValues,
  type Rule,
  subjectOf,
} from "../../scripts/lib/css-rules";
import { STAT_CARD_TONES } from "../../src/design-system/statcard";
import { type CensusFile, type CensusRule, sheetRules, srcDir } from "../tree";

/** The line of every `openLabel` attribute in one module. */
export function openLabels(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isJsxAttribute(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "openLabel"
    ) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return lines;
}

/** Every rule outside atoms.css that colours the figure, and any second ink in it. */
export function figureInks(all: readonly Rule[]): string[] {
  return all.flatMap((rule) =>
    alternativesOf(rule.selector).flatMap((selector) => {
      const classes = classesOf(subjectOf(selector));
      const tone = STAT_CARD_TONES.find((one) =>
        classes.has(`stat-card-${one}`),
      );
      if (tone === undefined && !classes.has("stat-card-value")) return [];
      const home = basename(rule.file) === "atoms.css";
      return colorValues(rule.body)
        .filter((value) => !home || (tone && value !== `var(--${tone}Text)`))
        .map(
          (value) => `${basename(rule.file)} ${selector} { color: ${value} }`,
        );
    }),
  );
}

const WRAP_CUT = /(?:-webkit-)?line-clamp\s*:|white-space\s*:\s*nowrap/;
const END_CUT =
  /text-overflow\s*:\s*ellipsis|overflow(?:-x|-y)?\s*:\s*(?:hidden|clip)/;
const WHOLE = [
  "stat-card",
  "stat-card-label",
  "stat-card-label-text",
  "stat-card-value",
];
// A figure breaks only at its spaces: a split number reads as two figures.
const NUMBER_SPLIT =
  /overflow-wrap\s*:\s*(?:anywhere|break-word)|word-break\s*:\s*break-all/;
// A number cannot break, so this one rule may end an over-wide one in "…".
const FIGURE_LAST_RESORT = "design-system/atoms.css .stat-card-value";

/** Every rule that clamps, cuts or splits the name or the figure. */
export function cutsOfWholeText(all: readonly Rule[]): string[] {
  return all.flatMap((rule) =>
    alternativesOf(rule.selector).flatMap((selector) => {
      const where = `${relative(srcDir, rule.file)} ${selector}`;
      const subject = classesOf(subjectOf(selector));
      if (!WHOLE.some((one) => subject.has(one))) return [];
      const cut =
        WRAP_CUT.test(rule.body) ||
        (subject.has("stat-card-value") && NUMBER_SPLIT.test(rule.body)) ||
        (END_CUT.test(rule.body) && where !== FIGURE_LAST_RESORT);
      return cut ? [where] : [];
    }),
  );
}

const isSheet = (file: CensusFile) => file.rel.endsWith(".css");

// Stories and tests may talk about the prop, so they are left out.
const isApp = (file: CensusFile) =>
  file.source !== undefined &&
  !/\.(stories|test)\.tsx$/.test(file.rel) &&
  (file.area === "extensions" || file.rel.endsWith(".tsx"));

export const rule: CensusRule = {
  name: "statcard",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        (isSheet(file) || isApp(file)),
    ),
  floors: (corpus) => {
    const modules = corpus.filter(isApp);
    return [
      ...(modules.length > 400 ? [] : ["fewer than 400 app modules were read"]),
      ...(modules.some((file) => file.area === "extensions")
        ? []
        : ["the census reached no extension frontend layer"]),
      ...(sheetRules(corpus.filter(isSheet)).length > 0
        ? []
        : ["no style rule was read"]),
    ];
  },
  across: (corpus) => {
    const rules = sheetRules(corpus.filter(isSheet));
    return [
      ...corpus
        .filter(isApp)
        .flatMap((file) =>
          file.source
            ? openLabels(file.source).map(
                (line) => `${file.rel}:${line}: openLabel`,
              )
            : [],
        ),
      ...figureInks(rules),
      ...cutsOfWholeText(rules).map((where) => `${where} cuts the reading`),
    ];
  },
  advice:
    "A reading's door says what STAT_CARD_ACTIONS says, so pass `opens`, not " +
    "an openLabel. Colour the figure only in atoms.css, in its tone's Text " +
    "token. Never clamp, nowrap, ellipsis or hide the overflow of the name " +
    "or the figure: fix a long reading in the layout.",
};
