// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { togglePick } from "./booking-invite";
import { overlapsAPick, weekDays } from "./booking-picker";

it("shows the days a host's working hours reach on the reader's clock, not the host's weekdays", () => {
  // Tokyo 09:00–17:00 is Los Angeles 17:00 the evening before until 01:00, so
  // a Monday-to-Friday host is free from Sunday evening to early Friday there.
  const days = weekDays(
    [],
    "2026-10-04T07:00:00Z",
    7,
    {
      days: [1, 2, 3, 4, 5],
      start_time: "09:00",
      end_time: "17:00",
      timezone: "Asia/Tokyo",
    },
    "en",
    "America/Los_Angeles",
  );
  expect(days.map((day) => day.key)).toEqual([
    "2026-10-04",
    "2026-10-05",
    "2026-10-06",
    "2026-10-07",
    "2026-10-08",
    "2026-10-09",
  ]);
});

it("shows only the host’s working days when both clocks agree", () => {
  const days = weekDays(
    [],
    "2026-10-04T22:00:00Z",
    7,
    {
      days: [2, 4],
      start_time: "09:00",
      end_time: "17:00",
      timezone: "Europe/Berlin",
    },
    "en",
    "Europe/Berlin",
  );
  expect(days.map((day) => day.key)).toEqual(["2026-10-06", "2026-10-08"]);
});

// A 30-minute time on the 15-minute grid, as the picker offers it.
const halfHour = (start: string) => ({
  start,
  end: new Date(Date.parse(start) + 30 * 60000).toISOString(),
});

it("refuses a proposed time that shares minutes with one already picked", () => {
  const first = halfHour("2026-11-09T08:30:00Z");
  const overlapping = halfHour("2026-11-09T08:45:00Z");
  const touching = halfHour("2026-11-09T09:00:00Z");

  expect(overlapsAPick([first], overlapping)).toBe(true);
  expect(overlapsAPick([first], touching)).toBe(false);
  // A pick is never blocked by itself, so it can still be unpicked.
  expect(overlapsAPick([first], first)).toBe(false);

  expect(togglePick([first], overlapping, "propose")).toEqual([first]);
  expect(togglePick([first], touching, "propose")).toEqual([first, touching]);
  expect(togglePick([first], first, "propose")).toEqual([]);
  // An invite holds one time, so a new one replaces it.
  expect(togglePick([first], overlapping, "invite")).toEqual([overlapping]);
});
