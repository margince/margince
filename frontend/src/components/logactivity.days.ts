// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ISODate, isISODate } from "../design-system/dateinput";
import { dueDayBounds } from "../format/calendarday";
import { type ActivityDraft, todayDay } from "../lib/activitybody";

// The days the logging form's date field accepts for one kind of activity.

export function dueBoundsOf(
  kind: ActivityDraft["kind"],
  recordZone: string,
): { min?: ISODate; max?: ISODate } {
  return kind === "task" ? dueDayBounds(recordZone) : {};
}

export function latestDayOf(
  kind: ActivityDraft["kind"],
  recordZone: string,
): ISODate | undefined {
  const day = todayDay(kind, recordZone);
  return kind !== "task" && isISODate(day) ? day : undefined;
}
