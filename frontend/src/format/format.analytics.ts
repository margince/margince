// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { numberFormat } from "./formatters";
import { toMajorUnits } from "./minorunits";
import { formatMoneyOrWord, MONEY_ABSENT } from "./moneyword";

/**
 * An analytics amount in whole major units, written with its currency's sign
 * the way `formatMoney` writes it ("€420,000"). "≈" marks a figure rounded to
 * the unit, and an amount under half a unit reads "<€1" rather than a zero.
 */
export function formatAnalyticsMoney(
  amountMinor: number,
  currency: string,
  locale: Locale,
): string {
  const major = toMajorUnits(amountMinor, currency);
  if (!Number.isFinite(major)) return MONEY_ABSENT;
  const format = numberFormat(locale, {
    style: "currency",
    currency,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  });
  const magnitude = Math.abs(major);
  if (magnitude > 0 && magnitude < 0.5)
    return major < 0 ? `−(<${format.format(1)})` : `<${format.format(1)}`;
  const rounded = Math.sign(major) * Math.round(magnitude);
  return `${major === rounded ? "" : "≈"}${format.format(rounded === 0 ? 0 : rounded)}`;
}

export function formatAnalyticsNumber(value: number, locale: Locale): string {
  if (!Number.isFinite(value)) return MONEY_ABSENT;
  if (value !== 0 && Math.abs(value) < 0.5) return value < 0 ? "−(<1)" : "<1";
  const rounded = Math.sign(value) * Math.round(Math.abs(value));
  return `${value === rounded ? "" : "≈"}${numberFormat(locale, {
    maximumFractionDigits: 0,
  }).format(rounded === 0 ? 0 : rounded)}`;
}

export function formatAnalyticsHours(hours: number, locale: Locale): string {
  if (!Number.isFinite(hours)) return MONEY_ABSENT;
  const minutes = Math.round(Math.abs(hours) * 60);
  const part = (value: number, unit: "hour" | "minute") =>
    numberFormat(locale, {
      style: "unit",
      unit,
      unitDisplay: "short",
      maximumFractionDigits: 0,
    }).format(value);
  if (hours !== 0 && minutes === 0)
    return `${hours < 0 ? "−" : ""}<${part(1, "minute")}`;
  const fullHours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  const text = [
    fullHours ? part(fullHours, "hour") : "",
    rest || !fullHours ? part(rest, "minute") : "",
  ]
    .filter(Boolean)
    .join(" ");
  return `${hours < 0 ? "−" : ""}${text}`;
}

export function formatAnalyticsMoneyOrAbsent(
  amountMinor: number | null | undefined,
  currency: string | null | undefined,
  locale: Locale,
): string {
  return formatMoneyOrWord(
    amountMinor,
    currency,
    locale,
    MONEY_ABSENT,
    formatAnalyticsMoney,
  );
}

export function formatAnalyticsPercent(
  fraction: number,
  locale: Locale,
): string {
  return formatAnalyticsPercentPoints(fraction * 100, locale);
}

/** A figure already in percent (0 to 100), with the sign set as the locale
 *  sets it: "40 %" in German, "40%" in English. */
export function formatAnalyticsPercentPoints(
  points: number,
  locale: Locale,
): string {
  const figure = formatAnalyticsNumber(points, locale);
  return numberFormat(locale, { style: "percent" })
    .formatToParts(0)
    .map((part) => (part.type === "integer" ? figure : part.value))
    .join("");
}
