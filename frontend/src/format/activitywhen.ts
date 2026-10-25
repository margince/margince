// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one reader of an activity's time. A `day` activity knows its date and no
// time: the server stores it at noon in the record zone, so no clock is shown.
//
// A `day` entry is a date on the record's clock and reads the same for every
// viewer. Only an instant is drawn in the zone the screen draws instants in.

import type { components } from "../api/schema";
import type { Locale, Translator } from "../i18n";
import { calendarDay } from "./calendarday";
import { formatDate, formatDateTime, formatTimeOfDay } from "./format.dates";
import { formatTimeRange } from "./meetingtime";
import { formatElapsed } from "./now";

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

/** The record zone, and the zone this screen draws an instant in. */
export type ActivityZones = Readonly<{ record: string; viewer: string }>;

type DayFormatter = (at: string, locale: Locale, zone: string) => string;

function zoneOf(precision: OccurredPrecision, zones: ActivityZones): string {
  return precision === "day" ? zones.record : zones.viewer;
}

/** When an activity happened: its date and time, or its date alone. */
export function formatActivityWhen(
  at: string,
  precision: OccurredPrecision,
  locale: Locale,
  zones: ActivityZones,
): string {
  return precision === "day"
    ? formatDate(at, locale, zones.record)
    : formatDateTime(at, locale, zones.viewer);
}

/** formatActivityWhen for a record that carries its own time and precision. */
export function formatRecordWhen(
  record: Readonly<{
    occurred_at: string;
    occurred_precision: OccurredPrecision;
  }>,
  locale: Locale,
  zones: ActivityZones,
): string {
  return formatActivityWhen(
    record.occurred_at,
    record.occurred_precision,
    locale,
    zones,
  );
}

/** The date an activity falls on, drawn by a date-only formatter. */
export function activityDay(
  at: string,
  precision: OccurredPrecision,
  locale: Locale,
  zones: ActivityZones,
  format: DayFormatter = formatDate,
): string {
  return format(at, locale, zoneOf(precision, zones));
}

/** The clock reading of an activity, or null when only its day is known. */
export function activityTimeOfDay(
  at: string,
  precision: OccurredPrecision,
  locale: Locale,
  zones: ActivityZones,
): string | null {
  return precision === "day" ? null : formatTimeOfDay(at, locale, zones.viewer);
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
  record: string,
): string {
  return precision === "day"
    ? formatDate(at, locale, record)
    : formatElapsed(now - Date.parse(at), t, locale);
}

/** A meeting's start and end as one span, or null when only its day is known. */
export function activityTimeSpan(
  start: string,
  end: string | undefined,
  precision: OccurredPrecision,
  locale: Locale,
  zones: ActivityZones,
): string | null {
  if (precision === "day") return null;
  return end
    ? formatTimeRange(start, end, locale, zones.viewer)
    : formatTimeOfDay(start, locale, zones.viewer);
}

// A `day` entry is current for its whole date in the record zone. It has begun
// once that date starts, and is still ahead until the date ends.

/** Whether an activity is not over yet at `now`. */
export function activityIsAhead(
  at: string,
  precision: OccurredPrecision,
  now: number,
  record: string,
): boolean {
  if (precision === "day") {
    return (
      calendarDay(new Date(at), record) >= calendarDay(new Date(now), record)
    );
  }
  return Date.parse(at) > now;
}

/** Whether an activity has begun by `now`. */
export function activityHasBegun(
  at: string,
  precision: OccurredPrecision,
  now: number,
  record: string,
): boolean {
  if (precision === "day") {
    return (
      calendarDay(new Date(at), record) <= calendarDay(new Date(now), record)
    );
  }
  return Date.parse(at) <= now;
}

/** A row whose `due_at` is a deadline, never a meeting's start. */
export type DeadlineRow<Row extends { source: string }> = Row & {
  source: Exclude<Row["source"], "meeting">;
};

export function isDeadlineRow<Row extends { source: string }>(
  row: Row,
): row is DeadlineRow<Row> {
  return row.source !== "meeting";
}

/**
 * The wire value of a date-only time. The server reads the date in the value's
 * own offset and stores it at noon in the record zone, so UTC noon names it.
 */
export function dayOccurredAt(day: string): string {
  return `${day}T12:00:00Z`;
}
