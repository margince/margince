// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir } from "../tree";
import { rule } from "./scrollercontainingblock.census";

const area: Area = "src";
const sheet = (text: string) => [
  { path: join(srcDir, "features", "planted.css"), area, text },
];

describe("a sideways scroller", () => {
  it.each([
    "overflow-y: auto",
    "overflow-block: scroll",
    "overflow: visible auto",
    "overflow-x: hidden; overflow: auto",
    "overflow: auto !important; overflow-x: hidden",
    "overflow-y: auto !important; overflow: visible",
  ])("covers implicit horizontal scrolling: %s", (declarations) => {
    expect(plantedFindings(rule, sheet(`.strip { ${declarations}; }`))).toEqual(
      ["src/features/planted.css: .strip"],
    );
  });

  it.each([
    "overflow-y: auto; overflow-x: hidden",
    "overflow-inline: clip; overflow-block: scroll",
    "overflow: hidden auto",
    "overflow-x: hidden !important; overflow: auto",
  ])("keeps an explicit horizontal clip: %s", (declarations) => {
    expect(plantedFindings(rule, sheet(`.strip { ${declarations}; }`))).toEqual(
      [],
    );
  });
  it("is reported when nothing positions it", () => {
    expect(
      plantedFindings(rule, sheet(".strip { overflow-x: auto; }")),
    ).toEqual(["src/features/planted.css: .strip"]);
  });

  it("is reported when it is positioned only under a narrower query", () => {
    expect(
      plantedFindings(
        rule,
        sheet(
          ".strip { overflow: auto; }\n@media (width < 40rem) { .strip { position: relative; } }",
        ),
      ),
    ).toEqual(["src/features/planted.css: .strip"]);
  });

  it("passes when it or a class it carries is positioned", () => {
    expect(
      plantedFindings(
        rule,
        sheet(".strip { overflow-x: scroll; position: relative; }"),
      ),
    ).toEqual([]);
    expect(
      plantedFindings(
        rule,
        sheet(".pop .list { overflow: auto; }\n.list { position: relative; }"),
      ),
    ).toEqual([]);
  });

  it("leaves a box that only clips alone", () => {
    expect(
      plantedFindings(rule, sheet(".strip { overflow: hidden; }")),
    ).toEqual([]);
  });
});
