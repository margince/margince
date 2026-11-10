// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A box that scrolls must be the containing block of what it scrolls. A
// `.sr-only` span is absolutely positioned. In a static scroller it escapes
// the clip and widens the page: a company's tab strip panned 59px at 390px.
//
// Not covered: a box that only clips (`hidden`, `clip`). It is not built to
// hold content past its edge, and most such boxes cut one line of text.

import { selectorList, subjectClasses } from "../../scripts/lib/css-rules";
import { rulesIn } from "../../src/testing/css";
import type { CensusFile, CensusRule } from "../tree";
import { isAppSheet } from "../tree";

const POSITIONED =
  /(?:^|[;\s])position\s*:\s*(?:relative|absolute|fixed|sticky)\b/;

type Styled = {
  file: string;
  selector: string;
  conditions: string[];
  body: string;
};

function styledRules(sheets: readonly CensusFile[]): Styled[] {
  return sheets.flatMap((file) =>
    rulesIn(file.text).flatMap((rule) =>
      selectorList(rule.selector).map((selector) => ({
        file: file.rel,
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

function scrollsHorizontally(body: string): boolean {
  const axes = {
    inline: { value: "visible", important: false },
    block: { value: "visible", important: false },
  };
  const set = (axis: keyof typeof axes, value: string, important: boolean) => {
    if (important || !axes[axis].important) axes[axis] = { value, important };
  };
  for (const [, axis, raw] of body.matchAll(
    /(?:^|[;\s])overflow(-x|-y|-inline|-block)?\s*:\s*([^;]+)/g,
  )) {
    const important = /!important\s*$/.test(raw);
    const values = raw
      .replace(/\s*!important\s*$/, "")
      .trim()
      .split(/\s+/);
    const first = values[0] ?? "visible";
    if (!axis) {
      set("inline", first, important);
      set("block", values[1] ?? first, important);
    } else if (axis === "-x" || axis === "-inline") {
      set("inline", first, important);
    } else {
      set("block", first, important);
    }
  }
  const scrolls = (value: string) => value === "auto" || value === "scroll";
  return (
    scrolls(axes.inline.value) ||
    (axes.inline.value === "visible" && scrolls(axes.block.value))
  );
}

const scrollersIn = (all: readonly Styled[]) =>
  all.filter((rule) => scrollsHorizontally(rule.body));

export const rule: CensusRule = {
  name: "scrollercontainingblock",
  corpus: (tree) => tree.files.filter(isAppSheet),
  floors: (corpus) =>
    scrollersIn(styledRules(corpus)).length >= 15
      ? []
      : ["fewer than 15 scrollers were found across the app's sheets"],
  across: (corpus) => {
    const all = styledRules(corpus);
    return scrollersIn(all)
      .filter((scroller) => !all.some((rule) => positionedBy(scroller, rule)))
      .map((scroller) =>
        [scroller.file, ...scroller.conditions, scroller.selector].join(": "),
      );
  },
  advice:
    "A box that scrolls is positioned, so its overflow clips the hidden text inside it.",
};
