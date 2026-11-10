// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The sidebar is glass over the page, never a dark green column and never the
// accent (the shell in DESIGN.md). The dark plate (`--bgInkField`) is for what
// passes over the page, and is easy to mistake for the sidebar's.

import {
  declaredValues,
  type Rule,
  rulesOfText,
  subjectClasses,
  subjectsOf,
} from "../../scripts/lib/css-rules";
import type { CensusFile, CensusRule } from "../tree";

type Ground = Readonly<{ selector: string; value: string }>;

/**
 * The backgrounds the panel itself declares, in any state or layout: a rule
 * whose subject is `.rail`, not a row or tooltip inside it.
 */
export function railGrounds(rules: readonly Rule[]): Ground[] {
  return rules
    .filter(
      (rule) =>
        subjectsOf(rule.selector).length === 1 &&
        subjectClasses(rule.selector).has("rail") &&
        !/\s/.test(rule.selector.trim()),
    )
    .flatMap((rule) =>
      ["background", "background-color"].flatMap((property) =>
        declaredValues(rule.body, property).map((value) => ({
          selector: rule.selector,
          value,
        })),
      ),
    );
}

const groundsOf = (corpus: readonly CensusFile[]) =>
  railGrounds(corpus.flatMap((file) => rulesOfText(file.path, file.text)));

export const rule: CensusRule = {
  name: "sidebar-ground",
  // Every sheet the bundle ships: the core's and each extension layer's.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.path.endsWith(".css"),
    ),
  floors: (corpus) =>
    groundsOf(corpus).length >= 1 ? [] : ["no background on .rail was found"],
  across: (corpus) =>
    groundsOf(corpus)
      .filter(({ value }) => /--(bgInkField|accent)/.test(value))
      .map(({ selector, value }) => `${selector} { background: ${value} }`),
  advice: "The sidebar is glass: `--pane` over the page.",
};
