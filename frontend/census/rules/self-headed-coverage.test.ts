// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { headingFindings, rule } from "./self-headed-coverage.census";

describe("the self-headed-coverage census", () => {
  it("names a screen on each side of the set", () => {
    expect(
      headingFindings(
        new Set(["planted", "home"]),
        new Set(["home", "quiet", "tags"]),
      ),
    ).toEqual([
      "planted prints the page name and is not in SELF_HEADED_SCREENS",
      "quiet is in SELF_HEADED_SCREENS and prints no page name",
    ]);
  });
});

describe("the self-headed-coverage rule", () => {
  it("reports a planted screen that heads itself unlisted", () => {
    const path = join(srcDir, "features", "planted.tsx");
    const text = 'const name = usePageName("planted-screen");\n';
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toContain(
      "planted-screen prints the page name and is not in SELF_HEADED_SCREENS",
    );
  });
});
