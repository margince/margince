// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  activityAgo,
  activityDate,
  activityDay,
  activityDaysAway,
  activityHasBegun,
  activityHasHappened,
  activityIsAhead,
  activityTimeOfDay,
  activityTimeSpan,
  dayOccurredAt,
  formatActivityWhen,
  movedActivityStart,
} from "./activitywhen";
import { formatDayMonth } from "./format.dates";

// Zones far east and west of UTC, where a day's noon UTC falls on another
// local date. A reader that drew a day entry in one would show the wrong date.
const EAST = "Pacific/Kiritimati";
const WEST = "Pacific/Pago_Pago";
const FAR = [EAST, WEST, "Pacific/Auckland", "America/Los_Angeles"];

describe("an activity's time as a reader sees it", () => {
  const stored = dayOccurredAt("2026-09-15");

  it("draws a day entry as its UTC date, in every viewer zone", () => {
    for (const zone of FAR) {
      expect(activityDay(stored, "day", "en", zone)).toBe("15/09/2026");
      expect(formatActivityWhen(stored, "day", "en", zone)).toBe("15/09/2026");
      expect(formatActivityWhen(stored, "day", "de", zone)).toBe("15.09.2026");
      expect(activityDay(stored, "day", "en", zone, formatDayMonth)).toBe(
        formatDayMonth(stored, "en", "UTC"),
      );
    }
  });

  it("draws an instant with its clock reading on the viewer's clock", () => {
    expect(
      formatActivityWhen("2026-09-15T08:30:00Z", "instant", "en", "UTC"),
    ).toBe("15/09/2026, 08:30");
    expect(activityDay("2026-09-14T22:00:00Z", "instant", "en", EAST)).toBe(
      "15/09/2026",
    );
  });

  it("names no time of day for a day entry", () => {
    expect(activityTimeOfDay(stored, "day", "en", WEST)).toBeNull();
    expect(activityTimeOfDay(stored, "instant", "en", "UTC")).toBe("12:00");
  });

  it("names no span for a meeting whose day alone is known", () => {
    const start = "2026-09-15T08:30:00Z";
    const end = "2026-09-15T09:00:00Z";
    expect(activityTimeSpan(start, end, "day", "en", "UTC")).toBeNull();
    expect(activityTimeSpan(start, end, "instant", "en", "UTC")).toBe(
      "08:30–09:00",
    );
    expect(activityTimeSpan(start, undefined, "instant", "en", "UTC")).toBe(
      "08:30",
    );
  });
});

describe("how long ago an activity happened", () => {
  // An English catalog line, with its values filled in.
  const t = (key: string, params?: Record<string, string>) =>
    `${key}:${JSON.stringify(params ?? {})}`;
  const now = Date.parse("2026-09-15T12:00:00Z");

  it("counts an instant's age in hours", () => {
    expect(
      activityAgo(
        { at: "2026-09-15T09:00:00Z", precision: "instant" },
        now,
        t,
        "en",
      ),
    ).toBe('elapsed.hours:{"hours":"3"}');
  });

  it("names a day entry's date, since its noon is nobody's hour", () => {
    const moment = {
      at: dayOccurredAt("2026-09-14"),
      precision: "day",
    } as const;
    expect(activityAgo(moment, now, t, "en")).toBe("14/09/2026");
    expect(activityAgo(moment, now, t, "en", formatDayMonth)).toBe(
      formatDayMonth(moment.at, "en", "UTC"),
    );
  });
});

describe("whether an activity is still to come", () => {
  const stored = dayOccurredAt("2026-09-15");

  it("keeps a day entry current for its whole date in the record zone", () => {
    // 23:30 on the 15th in Kiritimati, 09:30 UTC: the stored noon is to come.
    const lateThatDay = Date.parse("2026-09-15T09:30:00Z");
    expect(activityIsAhead(stored, "day", lateThatDay, EAST)).toBe(true);
    expect(activityHasBegun(stored, "day", lateThatDay, EAST)).toBe(true);
    // 23:30 on the 15th in Pago Pago, the 16th in UTC: noon is long past.
    const lateInTheWest = Date.parse("2026-09-16T10:30:00Z");
    expect(activityIsAhead(stored, "day", lateInTheWest, WEST)).toBe(true);
    expect(activityIsAhead(stored, "instant", lateInTheWest, WEST)).toBe(false);
  });

  it("lets a day entry pass once its date ends in the record zone", () => {
    // 00:30 on the 16th in Kiritimati, still the 15th in UTC.
    const nextMorning = Date.parse("2026-09-15T10:30:00Z");
    expect(activityIsAhead(stored, "day", nextMorning, EAST)).toBe(false);
    expect(activityIsAhead(stored, "day", nextMorning, WEST)).toBe(true);
  });

  it("has not begun a day entry before its date starts in the record zone", () => {
    // 23:30 on the 14th in Pago Pago, already the 15th in UTC.
    const eveBefore = Date.parse("2026-09-15T10:30:00Z");
    expect(activityHasBegun(stored, "day", eveBefore, WEST)).toBe(false);
    expect(activityHasBegun(stored, "day", eveBefore, EAST)).toBe(true);
    expect(activityHasBegun(stored, "instant", eveBefore, WEST)).toBe(false);
  });
});

describe("how many dates lie between today and an activity", () => {
  const stored = dayOccurredAt("2026-09-15");

  it("counts a day entry's date from today in the record zone", () => {
    // 00:30 on the 16th in Kiritimati, 23:30 on the 14th in Pago Pago.
    const now = Date.parse("2026-09-15T10:30:00Z");
    expect(activityDate(stored, "day", EAST)).toBe("2026-09-15");
    expect(activityDaysAway(stored, "day", now, "UTC", EAST)).toBe(-1);
    expect(activityDaysAway(stored, "day", now, "UTC", WEST)).toBe(1);
  });

  it("counts an instant on the calendar of the zone it is drawn in", () => {
    const now = Date.parse("2026-09-15T10:30:00Z");
    expect(activityDate(stored, "instant", EAST)).toBe("2026-09-16");
    expect(activityDaysAway(stored, "instant", now, EAST, WEST)).toBe(0);
  });
});

describe("whether an activity has taken place", () => {
  const now = Date.parse("2026-09-15T15:00:00Z");
  const today = dayOccurredAt("2026-09-15");

  it("counts a day note dated today, but not a day meeting booked for today", () => {
    const note = {
      kind: "note",
      occurred_at: today,
      occurred_precision: "day",
    } as const;
    const booked = { ...note, kind: "meeting", meeting_status: "booked" };
    expect(activityHasHappened(note, now, "UTC")).toBe(true);
    expect(activityHasHappened(booked, now, "UTC")).toBe(false);
    expect(
      activityHasHappened({ ...booked, meeting_status: "held" }, now, "UTC"),
    ).toBe(true);
  });
});

describe("the start a moved activity is sent with", () => {
  const now = new Date("2026-09-20T12:00:00Z");

  it("names a day entry's new date alone", () => {
    const start = {
      at: dayOccurredAt("2026-09-15"),
      precision: "day",
    } as const;
    expect(movedActivityStart(start, "2026-09-12", EAST, now)).toEqual({
      at: "2026-09-12T12:00:00Z",
      precision: "day",
    });
  });

  it("keeps an instant's time of day, and never moves it past now", () => {
    const start = { at: "2026-09-15T08:30:00Z", precision: "instant" } as const;
    expect(movedActivityStart(start, "2026-09-12", "UTC", now)).toEqual({
      at: "2026-09-12T08:30:00.000Z",
      precision: "instant",
    });
    expect(movedActivityStart(start, "2026-09-25", "UTC", now).at).toBe(
      now.toISOString(),
    );
  });
});

describe("the wire value of a date-only time", () => {
  it("is noon UTC on the picked date, whatever zone the record keeps", () => {
    expect(dayOccurredAt("2026-09-15")).toBe("2026-09-15T12:00:00Z");
  });
});
