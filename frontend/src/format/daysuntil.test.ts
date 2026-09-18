// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { calendarDaysUntil } from "./daysuntil";

// A record's date is drawn in the record's zone, so the count beside it is read
// there too. UTC is the one zone that cannot be assumed: it is the right answer
// about an instant and the wrong one about a day somebody named.
//
// Every case names its own `now`. Nothing here reads the machine's clock or the
// machine's zone, so the verdicts are the same in CI, on a laptop in Hanoi, and
// at +200 days under `make fe-clock-drift`.

const target = "2026-09-20";

describe("calendarDaysUntil", () => {
  it("counts from the day the reader is in, ahead of UTC", () => {
    // 09:00 on the 18th in Auckland, still the 17th in UTC. Counted in UTC the
    // target reads three mornings away on a morning when it is two.
    const now = new Date("2026-09-17T21:00:00Z");
    expect(calendarDaysUntil(target, "Pacific/Auckland", now)).toBe(2);
    expect(calendarDaysUntil(target, "UTC", now)).toBe(3);
  });

  it("counts from the day the reader is in, behind UTC", () => {
    // The mirror, and the reason the first case is not an artefact of one
    // hemisphere: 19:00 on the 17th in Los Angeles, already the 18th in UTC.
    const now = new Date("2026-09-18T02:00:00Z");
    expect(calendarDaysUntil(target, "America/Los_Angeles", now)).toBe(3);
    expect(calendarDaysUntil(target, "UTC", now)).toBe(2);
  });

  it("is zero on the day itself, whatever hour the reader opens it", () => {
    // The boundary an elapsed-milliseconds count gets wrong in both directions:
    // it reads the first hour of the day as one day off and the last as none.
    for (const hour of ["00:30", "12:00", "23:30"]) {
      expect(
        calendarDaysUntil(target, "UTC", new Date(`2026-09-20T${hour}:00Z`)),
        hour,
      ).toBe(0);
    }
  });

  it("is negative once the day has passed, so overdue is a count", () => {
    expect(
      calendarDaysUntil(target, "UTC", new Date("2026-09-27T08:00:00Z")),
    ).toBe(-7);
  });

  it("holds its count across a DST switch", () => {
    // Central Europe puts its clocks back on 25 October 2026. Counted in
    // elapsed time the span holds an extra hour and the last day rounds away;
    // counted by the calendar the reader crosses the same 32 dates either way.
    expect(
      calendarDaysUntil(
        "2026-11-01",
        "Europe/Berlin",
        new Date("2026-09-30T08:00:00Z"),
      ),
    ).toBe(32);
  });

  it("reads a full instant as the day it lands on in that zone", () => {
    // Both kinds of value reach this: a record's `yyyy-mm-dd`, whose day
    // survives every zone, and an instant, which lands on one.
    const now = new Date("2026-09-17T09:00:00Z");
    expect(calendarDaysUntil("2026-09-20T23:00:00Z", "UTC", now)).toBe(3);
    expect(
      calendarDaysUntil("2026-09-20T23:00:00Z", "Pacific/Auckland", now),
    ).toBe(4);
  });
});
