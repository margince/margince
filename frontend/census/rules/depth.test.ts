// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { registerDrift, rule, spelledShadows } from "./depth.census";

describe("a spelled shadow", () => {
  const found = (css: string) => spelledShadows("a.css", css);

  it("passes the tokens, the suppression and a flat inset ring", () => {
    expect(
      found(
        ".a { box-shadow: var(--shadow-rest); } .b { box-shadow: none; } " +
          ".c { box-shadow: inset 3px 0 0 var(--accent); }",
      ),
    ).toEqual([]);
  });

  it("reads a blurred layer behind a flat ring, and a hand-spelled well", () => {
    expect(
      found(
        ".a { box-shadow: inset 0 0 0 1px red, 0 0 44px -22px blue; }\n" +
          ".b { box-shadow: inset 0 1px 2px black; }",
      ),
    ).toEqual([
      "a.css: box-shadow: inset 0 0 0 1px red, 0 0 44px -22px blue",
      "a.css: box-shadow: inset 0 1px 2px black",
    ]);
  });

  it("reads a statement the formatter wrapped, and no prose about one", () => {
    expect(
      found(
        "/* box-shadow: 0 1px red; */\n.a {\n  box-shadow:\n    0 1px 2px red,\n    0 0 1px blue;\n}",
      ),
    ).toEqual(["a.css: box-shadow: 0 1px 2px red, 0 0 1px blue"]);
  });

  it("counts a register entry once per spelling", () => {
    expect(registerDrift(["x", "x"], ["x"])).toEqual(["x"]);
    expect(registerDrift([], ["x"])).toEqual([
      "x is registered and no sheet spells it",
    ]);
  });
});

describe("the depth rule", () => {
  it("reports a planted shadow through its own corpus and across", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "features", "p.css"),
        area: "src",
        text: ".a { box-shadow: 0 2px 4px black; }\n",
      },
    ]);
    expect(findings[0]).toBe("src/features/p.css: box-shadow: 0 2px 4px black");
  });
});
