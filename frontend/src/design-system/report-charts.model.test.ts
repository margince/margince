// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { niceTop } from "./report-charts.model";

describe("a chart's axis top", () => {
  it("rises to the nearest round step, not to the next power of five", () => {
    expect(niceTop(24_534_566)).toBe(30_000_000);
    expect(niceTop(186_000)).toBe(200_000);
    expect(niceTop(800_000)).toBe(1_000_000);
  });

  it("keeps air above a value that sits on a step", () => {
    expect(niceTop(1_000)).toBe(1_200);
  });

  it("draws an empty period on a unit axis", () => {
    expect(niceTop(0)).toBe(1);
  });
});
