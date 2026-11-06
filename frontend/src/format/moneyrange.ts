// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { formatMoneyCompact } from "./format";
import { INTL_LOCALE } from "./intllocale";

/**
 * A range as two short amounts that stay apart. Each end gains a digit until
 * the two read differently, so €24,534,566 to €24,547,766 never reads as
 * "€24.5M–24.5M". Ends that are one amount are said once.
 */
export function formatMoneyCompactRange(
  lowMinor: number,
  highMinor: number,
  currency: string,
  locale: Locale,
): string {
  for (let digits = 3; digits <= 21; digits++) {
    const low = formatMoneyCompact(lowMinor, currency, locale, digits);
    const high = formatMoneyCompact(highMinor, currency, locale, digits);
    if (low !== high) return joinedRange(low, high, currency, locale);
  }
  return formatMoneyCompact(lowMinor, currency, locale);
}

// Two amounts in one currency carry its sign once, as "€610k–980k" or
// "610–980 €", so a range fits the slot a single amount does.
function joinedRange(
  low: string,
  high: string,
  currency: string,
  locale: Locale,
): string {
  const sign = new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "currency",
    currency,
  })
    .formatToParts(0)
    .find((part) => part.type === "currency")?.value;
  if (sign && low.startsWith(sign) && high.startsWith(sign))
    return `${low}–${high.slice(sign.length).trimStart()}`;
  if (sign && low.endsWith(sign) && high.endsWith(sign))
    return `${low.slice(0, -sign.length).trimEnd()}–${high}`;
  return `${low} – ${high}`;
}
