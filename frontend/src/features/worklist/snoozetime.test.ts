// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { calendarSnooze } from "./snoozetime";

it("keeps the chosen local clock over a daylight-saving change", () => {
  const selected = calendarSnooze(
    Date.parse("2026-03-28T08:30:45.123Z"),
    1,
    "Europe/Berlin",
  );
  expect(selected).toEqual({
    snoozedUntil: "2026-03-29T07:30:45.123Z",
    snoozedIntent: {
      wall_clock: "2026-03-29T09:30:45.123",
      timezone: "Europe/Berlin",
    },
  });
});

it("retains a missing chosen clock rather than replacing it with its resolved hour", () => {
  const selected = calendarSnooze(
    Date.parse("2026-03-28T01:30:00Z"),
    1,
    "Europe/Berlin",
  );
  expect(selected).toEqual({
    snoozedUntil: "2026-03-29T01:00:00.000Z",
    snoozedIntent: {
      wall_clock: "2026-03-29T02:30:00.000",
      timezone: "Europe/Berlin",
    },
  });
});

it("uses the reader's next calendar day for a fixed morning choice", () => {
  expect(
    calendarSnooze(
      Date.parse("2026-11-10T23:30:00Z"),
      1,
      "Asia/Tokyo",
      "08:00",
    ),
  ).toEqual({
    snoozedUntil: "2026-11-11T23:00:00.000Z",
    snoozedIntent: { wall_clock: "2026-11-12T08:00", timezone: "Asia/Tokyo" },
  });
});
