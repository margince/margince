// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// No `:has()` argument may hold `:hover`. While one exists anywhere in the
// page, Chromium restyles the whole tree on every hover change. A board column
// of 500 cards then spent hundreds of milliseconds per scrolled frame. Mark the
// element in script instead, as lib/pointertarget.ts and DataTable's detail row do.

import { functionCall, rulesOfText } from "../../scripts/lib/css-rules";
import type { CensusFile, CensusRule } from "../tree";

/**
 * Each `:has(...)` call in a selector whose argument names `:hover`. CSS
 * pseudo-class names ignore case, so `:HAS(.x:HOVER)` counts too.
 */
export function hoverInHas(selector: string): string[] {
  const lower = selector.toLowerCase();
  const found: string[] = [];
  for (let at = lower.indexOf(":has("); at >= 0; ) {
    const call = functionCall(lower.slice(at + 1));
    if (call?.args.some((arg) => arg.includes(":hover"))) {
      found.push(selector.slice(at, at + 1 + call.end));
    }
    at = lower.indexOf(":has(", at + 1);
  }
  return found;
}

const isStylesheet = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.rel.endsWith(".css");

export const rule: CensusRule = {
  name: "hover-in-has",
  corpus: (tree) => tree.files.filter(isStylesheet),
  floors: (corpus) =>
    corpus.length > 40 ? [] : ["fewer than 40 stylesheets were read"],
  visit: (file) =>
    rulesOfText(file.rel, file.text).flatMap(({ selector }) =>
      hoverInHas(selector).map(
        (call) => `${file.rel}: \`${selector.trim()}\` holds \`${call}\``,
      ),
    ),
  advice:
    "Keep `:hover` out of `:has()`. Set an attribute from a pointer event on " +
    "the element the style is for, and select that attribute.",
};
