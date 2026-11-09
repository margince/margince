// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule, strayDeclarations } from "./stylesheetnamespace.census";

describe("a screen's class namespace", () => {
  it("lets the home sheet declare its own classes", () => {
    expect(
      strayDeclarations("src/components/auth-core.css", ".auth-shell { }"),
    ).toEqual([]);
  });

  it("reads past a class cited in a comment", () => {
    expect(
      strayDeclarations("src/app.css", "/* see .auth-shell */\n.a { }"),
    ).toEqual([]);
  });

  it("reports a planted stray declaration through its own corpus and visit", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "onboarding", "planted.css"),
          area: "src",
          text: ".auth-shell { margin: 4px; }",
        },
      ]),
    ).toEqual([
      "src/features/onboarding/planted.css: declares .auth-shell, and the auth-* namespace belongs to components/auth-core.css",
    ]);
  });
});
