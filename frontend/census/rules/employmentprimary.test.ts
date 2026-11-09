// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { LITERAL, rule } from "./employmentprimary.census";

describe("the employmentprimary rule", () => {
  it("matches a literal and not a value the reader chose", () => {
    expect(LITERAL.test("  is_current_primary: false,")).toBe(true);
    expect(LITERAL.test("  is_current_primary: isCurrent ?? undefined,")).toBe(
      false,
    );
  });

  it("fails a planted screen and leaves its story alone", () => {
    const text = "save({ is_current_primary: true });\n";
    const findings = plantedFindings(rule, [
      { path: join(srcDir, "features", "x", "rail.tsx"), area: "src", text },
      {
        path: join(srcDir, "features", "x", "rail.stories.tsx"),
        area: "src",
        text,
      },
    ]);
    expect(findings).toEqual([
      "src/features/x/rail.tsx: sends is_current_primary as a literal",
    ]);
  });
});
