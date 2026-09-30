// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { rulesIn } from "../testing/css";

// A SUGGESTION'S VALUE IS NEVER SQUEEZED OUT BY ITS HINT.
//
// The list is the text box's width, and a text box in a three-field form row is
// narrow. A row that keeps value and hint on one line with the hint rigid hands
// the hint every pixel and shrinks the value to nothing — a model picker whose
// only option read as a price with no model beside it. So the row wraps: where
// both do not fit, the hint drops under the value instead of eating it.
//
// WHAT THIS READS: the declarations. jsdom applies no stylesheet, so nothing
// here measures a row; it pins the two declarations that decide the outcome.

const sheet = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), "suggestlist.css"),
  "utf8",
);

function declarationsOf(selector: string): string {
  return rulesIn(sheet)
    .filter((rule) => rule.selector.trim() === selector)
    .map((rule) => rule.body)
    .join("\n");
}

describe("a suggestion row in a narrow list", () => {
  it("wraps the hint under the value rather than keeping one line", () => {
    expect(declarationsOf(".suggest-option")).toMatch(/flex-wrap:\s*wrap/);
  });

  it("lets the hint yield its width instead of holding it rigid", () => {
    expect(declarationsOf(".suggest-option-hint")).not.toMatch(/flex:\s*none/);
    expect(declarationsOf(".suggest-option-hint")).toMatch(/min-width:\s*0/);
  });
});
