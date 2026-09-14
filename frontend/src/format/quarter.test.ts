import { describe, expect, it } from "vitest";
import { quarterLabel } from "./quarter";

// The label a pipeline reading wears. Its whole job is to name the window the
// server chose without reading the viewer's clock, so the cases that matter are
// the quarter boundaries and the strings that are not a date at all.

describe("quarterLabel", () => {
  it("names the quarter each month falls in", () => {
    expect(quarterLabel("2026-01-01")).toBe("Q1");
    expect(quarterLabel("2026-03-31")).toBe("Q1");
    expect(quarterLabel("2026-04-01")).toBe("Q2");
    expect(quarterLabel("2026-06-30")).toBe("Q2");
    expect(quarterLabel("2026-07-01")).toBe("Q3");
    expect(quarterLabel("2026-09-30")).toBe("Q3");
    expect(quarterLabel("2026-10-01")).toBe("Q4");
    expect(quarterLabel("2026-12-31")).toBe("Q4");
  });

  // The defect this helper exists to avoid: a date-only value read through the
  // viewer's clock moves a quarter's first day into the one before it. Nothing
  // here touches `Date`, so the answer is the same wherever it is asked — and
  // the assertion is the first day of every quarter, which is the only day the
  // shift could reach.
  it("reads the month off the string, so no viewer's zone can move it", () => {
    for (const [day, label] of [
      ["2026-01-01", "Q1"],
      ["2026-04-01", "Q2"],
      ["2026-07-01", "Q3"],
      ["2026-10-01", "Q4"],
    ] as const) {
      expect(quarterLabel(day)).toBe(label);
    }
  });

  // A period the page cannot read is not a period, and a label invented for it
  // would sit under a figure nobody could reconcile it against.
  it("names no quarter for a value that is not a month", () => {
    expect(quarterLabel("")).toBeNull();
    expect(quarterLabel("2026-13-01")).toBeNull();
    expect(quarterLabel("2026-00-01")).toBeNull();
    expect(quarterLabel("not-a-date")).toBeNull();
  });
});
