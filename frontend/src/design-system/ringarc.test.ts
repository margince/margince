// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { fillDash, RING_CIRCUMFERENCE, segmentDashes } from "./ringarc";

const C = Math.round(RING_CIRCUMFERENCE * 100) / 100;

describe("the fill arc", () => {
  it("draws the share of the ring it is given", () => {
    expect(fillDash(0.5)).toBe(
      `${Math.round((RING_CIRCUMFERENCE / 2) * 100) / 100} ${C}`,
    );
  });

  it("holds a share outside 0 to 1 to the ring", () => {
    expect(fillDash(-0.2)).toBe(`0 ${C}`);
    expect(fillDash(1.4)).toBe(`${C} ${C}`);
  });
});

describe("the segment arcs", () => {
  it("cuts the ring into equal arcs with a gap after each", () => {
    const segments = segmentDashes(3, 4);
    const gap = (4 / 360) * RING_CIRCUMFERENCE;
    const drawn = Number(segments[0]?.dash.split(" ")[0]);
    expect(drawn).toBeCloseTo(RING_CIRCUMFERENCE / 3 - gap, 1);
    expect(segments.map((segment) => segment.offset)).toEqual([
      -0,
      -Math.round((RING_CIRCUMFERENCE / 3) * 100) / 100,
      -Math.round(((2 * RING_CIRCUMFERENCE) / 3) * 100) / 100,
    ]);
  });

  it("draws one arc as the whole ring, with no gap", () => {
    expect(segmentDashes(1, 4)).toEqual([{ dash: `${C} 0`, offset: -0 }]);
  });

  it("draws nothing for no arcs", () => {
    expect(segmentDashes(0, 4)).toEqual([]);
  });
});
