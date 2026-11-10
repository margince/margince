// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The ink field is one dark plate in both themes, and the page's inks are
// tuned for a light page. inkfield.css lists every component that paints the
// plate and hands it the plate's inks. This rule reads the painters from the
// app's sheets, so a new one cannot land unlisted.

import { join } from "node:path";
import {
  declaredValues,
  type Rule,
  selectorList,
} from "../../scripts/lib/css-rules";
import {
  type CensusRule,
  frontendRoot,
  isAppSheet,
  sheetRules,
  srcDir,
} from "../tree";

export const INKFIELD_SHEET = join(srcDir, "design-system", "inkfield.css");

const squash = (selector: string) => selector.replace(/\s+/g, " ").trim();

/** The selectors inside the `:where()` or `:is()` a scope rule opens with. */
export const scopeOf = (selector: string) =>
  selectorList(/^:(?:where|is)\(([\s\S]*)\)/.exec(selector)?.[1] ?? "").map(
    squash,
  );

export const paintsThePlate = (rule: Rule) =>
  ["background", "background-color"].some((property) =>
    declaredValues(rule.body, property).some((value) =>
      value.includes("--bgInkField"),
    ),
  );

/** Every painter selector the scope does not list, one finding each. */
export function unlisted(
  painters: readonly Rule[],
  scope: readonly string[],
): string[] {
  return painters.flatMap((rule) =>
    selectorList(rule.selector)
      .map(squash)
      .filter((selector) => !scope.includes(selector))
      .map(
        (selector) =>
          `${rule.file.slice(frontendRoot.length + 1)}: ${selector}`,
      ),
  );
}

export const rule: CensusRule = {
  name: "inkfield",
  corpus: (tree) => tree.files.filter(isAppSheet),
  floors: (corpus, tree) => [
    ...(tree.at(INKFIELD_SHEET)
      ? []
      : ["design-system/inkfield.css is not in the tree"]),
    ...(sheetRules(corpus).filter(paintsThePlate).length >= 6
      ? []
      : ["fewer than 6 rules paint the ink field plate"]),
  ],
  across: (corpus, tree) => {
    const owner = tree.at(INKFIELD_SHEET);
    const [tokenRule] = owner ? sheetRules([owner]) : [];
    const scope = tokenRule ? scopeOf(tokenRule.selector) : [];
    return unlisted(sheetRules(corpus).filter(paintsThePlate), scope);
  },
  advice: "add the selector to inkfield.css",
};
