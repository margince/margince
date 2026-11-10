// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { railGrounds, rule } from "./sidebar-ground.census";

describe("the sidebar-ground census", () => {
  it("reads the panel's own ground, not a row's inside it", () => {
    const grounds = railGrounds([
      { file: "a.css", selector: ".rail", body: "background: var(--pane);" },
      { file: "a.css", selector: ".rail.wide", body: "background-color: red;" },
      { file: "a.css", selector: ".rail .row", body: "background: blue;" },
    ]);
    expect(grounds).toEqual([
      { selector: ".rail", value: "var(--pane)" },
      { selector: ".rail.wide", value: "red" },
    ]);
  });
});

describe("the sidebar-ground rule", () => {
  it("reports a rail grounded on the ink field", () => {
    const path = join(srcDir, "app", "planted.css");
    const text = ".rail { background: var(--bgInkField); }\n";
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toEqual([
      ".rail { background: var(--bgInkField) }",
    ]);
  });
});
