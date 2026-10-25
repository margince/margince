// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one reader of an activity's time. A `day` activity knows its date and no
// time: the server stores it at noon UTC, so its UTC date is its date.
//
// A `day` entry reads the same date for every viewer and record zone. Only an
// instant is drawn in a zone, the one the screen draws instants in.

import type { components } from "../api/schema";
import type { Locale, Translator } from "../i18n";
import { calendarDay } from "./calendarday";
import { calendarDaysUntil } from "./daysuntil";
import { formatDate, formatDateTime, formatTimeOfDay } from "./format.dates";
import { formatTimeRange } from "./meetingtime";
import { formatElapsed } from "./now";
import { sameWallClockOnDay, UTC_ZONE } from "./timezone";

export type OccurredPrecision = components["schemas"]["OccurredPrecision"];

/** Draws an activity's time; a reader built on formatActivityWhen. */
export type ActivityWhenFormatter = (
  at: string,
  precision: OccurredPrecision,
) => string;

/** An activity's time, and how precisely it is known. */
export type ActivityMoment = Readonly<{
  at: string;
  precision: OccurredPrecision;
}>;

type DayFormatter = (at: string, locale: Locale, zone: string) => string;

function zoneOf(precision: OccurredPrecision, zone: string): string {
  return precision === "day" ? UTC_ZONE : zone;
}

/** When an activity happened: its date and time, or its date alone. */
export function formatActivityWhen(
  at: string,
  precision: OccurredPrecision,
  locale: Locale,
  zone: string,
): string {
  return precision === "day"
    ? formatDate(at, locale, UTC_ZONE)
    : formatDateTime(at, locale, zone);
}

/** The date an activity falls on, drawn by a date-only formatter. */
export function activityDay(
  at: string,
  precision: OccurredPrecision,
  locale: Locale,
  zone: string,
): string;
export function activityDay<Drawn>(
  at: string,
  precision: OccurredPrecision,
  locale: Locale,
  zone: string,
  format: (at: string, locale: Locale, zone: string) => Drawn,
): Drawn;
export function activityDay<Drawn>(
  at: string,
  precision: OccurredPrecision,
  locale: Locale,
  zone: string,
  format?: (at: string, locale: Locale, zone: string) => Drawn,
): Drawn | string {
  const read = zoneOf(precision, zone);
  return format ? format(at, locale, read) : formatDate(at, locale, read);
}

/** The `yyyy-mm-dd` an activity falls on; an instant's is read in `zone`. */
export function activityDate(
  at: string,
  precision: OccurredPrecision,
  zone: string,
): string {
  return calendarDay(new Date(at), zoneOf(precision, zone));
}

/** The clock reading of an activity, or null when only its day is known. */
export function activityTimeOfDay(
  at: string,
  precision: OccurredPrecision,
  locale: Locale,
  zone: string,
): string | null {
  return precision === "day" ? null : formatTimeOfDay(at, locale, zone);
}

/**
 * How long before `now` an activity happened. A `day` entry names its date
 * instead: an age in hours would count from a noon nobody said.
 */
export function activityAgo(
  { at, precision }: ActivityMoment,
  now: number,
  t: Translator,
  locale: Locale,
  format: DayFormatter = formatDate,
): string {
  return precision === "day"
    ? format(at, locale, UTC_ZONE)
    : formatElapsed(now - Date.parse(at), t, locale);
}

/** A meeting's start and end as one span, or null when only its day is known. */
export function activityTimeSpan(
  start: string,
  end: string | undefined,
  precision: OccurredPrecision,
  locale: Locale,
  zone: string,
): string | null {
  if (precision === "day") return null;
  return end
    ? formatTimeRange(start, end, locale, zone)
    : formatTimeOfDay(start, locale, zone);
}

// A `day` entry lasts its whole date, and the record zone says which date is
// today. It has begun once today reaches it, and is ahead until today passes.

/**
 * How many dates lie from today to an activity's date. An instant counts on
 * `zone`'s calendar; a `day` entry counts from today in the record zone.
 */
export function activityDaysAway(
  at: string,
  precision: OccurredPrecision,
  now: number,
  zone: string,
  record: string,
): number {
  return precision === "day"
    ? calendarDaysUntil(
        activityDate(at, precision, zone),
        record,
        new Date(now),
      )
    : calendarDaysUntil(at, zone, new Date(now));
}

/** Whether an activity is not over yet at `now`. */
export function activityIsAhead(
  at: string,
  precision: OccurredPrecision,
  now: number,
  record: string,
): boolean {
  if (precision === "day")
    return activityDaysAway(at, "day", now, record, record) >= 0;
  return Date.parse(at) > now;
}

/** Whether an activity has begun by `now`. */
export function activityHasBegun(
  at: string,
  precision: OccurredPrecision,
  now: number,
  record: string,
): boolean {
  if (precision === "day")
    return activityDaysAway(at, "day", now, record, record) <= 0;
  return Date.parse(at) <= now;
}

/**
 * Whether an activity has taken place by `now`, the mirror of
 * occurredday.Happened. A meeting has once it is not ahead; any other kind once
 * it has begun. A `day` note dated today has; a `day` meeting waits for its date to end.
 */
export function activityHasHappened(
  row: Readonly<{
    kind: string;
    occurred_at: string;
    occurred_precision: OccurredPrecision;
  }>,
  now: number,
  record: string,
): boolean {
  const { occurred_at: at, occurred_precision: precision } = row;
  return row.kind === "meeting"
    ? !activityIsAhead(at, precision, now, record)
    : activityHasBegun(at, precision, now, record);
}

/**
 * The start an activity moved to `day` is sent with. A `day` entry names the
 * date alone. An instant keeps its time of day in `zone`, capped at `now`.
 * Throws a RangeError for a day `zone` skipped.
 */
export function movedActivityStart(
  { at, precision }: ActivityMoment,
  day: string,
  zone: string,
  now: Date,
): ActivityMoment {
  if (precision === "day") return { at: dayOccurredAt(day), precision };
  const moved = sameWallClockOnDay(at, day, zone);
  const capped = Date.parse(moved) > now.getTime() ? now.toISOString() : moved;
  return { at: capped, precision };
}

/** The wire value of a date-only time: noon UTC of the picked date. */
export function dayOccurredAt(day: string): string {
  return `${day}T12:00:00Z`;
}
