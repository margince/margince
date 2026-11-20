// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// `--overlayScrim` is the one scrim colour, and tokens.test.ts holds that it
// dims the page in both themes. So no layer that dims the page names a colour
// of its own. A rule whose subject names a scrim, an overlay or a backdrop
// paints the token, or `none` for a layer that does not dim.

import {
  declaredValues,
  rulesOfText,
  subjectsOf,
} from "../../scripts/lib/css-rules";
import { type CensusFile, type CensusRule, isAppSheet } from "../tree";

const LAYER = /scrim|overlay|backdrop/i;
const ALLOWED = new Set(["var(--overlayScrim)", "none"]);

const rulesOf = (sheets: readonly CensusFile[]) =>
  sheets.flatMap((sheet) => rulesOfText(sheet.rel, sheet.text));

export const rule: CensusRule = {
  name: "overlayscrim",
  corpus: (tree) => tree.files.filter(isAppSheet),
  floors: (corpus) =>
    rulesOf(corpus).some((rule) => rule.selector === ".overlay")
      ? []
      : ["the .overlay rule was not read"],
  across: (corpus) =>
    rulesOf(corpus)
      .filter((rule) => subjectsOf(rule.selector).some((s) => LAYER.test(s)))
      .flatMap((rule) =>
        ["background", "background-color"].flatMap((property) =>
          declaredValues(rule.body, property)
            .filter((value) => !ALLOWED.has(value))
            .map((value) => `${rule.file}: ${rule.selector} paints ${value}`),
        ),
      ),
  advice:
    "A scrim, overlay or backdrop paints var(--overlayScrim), the colour that dims the page in both themes.",
};
