// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { calendarDaysBetween } from "../../../format/format";

/**
 * daysInStage counts the whole days a deal has stood in its stage. It answers
 * null when the date it entered is missing or unreadable. A server older than
 * the field sends none, and a count from no date is not zero days.
 */
export function daysInStage(
  enteredAt: string | undefined,
  now: number,
): number | null {
  const entered = enteredAt ? Date.parse(enteredAt) : Number.NaN;
  if (!Number.isFinite(entered)) {
    return null;
  }
  return Math.max(calendarDaysBetween(new Date(entered), new Date(now)), 0);
}
