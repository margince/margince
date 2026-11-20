// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./touchtargets.census";

const tokens = {
  path: join(srcDir, "design-system", "tokens.css"),
  area: "src" as const,
  text: ":root { --controlHeight: 32px; --gap: 12px; }",
};

const planted = (layer: string) =>
  plantedFindings(rule, [
    tokens,
    {
      path: join(srcDir, "planted.css"),
      area: "src",
      text: `@media (pointer: coarse) {
  .mark { position: relative; }
  .mark::before { content: ""; position: absolute; ${layer} }
}`,
    },
  ]);

describe("a touch layer under a coarse pointer", () => {
  it("passes a layer of its own 44px size", () => {
    expect(planted("inline-size: 44px; block-size: 44px;")).toEqual([]);
  });

  it("passes the centred inset that grows the control to 44px", () => {
    expect(planted("inset: calc((100% - max(44px, 100%)) / 2);")).toEqual([]);
  });

  it("holds only the height of a layer that keeps the control's width", () => {
    expect(
      planted(
        "inset-block: calc((100% - max(44px, 100%)) / 2); inset-inline: 0;",
      ),
    ).toEqual([]);
  });

  it("reports a layer under 44px on either axis", () => {
    expect(planted("inline-size: 30px; block-size: 44px;")).toEqual([
      "src/planted.css .mark::before is 30px wide",
    ]);
    expect(planted("inset: calc((100% - max(40px, 100%)) / 2);")).toEqual([
      "src/planted.css .mark::before is 40px tall",
      "src/planted.css .mark::before is 40px wide",
    ]);
  });

  it("follows a custom property, over a control of --controlHeight", () => {
    expect(planted("inset: calc(-1 * var(--gap) / 2);")).toEqual([]);
    expect(planted("inset: calc(-1 * var(--gap) / 4);")).toEqual([
      "src/planted.css .mark::before is 38px tall",
    ]);
  });

  it("reports a layer whose size it cannot read", () => {
    expect(planted("inset: anchor(top);")).toEqual([
      "src/planted.css .mark::before has a height the census cannot read",
    ]);
  });

  it("does not read a layer outside a coarse pointer", () => {
    expect(
      plantedFindings(rule, [
        tokens,
        {
          path: join(srcDir, "planted.css"),
          area: "src",
          text: ".mark { position: relative; } .mark::before { inline-size: 1px; block-size: 1px; }",
        },
      ]),
    ).toEqual([]);
  });
});
