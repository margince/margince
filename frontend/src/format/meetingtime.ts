// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { assertIanaZone, displayDay, INTL_LOCALE } from "./format";
import { hourCycle } from "./preferences";

// When a booked meeting happens, as the pages about one meeting say it: the
// day with nothing left out, and its two times as one span.

/**
 * A meeting's day in full — "Monday, 5 October 2026" — where the year is part
 * of the fact a guest or host acts on.
 */
export function formatDayFull(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(displayDay(utcIso, zone));
}

/**
 * Two instants' wall-clock times as one span — "09:30–10:00". Intl's own range
 * form, so the separator and any trailing unit ("Uhr") are the locale's.
 */
export function formatTimeRange(
  startIso: string,
  endIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: hourCycle(),
  }).formatRange(new Date(startIso), new Date(endIso));
}
