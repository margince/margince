// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import type { Rule } from "../../scripts/lib/css-rules";
import { type Area, plantedFindings, srcDir } from "../tree";
import { floatingLayers, rule, seeThrough } from "./floatinglayers.census";

const probe = (selector: string, body: string): Rule => ({
  file: "probe.css",
  selector,
  body,
});

describe("a layer that hangs over the page hides it", () => {
  it("finds a layer by its shadow and its position, across rules", () => {
    expect(
      floatingLayers([
        probe(".pop", "position: fixed; box-shadow: var(--shadow-pop)"),
        probe(".split", "position: absolute"),
        probe(".split", "box-shadow: 0 0 0 1px red, var(--shadow-pop)"),
        probe(".flow", "position: relative; box-shadow: var(--shadow-pop)"),
        probe(".lifted", "z-index: 1; box-shadow: var(--shadow-pop)"),
        probe(".flat", "position: fixed; box-shadow: var(--shadow-rest)"),
      ]),
    ).toEqual([".lifted", ".pop", ".split"]);
  });

  it("reads a ground from a compounded class and under an image layer", () => {
    expect(
      seeThrough([
        probe(":where(.bar)", "background-color: var(--bgPage)"),
        probe(".bar", "background-color: var(--pane)"),
        probe(".bar.open", "position: fixed; box-shadow: var(--shadow-pop)"),
        probe(".bar.open", "position: static; background: transparent"),
        probe(".orb", "position: absolute; box-shadow: var(--shadow-pop)"),
        probe(".orb", "background: radial-gradient(red, blue), var(--bgPage)"),
        probe(".bare", "position: fixed; box-shadow: var(--shadow-pop)"),
      ]),
    ).toEqual([".bar.open: var(--pane) in dark", ".bare: no ground"]);
  });

  it("reports a planted see-through layer through its own corpus", () => {
    const area: Area = "src";
    const sheet = (name: string, text: string) => ({
      path: join(srcDir, "features", name),
      area,
      text,
    });
    expect(
      plantedFindings(rule, [
        sheet(
          "planted.css",
          "/* a comment */\n.bare { position: fixed; box-shadow: var(--shadow-pop); }",
        ),
        sheet("planted.tsx", "export const A = 1;"),
      ]),
    ).toEqual([".bare: no ground"]);
  });
});
