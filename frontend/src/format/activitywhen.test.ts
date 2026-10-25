// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  activityAgo,
  activityDay,
  activityHasBegun,
  activityIsAhead,
  activityTimeOfDay,
  activityTimeSpan,
  dayOccurredAt,
  formatActivityWhen,
} from "./activitywhen";
import { middayInstant } from "./calendarday";
import { formatDayMonth } from "./format.dates";

// The zones furthest east and west of UTC. Their midday falls on another UTC
// day, so a reader that drifted to UTC would show the wrong date.
const EAST = "Pacific/Kiritimati";
const WEST = "Pacific/Pago_Pago";
// A day entry recorded in the far east, read by a colleague in the far west.
const FAR_APART = { record: EAST, viewer: WEST };

describe("an activity's time as a reader sees it", () => {
  it("draws a day entry as its date on the record's clock, for every viewer", () => {
    const stored = middayInstant("2026-09-15", EAST);
    expect(stored).toBe("2026-09-14T22:00:00.000Z");
    expect(formatActivityWhen(stored, "day", "en", FAR_APART)).toBe(
      "15/09/2026",
    );
    expect(formatActivityWhen(stored, "day", "de", FAR_APART)).toBe(
      "15.09.2026",
    );
    expect(activityDay(stored, "day", "en", FAR_APART, formatDayMonth)).toBe(
      formatDayMonth("2026-09-15", "en", EAST),
    );
  });

  it("draws an instant with its clock reading on the viewer's clock", () => {
    const zones = { record: EAST, viewer: "UTC" };
    expect(
      formatActivityWhen("2026-09-15T08:30:00Z", "instant", "en", zones),
    ).toBe("15/09/2026, 08:30");
    expect(activityDay("2026-09-14T22:00:00Z", "instant", "en", zones)).toBe(
      "14/09/2026",
    );
  });

  it("names no time of day for a day entry", () => {
    const at = middayInstant("2026-09-15", EAST);
    expect(activityTimeOfDay(at, "day", "en", FAR_APART)).toBeNull();
    expect(
      activityTimeOfDay(at, "instant", "en", { record: EAST, viewer: EAST }),
    ).toBe("12:00");
  });

  it("names no span for a meeting whose day alone is known", () => {
    const zones = { record: "UTC", viewer: "UTC" };
    const start = "2026-09-15T08:30:00Z";
    const end = "2026-09-15T09:00:00Z";
    expect(activityTimeSpan(start, end, "day", "en", zones)).toBeNull();
    expect(activityTimeSpan(start, end, "instant", "en", zones)).toBe(
      "08:30–09:00",
    );
    expect(activityTimeSpan(start, undefined, "instant", "en", zones)).toBe(
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
        EAST,
      ),
    ).toBe('elapsed.hours:{"hours":"3"}');
  });

  it("names a day entry's date, since its noon is nobody's hour", () => {
    const stored = middayInstant("2026-09-14", EAST);
    expect(
      activityAgo({ at: stored, precision: "day" }, now, t, "en", EAST),
    ).toBe("14/09/2026");
  });
});

describe("whether an activity is still to come", () => {
  const stored = middayInstant("2026-09-15", EAST);

  it("keeps a day entry current for its whole date in the record zone", () => {
    // 23:30 on the 15th in Kiritimati: the stored noon is long past.
    const lateThatDay = Date.parse("2026-09-15T09:30:00Z");
    expect(activityIsAhead(stored, "day", lateThatDay, EAST)).toBe(true);
    expect(activityHasBegun(stored, "day", lateThatDay, EAST)).toBe(true);
    expect(activityIsAhead(stored, "instant", lateThatDay, EAST)).toBe(false);
    // 00:30 on the 15th: the stored noon is still to come.
    const earlyThatDay = Date.parse("2026-09-14T10:30:00Z");
    expect(activityHasBegun(stored, "day", earlyThatDay, EAST)).toBe(true);
    expect(activityHasBegun(stored, "instant", earlyThatDay, EAST)).toBe(false);
  });

  it("lets a day entry pass once its date ends in the record zone", () => {
    const nextMorning = Date.parse("2026-09-15T10:30:00Z");
    expect(activityIsAhead(stored, "day", nextMorning, EAST)).toBe(false);
  });

  it("has not begun a day entry before its date starts in the record zone", () => {
    const dayBefore = Date.parse("2026-09-14T09:30:00Z");
    expect(activityHasBegun(stored, "day", dayBefore, EAST)).toBe(false);
  });
});

describe("the wire value of a date-only time", () => {
  it("is noon UTC on the picked date, whatever zone the record keeps", () => {
    expect(dayOccurredAt("2026-09-15")).toBe("2026-09-15T12:00:00Z");
  });
});
