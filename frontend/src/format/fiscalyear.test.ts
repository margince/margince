import { describe, expect, it } from "vitest";
import { fiscalYearLabel } from "./fiscalyear";
import { monthName } from "./format";

// The label an admin is shown before they save a fiscal start, and the two
// things about it that can be silently wrong.
//
// `backend/frontendfiscalyear_test.go` holds this against the SQL the server
// actually cuts reports with. This file holds the values themselves, which that
// gate cannot see: it reads shape out of both sources and cannot execute the
// SQL, so an off-by-one here would pass it.

describe("the label a fiscal year carries", () => {
  it("spells a January year as the plain calendar year", () => {
    // Not "FY2026/27", which would be false — a January year does not span
    // 2027. This is also every installation that predates the setting, so it
    // is the branch that must not change.
    expect(fiscalYearLabel(1, 2026)).toBe("2026");
  });

  it("names both calendar years a non-January year spans", () => {
    expect(fiscalYearLabel(4, 2026)).toBe("FY2026/27");
    expect(fiscalYearLabel(10, 2026)).toBe("FY2026/27");
    // A February start spans the same two years; the start month changes which
    // months fall inside, never how many calendar years are crossed.
    expect(fiscalYearLabel(2, 2026)).toBe("FY2026/27");
  });

  it("rolls the ending year over a century boundary", () => {
    // The case a string slice of the first year gets wrong: 2099 + 1 is 2100,
    // whose last two digits are "00" rather than "100".
    expect(fiscalYearLabel(4, 2099)).toBe("FY2099/00");
  });

  it("keeps the labels sorting chronologically as text", () => {
    // The property the whole period vocabulary rests on: a bucket value is
    // BOTH what a reader sees and what sorts, so a report needs no separate
    // sort key. A longer label could have broken it silently.
    const labels = [
      fiscalYearLabel(4, 2026),
      fiscalYearLabel(4, 2024),
      fiscalYearLabel(4, 2099),
      fiscalYearLabel(4, 2025),
    ];
    expect([...labels].sort()).toEqual([
      "FY2024/25",
      "FY2025/26",
      "FY2026/27",
      "FY2099/00",
    ]);
  });
});

describe("the month's own name", () => {
  it("is the reader's language, not a table of ours", () => {
    expect(monthName(1, "en")).toBe("January");
    expect(monthName(1, "de")).toBe("Januar");
  });

  it("names the month it was given from any reader's timezone", () => {
    // The date behind the lookup is minted in UTC, so a formatter running on
    // the reader's own clock reads it back in a different zone than it was
    // written in — and midnight on the 1st is the worst instant for that: every
    // zone behind UTC lands on the last day of the PREVIOUS month. This
    // returned "December" for month 1 in America/New_York, so the fiscal-year
    // picker offered a label one month off the value it saved.
    //
    // Driven through the formatter's own resolved zone rather than by setting
    // TZ, because vitest cannot change the process zone mid-run. What it
    // asserts instead is the property that makes the zone irrelevant: the same
    // answer for a month whose UTC-midnight instant falls in the prior month
    // for every negative offset.
    const west = new Intl.DateTimeFormat("en-GB", {
      timeZone: "America/Los_Angeles",
      month: "long",
    }).format(new Date(Date.UTC(2026, 0, 1)));
    // Proof the trap is real in the first place: read on a western clock, that
    // instant IS December.
    expect(west).toBe("December");
    // And proof monthName does not fall into it.
    expect(monthName(1, "en")).toBe("January");
  });

  it("reads every month rather than only the ones a test happened to try", () => {
    // A guard against an off-by-one in the 0-indexed Date constructor: month 12
    // must be December, not January of the next year.
    expect(monthName(12, "en")).toBe("December");
    const names = Array.from({ length: 12 }, (_, index) =>
      monthName(index + 1, "en"),
    );
    expect(new Set(names).size).toBe(12);
  });
});
