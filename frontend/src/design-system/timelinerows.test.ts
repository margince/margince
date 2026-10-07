// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  alternativesOf,
  appStylesheets,
  classesOf,
  compounds,
  rulesOf,
  selectorList,
} from "../../scripts/lib/css-rules";

// A row rule that reaches its `li` by anything but `>` also paints every list
// nested in a row, and the story plays that measure it run in no required lane.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

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
// the run, and that one must hang off the `.timeline` compound by `>`.
function judge(selector: string): Verdict {
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

const sheets = appStylesheets(frontendRoot);

describe("a timeline row rule", () => {
  it.each([
    [".timeline li", [".timeline li"]],
    [
      ".timeline.timeline-plain li + li",
      [".timeline.timeline-plain li + li", ".timeline.timeline-plain li + li"],
    ],
    [".timeline .x li", [".timeline .x li"]],
    [
      ".timeline .tl-thread-messages > li",
      [".timeline .tl-thread-messages > li"],
    ],
    [":is(.timeline, .other) li", [".timeline li"]],
  ])("refuses %s, which reaches a list nested in a row", (selector, leaks) => {
    expect(judge(selector).leaks).toEqual(leaks);
  });

  it.each([
    ".timeline > li",
    ".timeline.timeline-plain > li + li",
    ".timeline > li:first-child > .tl-rail::before",
    ".timeline-header li",
  ])("accepts %s", (selector) => {
    expect(judge(selector).leaks).toEqual([]);
  });

  it("reaches only the list's own rows, in every sheet the app ships", () => {
    const rows = rulesOf(sheets).flatMap((rule) =>
      selectorList(rule.selector).map((selector) => ({
        at: `${relative(frontendRoot, rule.file)}: ${selector}`,
        ...judge(selector),
      })),
    );
    // Read again as plain text, so a parser that stops recognising a row
    // selector shows up as a short census instead of a clean pass.
    const written = sheets.flatMap((file) => [
      ...readFileSync(file, "utf8")
        .replace(/\/\*[\s\S]*?\*\//g, "")
        .matchAll(/\.timeline(?![\w-])[^{},;]*?(?<![\w-])li(?![\w-])/g),
    ]).length;
    expect(sheets.length).toBeGreaterThan(0);
    expect(written).toBeGreaterThan(0);
    expect(rows.reduce((sum, row) => sum + row.rows, 0)).toBeGreaterThanOrEqual(
      written,
    );
    expect(rows.flatMap((row) => row.leaks.map(() => row.at))).toEqual([]);
  });
});
