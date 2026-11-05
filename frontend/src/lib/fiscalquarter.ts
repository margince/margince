// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { isoDay } from "../format/calendarday";

/**
 * Whether calendar month `month` (1 to 12) opens a quarter of a fiscal year
 * that starts in `fiscalStartMonth`.
 */
export function opensFiscalQuarter(
  month: number,
  fiscalStartMonth: number,
): boolean {
  return monthsIntoQuarter(month, fiscalStartMonth) === 0;
}

/**
 * The fiscal quarter holding `day`, as its first and last calendar day.
 * `fiscalStartMonth` is 1 for a year that starts in January.
 */
export function fiscalQuarter(
  day: string,
  fiscalStartMonth: number,
): Readonly<{ from: string; to: string }> {
  const [year = 0, month = 1] = day.split("-").map(Number);
  // Built from parts and read back by the same clock, so no zone moves a day.
  const start = new Date(
    year,
    month - 1 - monthsIntoQuarter(month, fiscalStartMonth),
    1,
  );
  const end = new Date(start.getFullYear(), start.getMonth() + 3, 0);
  return { from: isoDay(start), to: isoDay(end) };
}

function monthsIntoQuarter(month: number, fiscalStartMonth: number): number {
  return (month - fiscalStartMonth + 12) % 3;
}
