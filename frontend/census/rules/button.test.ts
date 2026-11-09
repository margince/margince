// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { bareTwinOf, namesLoneBare, rule, sizedGlyphs } from "./button.census";

describe("the .iconbtn and .btn.btn-bare pair", () => {
  it.each([
    [".btn.btn-bare", false],
    [".btn.btn-bare:hover svg", false],
    [".btn.btn-barely", false],
    [".btn-bare svg", true],
    [".notifbell .btn-bare", true],
    [".btn-bare.btn:hover", true],
    [".xbtn.btn-bare", true],
    [".btn.btn-bare + .btn-bare", true],
  ])("reads %j as naming a lone .btn-bare: %s", (selector, lone) => {
    expect(namesLoneBare(selector)).toBe(lone);
  });

  it("spells each selector for its twin, and nothing for a stranger", () => {
    expect(bareTwinOf(".row .iconbtn:hover")).toBe(".row .btn.btn-bare:hover");
    expect(bareTwinOf(".btn.btn-bare svg")).toBe(".iconbtn svg");
    expect(bareTwinOf(".iconbtnx")).toBeUndefined();
  });
});

describe("a glyph inside a control", () => {
  const found = (markup: string) =>
    sizedGlyphs(
      "x.tsx",
      parseSource("x.tsx", `export const F = () => (${markup});\n`),
    );

  it("finds a size prop or an inline size on a wrapped glyph", () => {
    expect(found("<Button><Plus size={14} /></Button>")).toEqual([
      "x.tsx:1: <Plus size={14}>",
    ]);
    expect(
      found('<a className="btn"><Plus style={{ width: 12 }} /></a>'),
    ).toHaveLength(1);
  });

  it("leaves the control's own props and a glyph outside any control", () => {
    expect(found('<Button style={{ width: "100%" }}>Go</Button>')).toEqual([]);
    expect(found("<div><Plus size={14} /></div>")).toEqual([]);
  });
});

describe("the button rule", () => {
  it("reports planted offences through its own corpus and across", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "features", "planted.css"),
        area: "src",
        text: ".row .iconbtn { color: red; }\n.x .btn-bare { color: red; }\n",
      },
      {
        path: join(srcDir, "features", "planted.tsx"),
        area: "src",
        text: "export const F = () => <IconAction><Bell size={16} /></IconAction>;\n",
      },
    ]);
    expect(findings).toEqual([
      "src/features/planted.css: .row .iconbtn has no .iconbtn / .btn.btn-bare twin",
      "src/features/planted.css: .x .btn-bare names .btn-bare without .btn",
      "src/features/planted.tsx:1: <Bell size={16}>",
    ]);
  });
});
