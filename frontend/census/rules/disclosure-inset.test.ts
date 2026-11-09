// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { mentionsIn, rule } from "./disclosure-inset.census";

describe("a mention of .disclosure-body", () => {
  it("reads each selector of a list, and no commented-out rule", () => {
    expect(
      mentionsIn(
        "a.css",
        "/* .x .disclosure-body { padding: 0; } */\n.a, .b > .disclosure-body { padding: 0; }",
      ),
    ).toEqual([{ sheet: "a.css", selector: ".b > .disclosure-body" }]);
  });
});

describe("the disclosure-inset rule", () => {
  it("reports a descendant inset and passes a child one", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "p.css"),
          area: "src",
          text:
            ".disclosure-body { padding: 0; }\n" +
            ".rail .sect .disclosure-body { padding: 8px; }\n" +
            ".rail .sect > .disclosure-body { padding: 8px; }\n",
        },
      ]),
    ).toEqual([
      "src/features/p.css: `.rail .sect .disclosure-body` insets the body by descent",
    ]);
  });
});
