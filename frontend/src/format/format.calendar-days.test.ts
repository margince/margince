// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { calendarDaysBetween, daysAgoText } from "./format";

// Day counting reads the CALENDAR, not the clock.
//
// The two private copies this helper replaced both divided a millisecond span
// by 86_400_000, and the product has already shipped that defect once: two
// surfaces on the deal page counted the same silence and printed 96 and 95 on
// one card, agreeing only around midnight. The server now counts one way
// (shared/kernel/elapsed), and these cases are the ones where the two spellings
// visibly disagree.

// Mirrors the real translator's parameter type rather than a wider one. A
// double that accepted a number would accept the raw count `daysAgoText` must
// not hand across.
const t = (key: string, vars?: Record<string, string>) =>
  vars ? `${key}:${JSON.stringify(vars)}` : key;

describe("calendarDaysBetween", () => {
  it("counts a midnight crossing as a day, though two hours passed", () => {
    // The case the millisecond spelling gets wrong: it answers 0.
    const late = new Date("2026-05-20T23:00:00Z");
    const early = new Date("2026-05-21T01:00:00Z");
    expect(calendarDaysBetween(late, early, "UTC")).toBe(1);
  });

  it("counts a full day inside one date as zero, though 23 hours passed", () => {
    // The mirror, and the reason the first case is not just a rounding choice:
    // the millisecond spelling answers 0 here too, so it cannot tell the two
    // apart. A reader looking at two dates can.
    const morning = new Date("2026-05-20T00:30:00Z");
    const night = new Date("2026-05-20T23:30:00Z");
    expect(calendarDaysBetween(morning, night, "UTC")).toBe(0);
  });

  it("counts whole days across a longer span", () => {
    expect(
      calendarDaysBetween(
        new Date("2026-05-20T12:00:00Z"),
        new Date("2026-08-24T12:00:00Z"),
        "UTC",
      ),
    ).toBe(96);
  });

  it("is negative into the future, so a plan is not read as an event", () => {
    expect(
      calendarDaysBetween(
        new Date("2026-05-23T12:00:00Z"),
        new Date("2026-05-20T12:00:00Z"),
        "UTC",
      ),
    ).toBe(-3);
  });
});

describe("calendarDaysBetween in a reader zone", () => {
  // Fixed instants, no real clock. The same pair is a different number of
  // calendar days in each zone.
  it("counts 15:00 yesterday to 01:00 as one day in UTC+7 and zero in UTC", () => {
    const met = new Date("2026-10-06T08:00:00Z");
    const now = new Date("2026-10-06T18:00:00Z");
    expect(calendarDaysBetween(met, now, "Asia/Bangkok")).toBe(1);
    expect(calendarDaysBetween(met, now, "UTC")).toBe(0);
  });

  it("counts an evening in UTC-5 as today although UTC has turned", () => {
    const met = new Date("2026-12-05T23:00:00Z");
    const now = new Date("2026-12-06T01:00:00Z");
    expect(calendarDaysBetween(met, now, "America/New_York")).toBe(0);
    expect(calendarDaysBetween(met, now, "UTC")).toBe(1);
  });

  it("stays whole across a DST change", () => {
    expect(
      calendarDaysBetween(
        new Date("2026-03-07T12:00:00Z"),
        new Date("2026-03-09T12:00:00Z"),
        "America/New_York",
      ),
    ).toBe(2);
    expect(
      calendarDaysBetween(
        new Date("2026-03-28T23:30:00Z"),
        new Date("2026-03-29T22:30:00Z"),
        "Europe/Berlin",
      ),
    ).toBe(1);
  });

  it("follows a half-hour offset", () => {
    expect(
      calendarDaysBetween(
        new Date("2026-10-06T17:30:00Z"),
        new Date("2026-10-06T18:45:00Z"),
        "Asia/Kolkata",
      ),
    ).toBe(1);
  });
});

describe("daysAgoText", () => {
  it("says today for none and for a future count, yesterday for one", () => {
    expect(daysAgoText(0, t, "en")).toBe("contact.strip.today");
    expect(daysAgoText(-3, t, "en")).toBe("contact.strip.today");
    expect(daysAgoText(1, t, "en")).toBe("contact.strip.yesterday");
  });

  it("hands the count across already grouped for this reader", () => {
    // A four-figure count is where the two notations part: a German reader
    // reads "1.200" and an English one "1,200", and a raw number would have
    // reached the sentence as "1200" for both.
    expect(daysAgoText(96, t, "en")).toBe('contact.strip.days:{"count":"96"}');
    expect(daysAgoText(1200, t, "en")).toBe(
      'contact.strip.days:{"count":"1,200"}',
    );
    expect(daysAgoText(1200, t, "de")).toBe(
      'contact.strip.days:{"count":"1.200"}',
    );
  });
});
