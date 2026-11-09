// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A row rule that reaches its `li` by anything but `>` also reaches `li` items
// nested inside a row. The story plays that measure it run in no required lane.

import { relative } from "node:path";
import {
  alternativesOf,
  classesOf,
  compounds,
} from "../../scripts/lib/css-rules";
import {
  type CensusFile,
  type CensusRule,
  frontendRoot,
  sheetRules,
} from "../tree";

type Verdict = { rows: number; leaks: string[] };

function combinatorsOf(selector: string, parts: readonly string[]): string[] {
  const between: string[] = [];
  let at = selector.indexOf(parts[0]) + parts[0].length;
  for (const part of parts.slice(1)) {
    const next = selector.indexOf(part, at);
    between.push(selector.slice(at, next).trim() || " ");
    at = next + part.length;
  }
  return between;
}

const isLi = (compound: string) => /^li(?![\w-])/i.test(compound);
const isTimeline = (compound: string) => classesOf(compound).has("timeline");

// A sibling of a row is a row, so `+` and `~` walk back to the first `li` of
// the run. That one must hang off the `.timeline` compound by `>`.
export function judge(selector: string): Verdict {
  const verdict: Verdict = { rows: 0, leaks: [] };
  for (const alternative of alternativesOf(selector)) {
    const parts = compounds(alternative);
    const joins = combinatorsOf(alternative, parts);
    parts.forEach((part, index) => {
      if (!isLi(part) || !parts.slice(0, index).some(isTimeline)) {
        return;
      }
      let first = index;
      while (first > 0 && "+~".includes(joins[first - 1])) {
        first--;
      }
      if (joins[first - 1] === ">" && isTimeline(parts[first - 1] ?? "")) {
        verdict.rows++;
      } else {
        verdict.leaks.push(alternative);
      }
    });
  }
  return verdict;
}

// One count per selector alternative that reaches a `.timeline` `li`, as a row
// or as a leak; the plain-text count below matches once per alternative.
export function parsedReach(selectors: readonly string[]): number {
  return selectors
    .flatMap((selector) => alternativesOf(selector))
    .filter((alternative) => {
      const parts = compounds(alternative);
      return parts.some(
        (part, index) => isLi(part) && parts.slice(0, index).some(isTimeline),
      );
    }).length;
}

export function writtenReach(css: string): number {
  return [
    ...css
      .replace(/\/\*[\s\S]*?\*\//g, "")
      .matchAll(/\.timeline(?![\w-])[^{},;]*?(?<![\w-])li(?![\w-])/g),
  ].length;
}

const isSheet = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.rel.endsWith(".css");

export const rule: CensusRule = {
  name: "timelinerows",
  corpus: (tree) => tree.files.filter(isSheet),
  // Read again as plain text, so a parser that stops recognising a row
  // selector shows as a short census, not a clean pass.
  floors: (corpus) => {
    const written = corpus.reduce(
      (sum, file) => sum + writtenReach(file.text),
      0,
    );
    const parsed = parsedReach(sheetRules(corpus).map((one) => one.selector));
    return [
      ...(written > 0 ? [] : ["no rule reaches a .timeline li"]),
      ...(parsed === written
        ? []
        : [`${written} row selectors are written but ${parsed} were parsed`]),
    ];
  },
  across: (corpus) =>
    sheetRules(corpus).flatMap((one) =>
      judge(one.selector).leaks.map(
        () => `${relative(frontendRoot, one.file)}: ${one.selector}`,
      ),
    ),
  advice:
    "Reach a timeline row as `.timeline > li`. Any other combinator also " +
    "reaches the li items nested inside a row.",
};
