// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir } from "../tree";
import { rule, weightedHeadingMargins } from "./heading.census";

describe("a heading's margin is its caller's", () => {
  it("tells a margin on a bare heading from a caller's own", () => {
    expect(weightedHeadingMargins(".heading { margin: 0 }")).toEqual([
      ".heading (line 1)",
    ]);
    expect(
      weightedHeadingMargins(
        '.panel h2.heading[data-size="large"] { MARGIN-BOTTOM: 4px }',
      ),
    ).toEqual(['.panel h2.heading[data-size="large"] (line 1)']);
    expect(weightedHeadingMargins(".panel { .heading { margin: 0 } }")).toEqual(
      [".panel .heading (line 1)"],
    );
    expect(
      weightedHeadingMargins(":where(.lead), .heading { margin: 0 }"),
    ).toEqual([".heading (line 1)"]);
    expect(weightedHeadingMargins(":where(.heading) { margin: 0 }")).toEqual(
      [],
    );
    expect(
      weightedHeadingMargins(".heading.modal-title { margin-bottom: 4px }"),
    ).toEqual([]);
  });

  it("reports a planted weighted margin through its own corpus", () => {
    const area: Area = "src";
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.css"),
          area,
          text: ".card .heading { margin-top: 4px }",
        },
      ]),
    ).toEqual(["features/planted.css: .card .heading (line 1)"]);
  });
});
