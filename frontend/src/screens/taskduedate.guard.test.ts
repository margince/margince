// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the due-date picker is allowed to send.
//
// Asked of the guard directly rather than through the control, because jsdom's
// date input is STRICTER than the HTML spec: it clears itself for a five-digit
// year that a real browser reports happily, so a test driving the input can
// never deliver the value this guard exists to refuse. Driving the box would
// pass with no guard at all — measured, not assumed.

import { describe, expect, it } from "vitest";
import { isISODate } from "../design-system/dateinput";
import { calendarDay, dueInstant } from "../format/calendarday";
import { snoozedDueAt } from "./taskactions";

describe("the days a task may be moved to", () => {
  it("refuses what dueInstant cannot convert", () => {
    // HTML permits a year of four OR MORE digits, so this is a value a native
    // date box reports — and dueInstant throws a RangeError on it. Without the
    // guard a year typo reaches an uncaught exception with nothing on screen
    // to say the date was refused.
    expect(isISODate("10000-09-15")).toBe(false);
    expect(() => dueInstant("10000-09-15", "Europe/Berlin")).toThrow();
  });

  it("refuses the cleared box", () => {
    // Clearing is the browser's own empty state, not a request to undate the
    // task. It is also what the element reports for anything it could not
    // parse, so one question answers both.
    expect(isISODate("")).toBe(false);
  });

  it("admits an ordinary day, which dueInstant then converts", () => {
    // The guard has to ADMIT as well as refuse: one that said no to everything
    // would pass both tests above and break the feature.
    expect(isISODate("2026-09-15")).toBe(true);
    // Read back in the zone it was minted for, the instant is that day's last
    // whole second — the same day the picker offered, for every reader.
    expect(dueInstant("2026-09-15", "Europe/Berlin")).toBe(
      "2026-09-15T21:59:59.000Z",
    );
  });
});

// Snoozing moves a task to the NEXT CALENDAR DAY, which is not twenty-four
// hours. A local day is not always that long: adding a day to Berlin's 28 March
// at 23:59:59 landed at 00:59:59 on the 30th, so a rep pressing "tomorrow"
// skipped the 29th entirely and the task they meant to see that day was never
// on it.
describe("snoozing a task by a day", () => {
  it("lands on the next calendar day across spring forward", () => {
    // 2026-03-28 23:59:59 Berlin. The next day is the 29th, the day the clocks
    // move — an hour shorter, and still one day away.
    const from = "2026-03-28T22:59:59.000Z";
    expect(snoozedDueAt(from, "Europe/Berlin")).toBe(
      "2026-03-29T21:59:59.000Z",
    );
    expect(
      calendarDay(
        new Date(snoozedDueAt(from, "Europe/Berlin") as string),
        "Europe/Berlin",
      ),
    ).toBe("2026-03-29");
  });

  it("lands on the next calendar day across autumn back", () => {
    const from = "2026-10-24T21:59:59.000Z";
    expect(
      calendarDay(
        new Date(snoozedDueAt(from, "Europe/Berlin") as string),
        "Europe/Berlin",
      ),
    ).toBe("2026-10-25");
  });

  it("steps a month end onto the first", () => {
    const from = "2026-01-31T22:59:59.000Z";
    expect(
      calendarDay(
        new Date(snoozedDueAt(from, "Europe/Berlin") as string),
        "Europe/Berlin",
      ),
    ).toBe("2026-02-01");
  });

  it("has nothing to move on an undated task", () => {
    expect(snoozedDueAt(null, "Europe/Berlin")).toBeNull();
  });
});
