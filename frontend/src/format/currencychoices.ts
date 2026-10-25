// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** Never empty: a form opens on the first code and sends it for an empty field. */
import { isAssignedCurrency } from "./assignedcurrencies";

export type LeadingCurrencies = readonly [string, ...string[]];

// What a picker leads with before the installation's base currency is known:
// the four a sales team reaches for most. Once the base is known, the base and
// the rate-sheet currencies lead instead (`useLeadingCurrencies`).
export const FALLBACK_LEADING_CURRENCIES: LeadingCurrencies = [
  "EUR",
  "USD",
  "GBP",
  "CHF",
];

/**
 * Every currency code a money field can offer: `leading` first, in the order
 * given, then the code the record already carries, then the platform's ISO
 * 4217 list. Only a code the server assigns is offered as a choice; the code
 * the record already carries stays listed so the field can show it.
 */
export function currencyCodes(
  leading: LeadingCurrencies,
  current?: string | null,
): string[] {
  const platform =
    typeof Intl.supportedValuesOf === "function"
      ? Intl.supportedValuesOf("currency")
      : [];
  return [
    ...new Set([
      ...leading.filter(isAssignedCurrency),
      ...(current ? [current] : []),
      ...platform.filter(isAssignedCurrency),
    ]),
  ];
}

/**
 * The options of a currency `<Select>`. A code is its own label: an ISO 4217
 * code is not copy, so there is nothing to translate.
 */
export function currencyOptions(
  leading: LeadingCurrencies,
  current?: string | null,
): { value: string; label: string }[] {
  return currencyCodes(leading, current).map((code) => ({
    value: code,
    label: code,
  }));
}
