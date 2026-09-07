// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  calendarDay,
  calendarMonth,
  dueInstant,
  isRealCalendarDay,
  localDateTimeValue,
  middayInstant,
} from "./calendarday";

// The zone the machine running this suite happens to be in. Every assertion
// below is written against it rather than against a fixed offset, because the
// invariant is "the day the reader picked is the day the reader reads" — a suite
// that only holds in Europe/Berlin would pass on one laptop and prove nothing.
const readerZone = Intl.DateTimeFormat().resolvedOptions().timeZone;

describe("calendarDay", () => {
  it("names the day the instant falls on in the given zone, not in UTC", () => {
    // 02:00 UTC on the 5th is still the evening of the 4th in New York.
    const at = new Date("2026-07-05T02:00:00Z");
    expect(calendarDay(at, "UTC")).toBe("2026-07-05");
    expect(calendarDay(at, "America/New_York")).toBe("2026-07-04");
    expect(calendarDay(at, "Asia/Tokyo")).toBe("2026-07-05");
  });

  it("returns ISO-ordered days, so two of them compare as strings", () => {
    const earlier = calendarDay(new Date("2026-07-04T12:00:00Z"), "UTC");
    const later = calendarDay(new Date("2026-07-05T12:00:00Z"), "UTC");
    expect(earlier).toBe("2026-07-04");
    expect(earlier < later).toBe(true);
  });
});

describe("dueInstant", () => {
  it("files the picked day under that same day in the zone it was minted for", () => {
    for (const zone of [
      "Europe/Berlin",
      "Asia/Bangkok",
      "America/Los_Angeles",
      "UTC",
    ]) {
      for (const day of ["2026-01-15", "2026-07-05", "2026-12-31"]) {
        expect(calendarDay(new Date(dueInstant(day, zone)), zone)).toBe(day);
      }
    }
  });

  // The defect this signature exists to end. A transcript proposal for the 9th
  // was accepted and came back as a task due the 10th: the day's end was
  // resolved on one clock and read on another, and the last second of the day
  // needs only a one-second eastward difference to fall into tomorrow.
  it("names the same day to a reader east of the zone it was minted for", () => {
    const at = new Date(dueInstant("2026-09-09", "Europe/Berlin"));
    expect(at.toISOString()).toBe("2026-09-09T21:59:59.000Z");
    expect(calendarDay(at, "Europe/Berlin")).toBe("2026-09-09");
    expect(calendarDay(at, "Asia/Bangkok")).toBe("2026-09-10");
  });

  it("lands at the END of the picked day, so a task filed for today is not overdue by breakfast", () => {
    const at = new Date(dueInstant("2026-07-05", "Europe/Berlin"));
    expect(at.toISOString()).toBe("2026-07-05T21:59:59.000Z");
  });

  // Whole seconds, not the helper's own last millisecond: this is the shape the
  // task writer has always put on the wire.
  it("carries no fractional second", () => {
    expect(dueInstant("2026-07-05", "Europe/Berlin")).toBe(
      "2026-07-05T21:59:59.000Z",
    );
  });

  // Both directions of the Berlin change, and a zone that has none.
  it("keeps its day across a daylight-saving boundary", () => {
    for (const day of ["2026-03-29", "2026-10-25"]) {
      expect(
        calendarDay(
          new Date(dueInstant(day, "Europe/Berlin")),
          "Europe/Berlin",
        ),
      ).toBe(day);
    }
    expect(dueInstant("2026-03-29", "Europe/Berlin")).toBe(
      "2026-03-29T21:59:59.000Z",
    );
    expect(dueInstant("2026-10-25", "Europe/Berlin")).toBe(
      "2026-10-25T22:59:59.000Z",
    );
    expect(dueInstant("2026-07-05", "Asia/Bangkok")).toBe(
      "2026-07-05T16:59:59.000Z",
    );
  });

  it("is a UTC instant on the wire whatever zone minted it", () => {
    expect(dueInstant("2026-07-05", "Asia/Bangkok")).toMatch(
      /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/,
    );
  });

  it("refuses a day no calendar holds", () => {
    expect(() => dueInstant("2026-02-30", "Europe/Berlin")).toThrow();
    expect(() => dueInstant("10000-09-15", "Europe/Berlin")).toThrow();
  });

  // Date.UTC maps a two-digit year onto 1900-1999, so this would have come
  // back as 1999 and the caller would never learn the day it asked for was not
  // the day it got.
  it("refuses a year the underlying clock would silently move", () => {
    expect(() => dueInstant("0099-09-09", "UTC")).toThrow();
  });

  // `% 1000` on a negative instant rounds toward zero, which is the wrong way:
  // the last second of 31 December 1969 became the first of 1 January 1970 —
  // the very off-by-one-day this signature exists to end.
  it("keeps its day before 1970, where the arithmetic changes sign", () => {
    expect(dueInstant("1969-12-31", "UTC")).toBe("1969-12-31T23:59:59.000Z");
    expect(calendarDay(new Date(dueInstant("1969-12-31", "UTC")), "UTC")).toBe(
      "1969-12-31",
    );
  });
});

describe("localDateTimeValue", () => {
  it("yields the shape a datetime-local input accepts", () => {
    expect(localDateTimeValue("2026-07-05T09:07:00Z")).toMatch(
      /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/,
    );
  });

  it("round-trips through the reading the composer submits", () => {
    // The invariant that matters: seeding a picker from an instant and reading
    // the picker back must land on the SAME minute. `new Date(local)` is what
    // compose.tsx's scheduleFields does, so this is the real pairing rather than
    // a restatement of the formatter. Asserted to the minute because that is the
    // resolution the control has — the seconds a wire instant carries are the
    // one thing a `datetime-local` cannot hold.
    for (const instant of [
      "2026-01-15T23:45:00Z",
      "2026-07-05T02:00:00Z",
      "2026-12-31T12:30:00Z",
    ]) {
      const readBack = new Date(localDateTimeValue(instant));
      expect(readBack.getTime()).toBe(
        Math.floor(new Date(instant).getTime() / 60_000) * 60_000,
      );
    }
  });

  it("names the reader's own day, not UTC's", () => {
    // 02:00 UTC is still the previous evening west of UTC and the same morning
    // east of it, and the picker has to open on the day the reader would say it
    // is. Read against the machine's own zone so the assertion holds wherever
    // the suite runs, which is the same choice `readerZone` above makes.
    const instant = "2026-07-05T02:00:00Z";
    expect(localDateTimeValue(instant).slice(0, 10)).toBe(
      calendarDay(new Date(instant), readerZone),
    );
  });
});

describe("middayInstant", () => {
  it("lands at the zone's own noon of the picked day, either side of a DST switch", () => {
    // Berlin is +01:00 in January and +02:00 in July.
    expect(middayInstant("2026-01-15", "Europe/Berlin")).toBe(
      "2026-01-15T11:00:00.000Z",
    );
    expect(middayInstant("2026-07-05", "Europe/Berlin")).toBe(
      "2026-07-05T10:00:00.000Z",
    );
  });

  it("keeps the picked day in the record zone AND for writers far west of it", () => {
    // The scenario that broke writer-local noon: a writer in Honolulu (UTC-10)
    // backdating an entry a Berlin-rendered timeline then filed under the
    // NEXT day. Minted at Berlin noon, both zones read the picked day.
    const at = new Date(middayInstant("2026-07-10", "Europe/Berlin"));
    expect(calendarDay(at, "Europe/Berlin")).toBe("2026-07-10");
    expect(calendarDay(at, "Pacific/Honolulu")).toBe("2026-07-10");
  });

  it("holds for zones on half-hour offsets and across the date line", () => {
    for (const zone of [
      "Asia/Kolkata",
      "Pacific/Kiritimati",
      "Pacific/Honolulu",
    ]) {
      const at = new Date(middayInstant("2026-03-29", zone));
      expect(calendarDay(at, zone)).toBe("2026-03-29");
    }
  });
});

describe("calendarMonth", () => {
  // The case the day rule was respelled for, at month granularity: the first
  // hours of a month in a zone east of UTC are still the previous month in UTC,
  // and a page that read UTC's month opens on one the reader has left.
  it("is the reader's month, not UTC's, in its first hours", () => {
    const firstHoursInSaigon = new Date("2026-08-31T20:00:00Z");
    expect(calendarMonth(firstHoursInSaigon, "Asia/Ho_Chi_Minh")).toBe(
      "2026-09",
    );
    expect(calendarMonth(firstHoursInSaigon, "UTC")).toBe("2026-08");
  });

  // And its mirror, west of UTC: the first hours of a month in UTC are still
  // the previous month in Los Angeles.
  it("is the reader's month west of UTC too", () => {
    const firstHoursInUTC = new Date("2026-09-01T02:00:00Z");
    expect(calendarMonth(firstHoursInUTC, "America/Los_Angeles")).toBe(
      "2026-08",
    );
    expect(calendarMonth(firstHoursInUTC, "UTC")).toBe("2026-09");
  });

  // It is the day rule cut short, not a second reading of the clock: the two
  // granularities cannot answer about different months.
  it("agrees with calendarDay it is derived from", () => {
    const at = new Date("2026-02-28T23:30:00Z");
    for (const zone of ["UTC", "Asia/Ho_Chi_Minh", "America/Los_Angeles"]) {
      expect(calendarMonth(at, zone)).toBe(calendarDay(at, zone).slice(0, 7));
    }
  });
});

describe("isRealCalendarDay", () => {
  it("accepts a day the calendar actually holds", () => {
    expect(isRealCalendarDay("2026-09-27")).toBe(true);
    expect(isRealCalendarDay("2026-01-31")).toBe(true);
    expect(isRealCalendarDay("2026-12-31")).toBe(true);
  });

  // The whole point: these are spelled correctly and are not dates. A shape
  // check passes them and the date control then blanks them silently.
  it("refuses a well-shaped day that does not exist", () => {
    expect(isRealCalendarDay("2026-02-30")).toBe(false);
    expect(isRealCalendarDay("2026-04-31")).toBe(false);
    expect(isRealCalendarDay("2026-13-01")).toBe(false);
    expect(isRealCalendarDay("2026-00-10")).toBe(false);
    expect(isRealCalendarDay("2026-01-00")).toBe(false);
    expect(isRealCalendarDay("2026-01-32")).toBe(false);
  });

  it("follows the Gregorian leap rule, centuries included", () => {
    expect(isRealCalendarDay("2028-02-29")).toBe(true);
    expect(isRealCalendarDay("2026-02-29")).toBe(false);
    expect(isRealCalendarDay("2000-02-29")).toBe(true);
    expect(isRealCalendarDay("2100-02-29")).toBe(false);
  });

  it("refuses anything that is not a bare YYYY-MM-DD", () => {
    expect(isRealCalendarDay("27/09/2026")).toBe(false);
    expect(isRealCalendarDay("2026-9-27")).toBe(false);
    expect(isRealCalendarDay("2026-09-27T00:00:00Z")).toBe(false);
    expect(isRealCalendarDay("")).toBe(false);
  });
});
