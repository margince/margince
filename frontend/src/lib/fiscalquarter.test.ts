// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { fiscalQuarter, opensFiscalQuarter } from "./fiscalquarter";

describe("the fiscal quarter", () => {
  it("is the calendar quarter when the fiscal year starts in January", () => {
    expect(fiscalQuarter("2026-10-26", 1)).toEqual({
      from: "2026-10-01",
      to: "2026-12-31",
    });
  });

  it("follows a fiscal year that starts in April", () => {
    expect(fiscalQuarter("2026-10-26", 4)).toEqual({
      from: "2026-10-01",
      to: "2026-12-31",
    });
    expect(fiscalQuarter("2026-06-15", 4)).toEqual({
      from: "2026-04-01",
      to: "2026-06-30",
    });
  });

  it("crosses the calendar year when the fiscal quarter does", () => {
    expect(fiscalQuarter("2027-01-10", 11)).toEqual({
      from: "2026-11-01",
      to: "2027-01-31",
    });
  });

  it("starts on the first day of a month that opens a quarter", () => {
    expect(fiscalQuarter("2026-02-01", 11).from).toBe("2026-02-01");
    expect(fiscalQuarter("2026-01-01", 11).from).toBe("2025-11-01");
  });

  it("opens on the start month and every third month after it", () => {
    const opening = (start: number) =>
      [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12].filter((month) =>
        opensFiscalQuarter(month, start),
      );
    expect(opening(1)).toEqual([1, 4, 7, 10]);
    expect(opening(11)).toEqual([2, 5, 8, 11]);
  });
});
