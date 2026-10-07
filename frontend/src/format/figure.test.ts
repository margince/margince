// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { floorFigure } from "./figure";

describe("floorFigure", () => {
  it("reads a cut-short figure as a minimum", () => {
    expect(floorFigure("200", true, 200)).toBe("200+");
    expect(floorFigure("200", false, 200)).toBe("200");
  });

  it("never marks a floor of none, which every number already meets", () => {
    expect(floorFigure("€0", true, 0)).toBe("€0");
  });
});
