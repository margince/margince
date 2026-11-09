// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// `Heading` brings no margin: heading.css zeroes the UA's at no weight, so a
// caller's own rule sets the space. A margin on `.heading` at class weight
// races every caller's rule, in any sheet.

import { join, relative } from "node:path";
import { resolveNesting, subjectOf } from "../../scripts/lib/css-rules";
import { type CssRule, rulesIn } from "../../src/testing/css";
import { type CensusRule, isAppSheet, srcDir } from "../tree";

const HEADING_SHEET = join(srcDir, "design-system", "heading.css");

function resolvedSelectors(rule: CssRule): string[] {
  return [...rule.parents, rule.selector].reduce<string[]>(resolveNesting, []);
}

type MarginDeclaration = { selector: string; value: string; line: number };

export function marginDeclarations(css: string): MarginDeclaration[] {
  return rulesIn(css).flatMap((rule) =>
    [
      ...rule.body.matchAll(/(?:^|[;{])\s*margin[a-z-]*\s*:\s*([^;]+)/gi),
    ].flatMap(([, value]) =>
      resolvedSelectors(rule).map((selector) => ({
        selector,
        value: value.trim(),
        line: rule.line,
      })),
    ),
  );
}

// A margin whose subject's only class is `.heading` races every caller's rule.
export function weightedHeadingMargins(css: string): string[] {
  return marginDeclarations(css)
    .filter(({ selector }) => {
      const subject = subjectOf(selector);
      const classes = new Set(
        subject
          .replaceAll(/:[\w-]+\((?:[^()]|\([^()]*\))*\)|\[[^\]]*\]/g, "")
          .match(/\.[\w-]+/g),
      );
      return classes.size === 1 && classes.has(".heading");
    })
    .map(({ selector, line }) => `${selector} (line ${line})`);
}

export const rule: CensusRule = {
  name: "heading",
  corpus: (tree) => tree.files.filter(isAppSheet),
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["the stylesheet walk came back small"]),
    ...(corpus.some((file) => file.path === HEADING_SHEET)
      ? []
      : ["the stylesheet walk did not reach design-system/heading.css"]),
  ],
  visit: (file) =>
    weightedHeadingMargins(file.text).map(
      (hit) => `${relative(srcDir, file.path)}: ${hit}`,
    ),
  advice:
    "a margin on .heading at class weight: leave every heading's margin to its caller.",
};
