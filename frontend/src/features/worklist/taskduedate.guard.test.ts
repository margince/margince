// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the due-date picker can hand the writer. The picker sends only a day its
// own parser read, so the parser is where a year typo is refused.

import { describe, expect, it } from "vitest";
import { snoozedDueAt } from "../../components/taskactions";
import { calendarDay, dueInstant } from "../../format/calendarday";
import { parseTypedDate } from "../../format/datepattern";
import { FIXED_DATE_PATTERNS } from "../../format/preferences";

describe("the days a task may be moved to", () => {
  it("never reads a five-digit year, which dueInstant cannot convert", () => {
    expect(parseTypedDate("15/09/10000", FIXED_DATE_PATTERNS.dmy)).toBeNull();
    expect(parseTypedDate("10000-09-15", FIXED_DATE_PATTERNS.dmy)).toBeNull();
    expect(() => dueInstant("10000-09-15", "Europe/Berlin")).toThrow();
  });

  it("admits an ordinary day, which dueInstant then converts", () => {
    expect(parseTypedDate("15/09/2026", FIXED_DATE_PATTERNS.dmy)).toBe(
      "2026-09-15",
    );
    // Read back in the zone it was minted for, the instant is that day's last
    // whole second. It is the day the picker offered, for every reader.
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
