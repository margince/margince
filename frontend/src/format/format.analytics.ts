// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { INTL_LOCALE } from "./intllocale";
import { toMajorUnits } from "./minorunits";
import { formatMoneyOrWord, MONEY_ABSENT } from "./moneyword";

export function formatAnalyticsMoney(
  amountMinor: number,
  currency: string,
  locale: Locale,
): string {
  const major = toMajorUnits(amountMinor, currency);
  if (!Number.isFinite(major)) return MONEY_ABSENT;
  const magnitude = Math.abs(major);
  if (magnitude > 0 && magnitude < 0.5)
    return `${major < 0 ? "−(<1)" : "<1"} ${currency}`;
  const rounded = Math.sign(major) * Math.round(magnitude);
  const number = new Intl.NumberFormat(INTL_LOCALE[locale], {
    maximumFractionDigits: 0,
  }).format(rounded === 0 ? 0 : rounded);
  return `${major === rounded ? "" : "≈"}${number} ${currency}`;
}

export function formatAnalyticsNumber(value: number, locale: Locale): string {
  if (!Number.isFinite(value)) return MONEY_ABSENT;
  if (value !== 0 && Math.abs(value) < 0.5) return value < 0 ? "−(<1)" : "<1";
  const rounded = Math.sign(value) * Math.round(Math.abs(value));
  return `${value === rounded ? "" : "≈"}${new Intl.NumberFormat(
    INTL_LOCALE[locale],
    { maximumFractionDigits: 0 },
  ).format(rounded === 0 ? 0 : rounded)}`;
}

export function formatAnalyticsHours(hours: number, locale: Locale): string {
  if (!Number.isFinite(hours)) return MONEY_ABSENT;
  const minutes = Math.round(Math.abs(hours) * 60);
  const part = (value: number, unit: "hour" | "minute") =>
    new Intl.NumberFormat(INTL_LOCALE[locale], {
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
  return `${formatAnalyticsNumber(fraction * 100, locale)}%`;
}
