// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { srcDir, treeOf } from "../tree";
import { hoverInHas, rule } from "./hover-in-has.census";

describe("the hover-in-has rule", () => {
  it("finds :hover inside :has(), however deep, and nothing else", () => {
    expect(hoverInHas("*:hover:not(:has(:hover))")).toEqual([":has(:hover)"]);
    expect(hoverInHas("tr.open:has(+ tr.detail:hover) > td")).toEqual([
      ":has(+ tr.detail:hover)",
    ]);
    expect(hoverInHas(".a:has(.b:is(:focus, :hover))")).toEqual([
      ":has(.b:is(:focus, :hover))",
    ]);
    expect(hoverInHas(":HAS(:HOVER)")).toEqual([":HAS(:HOVER)"]);
    expect(hoverInHas(".a:Has(.x:hOvEr) .b")).toEqual([":Has(.x:hOvEr)"]);
    expect(hoverInHas(".card:has(.open):hover .arrow")).toEqual([]);
    expect(hoverInHas("[data-pointer-over]:hover")).toEqual([]);
  });

  it("fails a stylesheet that puts :hover in :has()", () => {
    const [sheet] = treeOf([
      {
        path: join(srcDir, "design-system", "planted.css"),
        area: "src",
        text: ".row:has(+ .detail:hover) > td { color: red; }\n.ok:hover { color: blue; }",
      },
    ]).files;
    if (!("visit" in rule) || sheet === undefined) {
      throw new Error("hover-in-has visits one stylesheet at a time");
    }
    expect(rule.visit(sheet, treeOf([]))).toEqual([
      "src/design-system/planted.css: `.row:has(+ .detail:hover) > td` holds `:has(+ .detail:hover)`",
    ]);
  });
});
