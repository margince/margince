import type { Locale } from "../i18n";
import { calendarDay, middayInstant } from "./calendarday";
import { INTL_LOCALE } from "./intllocale";
import {
  dateTimePreferences,
  FIXED_DATE_PATTERNS,
  formatPreferredDate,
  hourCycle,
} from "./preferences";
import { assertIanaZone, DAY_MS } from "./timezone";

export { assertIanaZone, isRenderableZone } from "./timezone";

/** A month number has no viewer timezone. UTC keeps Intl's scaffolding in the same month. */
export function monthName(month: number, locale: Locale): string {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    month: "long",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(2026, month - 1, 1)));
}

/**
 * The month a calendar grid is showing, such as "August 2026".
 *
 * A month a reader is looking at, not a moment. The grid is built from local
 * dates, so this reads the local date back instead of pinning a zone. A
 * caller placing a UTC-dated moment names that zone.
 */
export function monthAndYear(
  month: Date,
  locale: Locale,
  zone?: string,
): string {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    month: "long",
    year: "numeric",
    timeZone: zone,
  }).format(month);
}

/**
 * A weekday as the one or two letters a calendar's legend uses.
 *
 * From Intl rather than a list of our own. "S M T W T F S" is English, and a
 * hand-written row would sit unchanged over a German or Vietnamese month.
 */
export function weekdayInitial(day: Date, locale: Locale): string {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    weekday: "narrow",
  }).format(day);
}

/**
 * A calendar day written out in full, such as "Tuesday, 25 August 2026".
 *
 * The accessible name of a day in a grid. "25" alone names no month, and a
 * screen reader reaches the grid's heading several stops away.
 */
export function fullDayName(day: Date, locale: Locale): string {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    dateStyle: "full",
  }).format(day);
}

// Calendar dates are not UTC-midnight instants: their day survives every zone.
export function displayDay(value: string, zone: string): Date {
  return new Date(
    /^\d{4}-\d{2}-\d{2}$/.test(value) ? middayInstant(value, zone) : value,
  );
}

// Zone by purpose: personal deadlines localize to the user zone, and
// reporting-period labels bucket on the workspace zone. The caller picks the
// purpose; this helper only attaches the zone.
export function formatDate(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  const formatter = new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  });
  return formatPreferredDate(formatter, displayDay(utcIso, zone));
}

/**
 * The day as a page names it over a greeting: the weekday, the day and the
 * month written out, as in "Thursday, 4 September". Intl's own names in the
 * reader's locale, so de and vi read correctly with no lookup table of ours.
 * Zone-by-purpose as everywhere here: the day a reader is in, so it takes
 * the viewer's zone, never the workspace's.
 */
export function formatDayLong(
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
  }).format(displayDay(utcIso, zone));
}

// A date a reader scans rather than keys into a form: a record's header meta
// line, never a table column copied into a spreadsheet.
// The abbreviated month (Intl's locale name, so de and vi read correctly) sets
// it apart from `formatDate`, the numeric, sortable form.
export function formatDateAbbrev(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  if (dateTimePreferences().dateFormat !== "locale")
    return formatDate(utcIso, locale, zone);
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "numeric",
    month: "short",
    year: "numeric",
  }).format(displayDay(utcIso, zone));
}

/**
 * A run of calendar days as one span, "29 June – 5 July". Intl's own range
 * form, so each locale joins and shortens the two ends its own way.
 */
export function formatDayRange(
  from: string,
  to: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "numeric",
    month: "long",
  }).formatRange(displayDay(from, zone), displayDay(to, zone));
}

/**
 * A day and month for dates whose surrounding period already supplies the year.
 * A fixed installation format writes it in that format's order without the
 * year ("23.10"), so it reads like every full date beside it.
 */
export function formatDayMonth(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  const format = dateTimePreferences().dateFormat;
  if (format !== "locale") {
    const parts = new Intl.DateTimeFormat(INTL_LOCALE[locale], {
      timeZone: zone,
      day: "2-digit",
      month: "2-digit",
    }).formatToParts(displayDay(utcIso, zone));
    const { order, separator } = FIXED_DATE_PATTERNS[format];
    return order
      .filter((part) => part !== "year")
      .map((type) => parts.find((part) => part.type === type)?.value ?? "")
      .join(separator);
  }
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "numeric",
    month: "short",
  }).format(displayDay(utcIso, zone));
}

// Not `formatDayMonth` on a card: a bare "4 Mar" is ambiguous once the date is
// outside `now`'s year. `now` is the caller's, so the formatter stays pure.
// A fixed numeric date format has no short form, so it always writes the year.
export function formatDayMonthNearYear(
  utcIso: string,
  locale: Locale,
  zone: string,
  now: Date,
): string {
  if (dateTimePreferences().dateFormat !== "locale")
    return formatDate(utcIso, locale, zone);
  assertIanaZone(zone);
  const day = displayDay(utcIso, zone);
  const yearIn = (at: Date) => calendarDay(at, zone).slice(0, 4);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "numeric",
    month: "short",
    ...(yearIn(day) === yearIn(now) ? {} : { year: "numeric" }),
  }).format(day);
}

/**
 * The wall-clock time of an instant, with no date, as in "09:05".
 *
 * For a row whose date is already printed beside it, where repeating the date
 * per line is noise. One caller today: it lives here because this module is
 * where a locale reaches a formatter, not because a second caller is expected.
 * A private copy in the screen would be a second locale decision, which is the
 * thing this module exists to prevent.
 */
export function formatTimeOfDay(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: hourCycle(),
  }).format(new Date(utcIso));
}

export function formatDateTime(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  if (dateTimePreferences().dateFormat !== "locale") {
    return `${formatDate(utcIso, locale, zone)}, ${formatTimeOfDay(utcIso, locale, zone)}`;
  }
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: hourCycle(),
  }).format(new Date(utcIso));
}

/**
 * calendarDaysBetween counts whole days between two instants by the calendar of
 * a named zone. It is the frontend spelling of the backend's
 * shared/kernel/elapsed.Days. The zone is required so a caller cannot count in
 * another calendar than the one its date is drawn in.
 *
 * Not by elapsed milliseconds: calendar days and a duration divided by 24 hours
 * agree only around midnight. The server counts calendar days. A screen
 * counting the other way would print two numbers for one silence.
 *
 * Each instant is named with `calendarDay`. The two names are read as UTC
 * midnights, a whole number of days apart even across a DST change.
 *
 * Held by two tests. format.calendar-days.test.ts pins the boundary cases:
 * 23:00 to 01:00 is one day, not zero, and the zone moves the answer.
 * calendar-days-zone.test.ts fails a caller that names no zone.
 */
export function calendarDaysBetween(
  from: Date,
  to: Date,
  zone: string,
): number {
  const midnightOfNamedDay = (at: Date) =>
    Date.parse(`${calendarDay(at, zone)}T00:00:00Z`);
  return (midnightOfNamedDay(to) - midnightOfNamedDay(from)) / DAY_MS;
}

/**
 * The clock hour, 0-23, that an instant reads as in one zone.
 *
 * Here because this file constructs formatters. Outside it, a locale reaches
 * `Intl` only through `INTL_LOCALE` (`one-locale.test.ts` holds that).
 * The hour carries no locale (08 is 08 in every language). So the tag is `en-GB`
 * for an unambiguous `hourCycle: "h23"`, and the zone is what the caller asks.
 *
 * Used to decide which greeting a reader gets, which is a fact about their
 * morning rather than about any record.
 */
export function hourInZone(instant: Date, zone: string): number {
  const hour = new Intl.DateTimeFormat(INTL_LOCALE.en, {
    hour: "numeric",
    hourCycle: "h23",
    timeZone: zone,
  }).format(instant);
  return Number.parseInt(hour, 10);
}
