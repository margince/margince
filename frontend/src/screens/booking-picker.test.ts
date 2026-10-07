// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { weekDays } from "./booking-picker";

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
