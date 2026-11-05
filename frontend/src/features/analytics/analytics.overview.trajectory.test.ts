// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
import { monthTicks, weeklyToToday } from "./analytics.overview.trajectory";
import type { ReportingChart } from "./reporting.model";

const quarter = {
  start_at: "2026-09-30T22:00:00Z",
  end_at: "2026-12-31T23:00:00Z",
};

// The server's daily trend: one point per day, each read through its end.
function daily(days: number): ReportingChart {
  const start = Date.parse(quarter.start_at);
  return {
    kind: "bookings_trend",
    metric: "bookings_won",
    context_id: "interval",
    unit: "EUR",
    coverage: { status: "ok", withheld: false },
    points: Array.from({ length: days }, (_, day) => {
      const through = new Date(start + (day + 1) * 86_400_000).toISOString();
      return {
        key: `day-${day}`,
        label: `Day ${day + 1}`,
        value: (day + 1) * 1000,
        status: "ok" as const,
        evidence: { metric: "bookings_won", context_id: "interval", through },
      };
    }),
  };
}

it("keeps one reading a week, counted back from today, so today stays on the line", () => {
  const placed = weeklyToToday(daily(31), quarter);
  expect(placed.map((point) => point.key)).toEqual([
    "day-2",
    "day-9",
    "day-16",
    "day-23",
    "day-30",
  ]);
  expect(placed.at(-1)?.value).toBe(31_000);
});

it("places each reading on the period's own time axis, from 0 to 1", () => {
  const placed = weeklyToToday(daily(92), quarter);
  expect(placed[0]?.at).toBeGreaterThan(0);
  expect(placed.at(-1)?.at).toBeCloseTo(1, 1);
  expect(placed.every((point) => point.at >= 0 && point.at <= 1)).toBe(true);
});

it("draws nothing for a period that has not started", () => {
  expect(
    weeklyToToday(daily(3), { ...quarter, end_at: quarter.start_at }),
  ).toEqual([]);
});

it("ticks the first day of each month in the company's zone, in the reader's language", () => {
  const ticks = monthTicks(quarter, REPORTING_FIXTURE_ZONE, "de");
  expect(ticks.map((tick) => tick.label)).toEqual([
    "Oktober",
    "November",
    "Dezember",
  ]);
  expect(ticks[0]?.at).toBe(0);
  expect(ticks[1]?.at).toBeCloseTo(31 / 92, 2);
});
