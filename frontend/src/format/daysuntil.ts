// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How many mornings away a record's date is, counted in the RECORD's calendar.
//
// Its own module rather than a line in either caller, because the two record
// heads that print this reading — a deal's close date, a project's target end —
// each counted it for themselves and each counted it in UTC, beside a date they
// drew in the record zone. Two halves of one line, answered in two calendars.

import { calendarDaysBetween, displayDay } from "./format";

/**
 * Whole calendar days from `now` until a date a record carries, counted in the
 * zone that same date is DRAWN in.
 *
 * A count in UTC is wrong beside a rendered date. A reader thirteen hours
 * ahead of UTC, opening a project on the morning of the 18th, read a target
 * end of the 20th as three mornings off. The right answer is two. UTC had not turned the
 * page yet. The count started from a day the reader was no longer in.
 * Both halves name their day the same way here, so they cannot disagree.
 *
 * It is `calendarDaysBetween` in the record zone, with the date rendered
 * through `displayDay` first.
 *
 * Takes both a date-only `yyyy-mm-dd` and a full instant, because `displayDay`
 * does and the date beside this count is rendered through it: a record's date
 * survives every zone, an instant lands on one.
 *
 * Negative once the day is behind us — a count, not a verdict. A caller that
 * needs to know whether a promised MOMENT has passed is asking
 * `format/lateness`, which counts elapsed days rather than calendar ones.
 */
export function calendarDaysUntil(
  value: string,
  zone: string,
  now: Date = new Date(),
): number {
  return calendarDaysBetween(now, displayDay(value, zone), zone);
}
