// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir } from "../tree";
import { rule } from "./page-measure.census";

const area: Area = "src";
const sheet = (text: string) => [
  { path: join(srcDir, "features", "planted.css"), area, text },
];

describe("no sheet gives the column a measure of its own", () => {
  it.each([
    [
      "a narrow page with a width of its own",
      ".wrap.narrow { max-width: 720px; }",
    ],
    [
      "a modifier inside a breakpoint, in any sheet",
      "@media (min-width: 1440px) { .deals .wrap.wide { max-width: 90rem; } }",
    ],
    ["a head capped alone", ".pagetitle.slim { margin: 0; max-width: 40ch; }"],
    ["the logical property", ".wrap { max-inline-size: 60rem; }"],
    ["one arm of an :is()", ":is(.wrap, .pane) { max-width: 50rem; }"],
    [
      "the measure bent",
      ".wrap { max-width: calc(var(--pageMeasure) - 2rem); }",
    ],
    ["the cap lifted", ".wrap.full { max-width: none; }"],
  ])("sees %s", (_case, css) => {
    expect(plantedFindings(rule, sheet(css))).toHaveLength(1);
  });

  it("leaves a cap on something inside the column alone", () => {
    expect(
      plantedFindings(
        rule,
        sheet(
          `.wrap .card { max-width: 40rem; }
           .wrap > .pagetitle-lede { max-width: 60ch; }
           .wrap.narrow { max-width: var( --pageMeasure ); }`,
        ),
      ),
    ).toEqual([]);
  });

  it("names the sheet and the cap it found", () => {
    expect(
      plantedFindings(rule, sheet(".wrap.narrow { max-width: 720px; }")),
    ).toEqual(["src/features/planted.css: .wrap.narrow { max-width: 720px }"]);
  });
});
