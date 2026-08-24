// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { calendarDaysBetween, relativeDays } from "./format";

// Day counting reads the CALENDAR, not the clock.
//
// The two private copies this helper replaced both divided a millisecond span
// by 86_400_000, and the product has already shipped that defect once: two
// surfaces on the deal page counted the same silence and printed 96 and 95 on
// one card, agreeing only around midnight. The server now counts one way
// (shared/kernel/elapsed), and these cases are the ones where the two spellings
// visibly disagree.

const t = (key: string, vars?: Record<string, string | number>) =>
  vars ? `${key}:${JSON.stringify(vars)}` : key;

describe("calendarDaysBetween", () => {
  it("counts a midnight crossing as a day, though two hours passed", () => {
    // The case the millisecond spelling gets wrong: it answers 0.
    const late = new Date("2026-05-20T23:00:00Z");
    const early = new Date("2026-05-21T01:00:00Z");
    expect(calendarDaysBetween(late, early)).toBe(1);
  });

  it("counts a full day inside one date as zero, though 23 hours passed", () => {
    // The mirror, and the reason the first case is not just a rounding choice:
    // the millisecond spelling answers 0 here too, so it cannot tell the two
    // apart. A reader looking at two dates can.
    const morning = new Date("2026-05-20T00:30:00Z");
    const night = new Date("2026-05-20T23:30:00Z");
    expect(calendarDaysBetween(morning, night)).toBe(0);
  });

  it("counts whole days across a longer span", () => {
    expect(
      calendarDaysBetween(
        new Date("2026-05-20T12:00:00Z"),
        new Date("2026-08-24T12:00:00Z"),
      ),
    ).toBe(96);
  });

  it("is negative into the future, so a plan is not read as an event", () => {
    expect(
      calendarDaysBetween(
        new Date("2026-05-23T12:00:00Z"),
        new Date("2026-05-20T12:00:00Z"),
      ),
    ).toBe(-3);
  });
});

describe("relativeDays", () => {
  const now = new Date("2026-08-24T12:00:00Z");

  it("says never for an absent timestamp", () => {
    expect(relativeDays(null, t, now)).toBe("person.strip.never");
    expect(relativeDays(undefined, t, now)).toBe("person.strip.never");
  });

  it("says today for the same calendar day", () => {
    expect(relativeDays("2026-08-24T01:00:00Z", t, now)).toBe(
      "person.strip.today",
    );
  });

  it("says today for a future timestamp rather than a negative count", () => {
    expect(relativeDays("2026-09-01T00:00:00Z", t, now)).toBe(
      "person.strip.today",
    );
  });

  it("says yesterday for the previous calendar day", () => {
    expect(relativeDays("2026-08-23T22:00:00Z", t, now)).toBe(
      "person.strip.yesterday",
    );
  });

  it("counts the days for anything older", () => {
    expect(relativeDays("2026-05-20T09:00:00Z", t, now)).toBe(
      'person.strip.days:{"count":96}',
    );
  });

  it("reads a late-evening timestamp as yesterday, not today", () => {
    // The boundary the millisecond spelling gets wrong: 13 hours earlier but a
    // different date, which a reader calls yesterday and a duration calls today.
    expect(relativeDays("2026-08-23T23:00:00Z", t, now)).toBe(
      "person.strip.yesterday",
    );
  });
});
