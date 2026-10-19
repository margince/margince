// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The four a sales team reaches for most lead the list, so the common case
// costs no scrolling. The rest come from the platform's own ISO 4217 list.
// The server accepts any assigned code, and a narrower picker hides currencies
// the workspace already prices in.
const LEADING = ["EUR", "USD", "GBP", "CHF"];

/** Every currency code a money field can offer, the leading four first. */
export function currencyCodes(current?: string | null): string[] {
  const platform =
    typeof Intl.supportedValuesOf === "function"
      ? Intl.supportedValuesOf("currency")
      : [];
  const rest = platform.filter((code) => !LEADING.includes(code));
  const extra = current && !LEADING.includes(current) ? [current] : [];
  return [...new Set([...LEADING, ...extra, ...rest])];
}

/**
 * The options of a currency `<Select>`. A code is its own label: an ISO 4217
 * code is not copy, so there is nothing to translate.
 */
export function currencyOptions(
  current?: string | null,
): { value: string; label: string }[] {
  return currencyCodes(current).map((code) => ({ value: code, label: code }));
}
