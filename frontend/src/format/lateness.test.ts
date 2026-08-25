// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { daysPast } from "./lateness";

// A fixed instant, not the machine's clock: every case below is an offset from
// it, so the suite reads the same in June as in December.
const NOW = Date.parse("2026-08-25T12:00:00Z");
const HOUR = 3_600_000;
const DAY = 24 * HOUR;

describe("daysPast", () => {
  it("calls a promise late the instant it passes, not a day later", () => {
    // The case the person card got wrong: 23 hours past due is zero WHOLE days
    // past due and unambiguously late. Every other surface — the task list,
    // shared/kernel/deadline, the SQL — has always said so.
    expect(daysPast(NOW - 23 * HOUR, NOW)).toEqual({ days: 0, late: true });
    // One millisecond is enough. There is no grace period anywhere else.
    expect(daysPast(NOW - 1, NOW)).toEqual({ days: 0, late: true });
  });

  it("is not late at the instant it falls due", () => {
    // Strictly past, matching deadline.Passed and `due_at < now()`. Being told
    // you have missed something at the moment it becomes due is a false claim.
    expect(daysPast(NOW, NOW)).toEqual({ days: 0, late: false });
    expect(daysPast(NOW + 1, NOW)).toEqual({ days: 0, late: false });
    expect(daysPast(NOW + 30 * DAY, NOW)).toEqual({ days: 0, late: false });
  });

  it("counts whole days elapsed, so the wording never rounds up", () => {
    expect(daysPast(NOW - DAY, NOW)).toEqual({ days: 1, late: true });
    // 47 hours is one whole day and 23 hours, not two days.
    expect(daysPast(NOW - 2 * DAY + HOUR, NOW)).toEqual({
      days: 1,
      late: true,
    });
    expect(daysPast(NOW - 12 * DAY, NOW)).toEqual({ days: 12, late: true });
  });

  it("refuses a verdict on an instant it cannot read", () => {
    // Date.parse of a malformed wire string. The alternative is "overdue NaN
    // days" on a screen, from a comparison that was never made.
    expect(daysPast(Date.parse("not an instant"), NOW)).toEqual({
      days: 0,
      late: false,
    });
  });
});
