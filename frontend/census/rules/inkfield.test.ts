// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir } from "../tree";
import { INKFIELD_SHEET, rule, scopeOf } from "./inkfield.census";

const area: Area = "src";
const scope =
  ":where(.plate, .hero .band) { --textPrimary: var(--inkFieldText); }";

describe("the ink field's scope", () => {
  it("reads the selectors a scope rule lists", () => {
    expect(scopeOf(":where(.plate,\n  .hero  .band)")).toEqual([
      ".plate",
      ".hero .band",
    ]);
  });

  it("reports a painter the scope does not list, and passes a listed one", () => {
    expect(
      plantedFindings(rule, [
        { path: INKFIELD_SHEET, area, text: scope },
        {
          path: join(srcDir, "features", "planted.css"),
          area,
          text: [
            ".plate { background: var(--bgInkField); }",
            ".hero .band { background-color: var(--bgInkField); }",
            ".stray { background: var(--bgInkField); }",
            ".other { background: var(--bgPage); }",
          ].join("\n"),
        },
      ]),
    ).toEqual(["src/features/planted.css: .stray"]);
  });
});
