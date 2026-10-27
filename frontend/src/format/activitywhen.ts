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
import {
  calendarDaysBetween,
  formatDate,
  formatDateTime,
  formatTimeOfDay,
} from "./format.dates";
import { formatTimeRange } from "./meetingtime";
import { formatElapsed } from "./now";
import { sameWallClockOnDay, startOfDayInZone, UTC_ZONE } from "./timezone";

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
  zone: string,
  format: DayFormatter = formatDate,
): string {
  return precision === "day"
    ? format(at, locale, UTC_ZONE)
    : formatElapsed(
        now - Date.parse(at),
        t,
        locale,
        Math.max(0, calendarDaysBetween(new Date(at), new Date(now), zone)),
      );
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
 * Whether an activity falls in the period [`from`, `to`), the mirror of
 * occurredday.WithinSQL. An instant falls by its time, a `day` entry where its
 * date begins in the record zone. A bound left out leaves that side open.
 */
export function activityFallsWithin(
  at: string,
  precision: OccurredPrecision,
  period: Readonly<{ from?: number; to?: number }>,
  record: string,
): boolean {
  const start =
    precision === "day"
      ? Date.parse(
          startOfDayInZone(activityDate(at, precision, record), record),
        )
      : Date.parse(at);
  return (
    (period.from === undefined || start >= period.from) &&
    (period.to === undefined || start < period.to)
  );
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

// The server refuses `held` for a start its own clock has not passed, and that
// clock may run behind the browser's.
const HELD_CLOCK_LEAD_MS = 60_000;

/** The latest start a meeting filed as held can carry at `now`. */
export function latestHeldMoment(now: Date): Date {
  return new Date(now.getTime() - HELD_CLOCK_LEAD_MS);
}

/**
 * The last date a meeting filed as held can name. A `day` meeting has taken
 * place only once its date is over in `zone`, so it ends the day before.
 */
export function latestHeldDay(
  precision: OccurredPrecision,
  now: Date,
  zone: string,
): string {
  const today = calendarDay(latestHeldMoment(now), zone);
  if (precision !== "day") return today;
  return calendarDay(
    new Date(Date.parse(startOfDayInZone(today, zone)) - 1),
    zone,
  );
}

/**
 * The start an activity moved to `day` is sent with. A `day` entry names the
 * date alone. An instant keeps its time of day in `zone`, capped at the latest
 * held start. Throws a RangeError for a day `zone` skipped.
 */
export function movedActivityStart(
  { at, precision }: ActivityMoment,
  day: string,
  zone: string,
  now: Date,
): ActivityMoment {
  if (precision === "day") return { at: dayOccurredAt(day), precision };
  const moved = sameWallClockOnDay(at, day, zone);
  const latest = latestHeldMoment(now);
  const capped =
    Date.parse(moved) > latest.getTime() ? latest.toISOString() : moved;
  return { at: capped, precision };
}

/** The wire value of a date-only time: noon UTC of the picked date. */
export function dayOccurredAt(day: string): string {
  return `${day}T12:00:00Z`;
}
