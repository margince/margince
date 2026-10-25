// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { isRealCalendarDay } from "./calendarday";
import { INTL_LOCALE } from "./intllocale";
import {
  type DateFormat,
  type DatePart,
  type DatePattern,
  FIXED_DATE_PATTERNS,
} from "./preferences";

export type { DatePattern } from "./preferences";

/** A calendar day as the contract's `format: date` fields spell it. */
export type ISODate = `${number}-${number}-${number}`;

/** Narrows a shape; `isRealCalendarDay` is the check that the day exists. */
export function isISODate(raw: string): raw is ISODate {
  return /^\d{4}-\d{2}-\d{2}$/.test(raw);
}

/** A stored or wire value as the date field's `value`: the day, or `""`. */
export function isoOrBlank(raw: string | null | undefined): ISODate | "" {
  return raw && isISODate(raw) ? raw : "";
}

/** The installation's date format, or the UI language's own when it names none. */
export function datePattern(locale: Locale, format: DateFormat): DatePattern {
  return format === "locale"
    ? localePattern(locale)
    : FIXED_DATE_PATTERNS[format];
}

function localePattern(locale: Locale): DatePattern {
  const parts = new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).formatToParts(new Date(2000, 0, 2));
  const order = parts
    .map((part) => part.type)
    .filter(
      (type): type is DatePart =>
        type === "day" || type === "month" || type === "year",
    );
  const separator = parts.find((part) => part.type === "literal")?.value;
  const [first, second, third] = order;
  if (!first || !second || !third || separator === undefined) {
    return FIXED_DATE_PATTERNS.dmy;
  }
  return { order: [first, second, third], separator };
}

export function formatISODate(day: ISODate, pattern: DatePattern): string {
  const [year, month, date] = day.split("-");
  const value: Record<DatePart, string | undefined> = {
    year,
    month,
    day: date,
  };
  return pattern.order.map((part) => value[part]).join(pattern.separator);
}

/** The pattern spelled in the reader's letters, such as `dd/mm/yyyy`. */
export function patternPlaceholder(
  pattern: DatePattern,
  letters: Readonly<Record<DatePart, string>>,
): string {
  return pattern.order.map((part) => letters[part]).join(pattern.separator);
}

/**
 * A typed day as `YYYY-MM-DD`, or null. A four-digit first part is read year
 * first whatever the pattern, so a pasted ISO date keeps its meaning. A year
 * below 1000 is a typo the instant writers cannot carry, so it reads as none.
 */
export function parseTypedDate(
  text: string,
  pattern: DatePattern,
): ISODate | null {
  const match = /^(\d{1,4})([./-])(\d{1,2})\2(\d{1,4})$/.exec(text.trim());
  if (!match) {
    return null;
  }
  const groups = [match[1], match[3], match[4]];
  const order: DatePattern["order"] =
    groups[0]?.length === 4 ? FIXED_DATE_PATTERNS.ymd.order : pattern.order;
  const value = new Map(order.map((part, index) => [part, groups[index]]));
  const year = value.get("year");
  const month = value.get("month");
  const day = value.get("day");
  if (
    year?.length !== 4 ||
    Number(year) < 1000 ||
    !month ||
    month.length > 2 ||
    !day ||
    day.length > 2
  ) {
    return null;
  }
  const iso = `${year}-${month.padStart(2, "0")}-${day.padStart(2, "0")}`;
  return isISODate(iso) && isRealCalendarDay(iso) ? iso : null;
}
