// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import {
  calendarDay,
  dueInstant,
  shiftCalendarDay,
} from "../../format/calendarday";
import { sameWallClockOnDay, wallClockTime } from "../../format/timezone";

export function calendarSnooze(
  nowMs: number,
  days: number,
  zone: string,
  clock?: string,
): {
  snoozedUntil: string;
  snoozedIntent: components["schemas"]["IntentTime"];
} {
  const now = new Date(nowMs);
  const day = shiftCalendarDay(calendarDay(now, zone), days);
  return {
    snoozedUntil:
      clock === undefined
        ? sameWallClockOnDay(now.toISOString(), day, zone)
        : dueInstant(day, zone, clock),
    snoozedIntent: {
      wall_clock: `${day}T${clock ?? wallClockTime(nowMs, zone, "millisecond")}`,
      timezone: zone,
    },
  };
}
