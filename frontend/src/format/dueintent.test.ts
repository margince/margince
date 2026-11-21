// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { afterEach, expect, it } from "vitest";
import { calendarDay, taskDueOnDay, taskDueZone } from "./calendarday";
import {
  calendarDaysBetween,
  formatDue,
  formatDueCompact,
} from "./format.dates";
import { setDateTimePreferences } from "./preferences";

afterEach(() => {
  setDateTimePreferences({ dateFormat: "locale", timeFormat: "locale" });
});

it("shows the chosen clock and names its zone to a reader elsewhere", () => {
  setDateTimePreferences({ dateFormat: "ymd", timeFormat: "24h" });
  const intent = {
    wall_clock: "2027-03-28T02:30:00",
    timezone: "Europe/Berlin",
  };
  const resolved = "2027-03-28T01:00:00Z";
  for (const format of [formatDue, formatDueCompact]) {
    const ownZone = format(resolved, true, "en", intent.timezone, intent);
    expect(ownZone).toContain("02:30");
    expect(format(resolved, true, "en", "Pacific/Honolulu", intent)).toBe(
      `${ownZone} (Europe/Berlin)`,
    );
  }
});

it("keeps a chosen calendar day when the reader is still on the previous day", () => {
  setDateTimePreferences({ dateFormat: "ymd", timeFormat: "24h" });
  const intent = { wall_clock: "2027-03-28T23:59:59", timezone: "Asia/Tokyo" };
  expect(
    formatDue("2027-03-28T14:59:59Z", false, "en", "Pacific/Honolulu", intent),
  ).toBe("2027-03-28 (Asia/Tokyo)");
});

it("continues to render a legacy instant in the reader's zone", () => {
  setDateTimePreferences({ dateFormat: "ymd", timeFormat: "24h" });
  expect(
    formatDue("2027-03-28T01:00:00Z", false, "en", "Pacific/Honolulu"),
  ).toBe("2027-03-27");
});

it("reads a date-only task's day as the picked date in any zone", () => {
  const intent = { wall_clock: "2026-11-16T23:59:59", timezone: "UTC" };
  for (const zone of ["Europe/Berlin", "America/Los_Angeles", "Asia/Tokyo"]) {
    const instant = taskDueOnDay("2026-11-16T23:59:59Z", false, intent, zone);
    expect(calendarDay(new Date(instant), zone)).toBe("2026-11-16");
  }
});

it("leaves a timed task and a task without a chosen clock on their instant", () => {
  const intent = { wall_clock: "2026-11-16T22:30", timezone: "UTC" };
  expect(taskDueOnDay("2026-11-16T22:30:00Z", true, intent, "Asia/Tokyo")).toBe(
    "2026-11-16T22:30:00Z",
  );
  expect(
    taskDueOnDay("2026-11-16T22:30:00Z", false, undefined, "Asia/Tokyo"),
  ).toBe("2026-11-16T22:30:00Z");
});

// The backend's deadline.WallDayOffset reads the same table, so a task counts
// as due on the same day on both sides.
it("counts the picked day from today the way the backend does", () => {
  const cases: {
    wall_clock: string;
    now: string;
    zone: string;
    offset: number;
  }[] = JSON.parse(
    readFileSync(
      "../backend/internal/shared/kernel/deadline/testdata/pickedday.json",
      "utf8",
    ),
  );
  expect(cases.length).toBeGreaterThan(0);
  for (const { wall_clock, now, zone, offset } of cases) {
    const intent = { wall_clock, timezone: "UTC" };
    const day = calendarDay(
      new Date(taskDueOnDay(`${wall_clock}Z`, false, intent, zone)),
      zone,
    );
    const days = calendarDaysBetween(
      new Date(now),
      new Date(`${day}T12:00:00Z`),
      zone,
    );
    expect(days, `${wall_clock} at ${now} in ${zone}`).toBe(offset);
  }
});

it("counts a dated task on the company's calendar and a timed one on the reader's", () => {
  expect(taskDueZone(false, "Asia/Tokyo", "Europe/Berlin")).toBe(
    "Europe/Berlin",
  );
  expect(taskDueZone(undefined, "Asia/Tokyo", "Europe/Berlin")).toBe(
    "Europe/Berlin",
  );
  expect(taskDueZone(true, "Asia/Tokyo", "Europe/Berlin")).toBe("Asia/Tokyo");
});
