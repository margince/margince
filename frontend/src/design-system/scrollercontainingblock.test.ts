// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  appStylesheets,
  selectorList,
  subjectClasses,
} from "../../scripts/lib/css-rules";
import { rulesIn } from "../testing/css";

// A box that scrolls must be the containing block of what it scrolls. A
// `.sr-only` span is absolutely positioned. In a static scroller it escapes
// the clip and widens the page: a company's tab strip panned 59px at 390px.
//
// Not covered: a box that only clips (`hidden`, `clip`). It is not built to
// hold content past its edge, and most such boxes cut one line of text.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

const SCROLLS =
  /(?:^|[;\s])overflow(?:-x|-inline)?\s*:[^;]*\b(?:auto|scroll)\b/;
const POSITIONED =
  /(?:^|[;\s])position\s*:\s*(?:relative|absolute|fixed|sticky)\b/;

type Styled = {
  file: string;
  selector: string;
  conditions: string[];
  body: string;
};

function styledRules(): Styled[] {
  return appStylesheets(frontendRoot).flatMap((file) =>
    rulesIn(readFileSync(file, "utf8")).flatMap((rule) =>
      selectorList(rule.selector).map((selector) => ({
        file: relative(frontendRoot, file).replaceAll("\\", "/"),
        selector: selector.trim(),
        conditions: rule.conditions.filter(
          (condition) => !condition.startsWith("@layer"),
        ),
        body: rule.body,
      })),
    ),
  );
}

// A positioning rule counts when it holds wherever the scroller does: its
// at-rules are the scroller's own or fewer. A layer only orders the cascade,
// so it is not a condition. It names the scroller itself, or
// one class the scroller's element also carries (a popover's own panel class).
function positionedBy(scroller: Styled, rule: Styled): boolean {
  const holds = rule.conditions.every(
    (condition, at) => scroller.conditions[at] === condition,
  );
  const names =
    rule.selector === scroller.selector ||
    [...subjectClasses(scroller.selector)].some(
      (name) => rule.selector === `.${name}`,
    );
  return holds && names && POSITIONED.test(rule.body);
}

describe("a sideways scroller", () => {
  const all = styledRules();
  const scrollers = all.filter((rule) => SCROLLS.test(rule.body));

  it("is found across the whole app, not a corner of it", () => {
    expect(scrollers.length).toBeGreaterThanOrEqual(15);
  });

  it("is positioned, so its overflow clips the hidden text inside it", () => {
    const loose = scrollers
      .filter((scroller) => !all.some((rule) => positionedBy(scroller, rule)))
      .map((scroller) =>
        [scroller.file, ...scroller.conditions, scroller.selector].join(": "),
      );
    expect(loose).toEqual([]);
  });
});
