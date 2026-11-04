// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { instant } from "./standingtimeline.layout";
import { defaultZoom, zoomStart } from "./standingtimeline.zoom";

const today = instant("2026-11-04T09:00:00Z");
const iso = (at: number) => new Date(at).toISOString();

describe("where a zoom starts the axis", () => {
  it("reads a month, a quarter and a year back from today", () => {
    expect(iso(zoomStart("month", today, []))).toBe("2026-10-04T09:00:00.000Z");
    expect(iso(zoomStart("quarter", today, []))).toBe(
      "2026-08-04T09:00:00.000Z",
    );
    expect(iso(zoomStart("year", today, []))).toBe("2025-11-04T09:00:00.000Z");
  });

  it("starts All time at the earliest moment the record holds", () => {
    const first = instant("2023-02-14");
    expect(zoomStart("all", today, [instant("2026-01-01"), first])).toBe(first);
  });

  it("starts All time at today when nothing is dated before it", () => {
    expect(zoomStart("all", today, [instant("2027-01-01")])).toBe(today);
  });
});

describe("the zoom a line opens at", () => {
  it("is a quarter while every event falls in it, however recent", () => {
    expect(defaultZoom(today, [instant("2026-11-02")])).toBe("quarter");
    expect(defaultZoom(today, [])).toBe("quarter");
  });

  it("widens to a year, then to All time, to hold the earliest event", () => {
    expect(defaultZoom(today, [instant("2026-03-01")])).toBe("year");
    expect(defaultZoom(today, [instant("2024-03-01")])).toBe("all");
  });

  it("is not widened by a date ahead", () => {
    expect(defaultZoom(today, [instant("2029-08-29")])).toBe("quarter");
  });
});
