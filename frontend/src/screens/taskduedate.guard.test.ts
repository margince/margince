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
import { dueInstant } from "../format/calendarday";

describe("the days a task may be moved to", () => {
  it("refuses what dueInstant cannot convert", () => {
    // HTML permits a year of four OR MORE digits, so this is a value a native
    // date box reports — and dueInstant throws a RangeError on it. Without the
    // guard a year typo reaches an uncaught exception with nothing on screen
    // to say the date was refused.
    expect(isISODate("10000-09-15")).toBe(false);
    expect(() => dueInstant("10000-09-15")).toThrow();
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
    expect(new Date(dueInstant("2026-09-15")).getTime()).toBe(
      new Date("2026-09-15T23:59:59").getTime(),
    );
  });
});
