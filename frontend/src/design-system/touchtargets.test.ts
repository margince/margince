// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { declaredValues } from "../../scripts/lib/css-rules";
import { rulesIn } from "../testing/css";

// census/rules/touchtargets.census.ts holds every touch layer to 44px. These
// cases hold the two layers that sit a few pixels from another target.

const here = dirname(fileURLToPath(import.meta.url));
const rulesOf = (sheet: string) =>
  rulesIn(readFileSync(join(here, sheet), "utf8"));

function coarseLayer(sheet: string, control: string): string {
  const found = rulesOf(sheet).filter(
    (rule) =>
      rule.selector === `${control}::before` &&
      rule.conditions.includes("@media (pointer: coarse)"),
  );
  expect(found, `${control}::before under a coarse pointer`).toHaveLength(1);
  return found[0]?.body ?? "";
}

describe("a touch layer beside another target", () => {
  // The name's link starts one gap past the box; a layer reaching past the gap
  // would take the press that opens the record.
  it("ends the row box's layer where the name's link starts", () => {
    const gap = rulesOf("listtable.css")
      .filter((rule) => rule.selector === ".lt-select")
      .flatMap((rule) => declaredValues(rule.body, "margin-inline-end"));
    expect(gap).toHaveLength(1);
    expect(
      declaredValues(
        coarseLayer("listtable.css", ".lt-select"),
        "inset-inline-end",
      ),
    ).toEqual([`calc(-1 * ${gap[0]})`]);
  });

  // A value and its pencil sit a few pixels from the ⓘ on its line.
  it("keeps the ⓘ layer off its neighbours on its line", () => {
    expect(
      declaredValues(coarseLayer("infotip.css", ".info-tip"), "inset-inline"),
    ).toEqual(["0"]);
  });
});
